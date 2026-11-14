package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stateDirSDDL: SYSTEM and administrators may do anything in OffLok's own folder, users only read it (the log, for
// a bug report), and LocalService, which Unbound runs as, too (its configuration). Protected: nothing comes from
// ProgramData, which lets every user create folders and files.
const stateDirSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)(A;OICI;0x1200a9;;;LS)"

// Unbound, as LocalService, may change its keys folder, where it rewrites the root's trust anchor on key rollovers,
// and run its program (unboundKeysSDDL, unboundBinSDDL: added to what the folders inherit).
const (
	unboundKeysSDDL = "D:AI(A;OICI;0x1301bf;;;LS)"
	unboundBinSDDL  = "D:AI(A;OICI;0x1200a9;;;LS)"
)

// unboundKeys is the keys folder, where files LocalService owns are Unbound's own (secureStateDir leaves them).
var unboundKeys string

// grantUnbound lets LocalService run Unbound's program, in binDir, and change its keys folder, in unboundDir.
func grantUnbound(binDir, unboundDir string) error {
	keys := filepath.Join(unboundDir, "keys")
	if err := os.MkdirAll(keys, 0o755); err != nil {
		return err
	}
	for _, g := range []struct{ path, sddl string }{{keys, unboundKeysSDDL}, {binDir, unboundBinSDDL}} {
		sd, err := windows.SecurityDescriptorFromString(g.sddl)
		if err != nil {
			return err
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			return err
		}
		// The folder's own entries become this one; what it inherits stays, and its files inherit both.
		if err := windows.SetNamedSecurityInfo(g.path, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			return fmt.Errorf("%s: set its permissions: %w", g.path, err)
		}
	}
	return nil
}

// secureStateDir makes C:\ProgramData\OffLok offlokd's alone. A user who created it, or something in it, before
// OffLok came could otherwise change what offlokd (SYSTEM) reads and writes there, Unbound's configuration among
// it. Links and anything not owned by SYSTEM or the administrators go; the folder's permissions are then reset,
// and every file in it inherits them.
func secureStateDir(dir string) error {
	unboundKeys = filepath.Join(dir, "unbound", "keys")
	sd, err := windows.SecurityDescriptorFromString(stateDirSDDL)
	if err != nil {
		return err
	}
	// Someone could make the folder again between its removal and its creation: it is looked at once made.
	for attempt := 0; ; attempt++ {
		if err := dropUntrusted(dir); err != nil {
			return err
		}
		if err := createDir(dir, sd); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		ok, err := ours(dir)
		if err != nil {
			return err
		}
		if ok {
			break
		}
		if attempt == 3 {
			return fmt.Errorf("%s keeps being made by someone else", dir)
		}
	}
	// From here on no user can create anything in it; then what was there is looked at.
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("%s: set its permissions: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := cleanTree(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func createDir(dir string, sd *windows.SECURITY_DESCRIPTOR) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(p, sa)
}

// ours reports whether path is a real folder or file (not a link, whose owner would be its target's) owned by
// SYSTEM or the administrators.
func ours(path string) (bool, error) {
	attrs, err := fileAttributes(path)
	if err != nil {
		return false, err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false, nil
	}
	return trustedOwner(path)
}

// dropUntrusted removes path when it is a link or someone else's; it leaves it, and what is in it, otherwise.
func dropUntrusted(path string) error {
	attrs, err := fileAttributes(path)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return removeLink(path, attrs)
	}
	ok, err := trustedOwner(path)
	if err != nil {
		return err
	}
	if !ok && !unboundsOwn(path) {
		return removeTree(path)
	}
	return nil
}

// unboundsOwn reports whether path is a file in the keys folder owned by LocalService: the trust anchor Unbound
// rewrote. Nobody but a program running as LocalService can make one.
func unboundsOwn(path string) bool {
	if unboundKeys == "" || !strings.EqualFold(filepath.Dir(path), unboundKeys) {
		return false
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := sd.Owner()
	return err == nil && owner.IsWellKnown(windows.WinLocalServiceSid)
}

// cleanTree goes through path and what is under it, dropping links and anything not owned by SYSTEM or the
// administrators.
func cleanTree(path string) error {
	if err := dropUntrusted(path); err != nil {
		return err
	}
	attrs, err := fileAttributes(path)
	if err != nil || attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return nil // dropped, or a file
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := cleanTree(filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// removeTree deletes path and everything under it, never following a link: a link is removed, not its target.
func removeTree(path string) error {
	attrs, err := fileAttributes(path)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return removeLink(path, attrs)
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return os.Remove(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := removeTree(filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(path)
}

func removeLink(path string, attrs uint32) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return windows.RemoveDirectory(p) // a junction or a directory link: the link alone goes
	}
	return windows.DeleteFile(p)
}

// fileAttributes reads path's own attributes: a link's, not its target's.
func fileAttributes(path string) (uint32, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.GetFileAttributes(p)
}

func trustedOwner(path string) (bool, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false, fmt.Errorf("%s: read its owner: %w", path, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return false, err
	}
	return owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid), nil
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stateDirSDDL: SYSTEM and administrators may do anything in OffLok's own folder, users only read it (the log, for
// a bug report). Protected: nothing comes from ProgramData, which lets every user create folders and files.
const stateDirSDDL = "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;0x1200a9;;;BU)"

// secureStateDir makes C:\ProgramData\OffLok offlokd's alone. A user who created it, or something in it, before
// OffLok came could otherwise change what offlokd (SYSTEM) reads and writes there, Unbound's configuration among
// it. Links and anything not owned by SYSTEM or the administrators go; the folder's permissions are then reset,
// and every file in it inherits them.
func secureStateDir(dir string) error {
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
	if !ok {
		return removeTree(path)
	}
	return nil
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

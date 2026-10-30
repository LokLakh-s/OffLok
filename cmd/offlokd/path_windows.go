package main

import (
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const environmentKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

// setPath adds dir to the machine's PATH, so that offlok works from any terminal, or takes it out. Every other
// entry stays as it was.
func setPath(dir string, add bool) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, environmentKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	path, typ, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	var out []string
	for _, p := range strings.Split(path, ";") {
		if !strings.EqualFold(filepath.Clean(strings.TrimSpace(p)), filepath.Clean(dir)) {
			out = append(out, p)
		}
	}
	if add {
		if len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1] // a PATH ending in ";"
		}
		out = append(out, dir)
	}
	updated := strings.Join(out, ";")
	if updated == path {
		return nil
	}
	if typ == registry.SZ {
		err = k.SetStringValue("Path", updated)
	} else {
		err = k.SetExpandStringValue("Path", updated)
	}
	if err != nil {
		return err
	}
	announceEnvironment()
	return nil
}

var procSendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

// announceEnvironment tells the open programs (Explorer, which starts the next terminals) that the environment
// changed.
func announceEnvironment() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	env, _ := windows.UTF16PtrFromString("Environment")
	var result uintptr
	procSendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000,
		uintptr(unsafe.Pointer(&result)))
}

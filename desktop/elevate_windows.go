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

// shellExecuteInfo is SHELLEXECUTEINFOW (x/sys/windows has ShellExecute, which gives no process to wait for).
type shellExecuteInfo struct {
	cbSize    uint32
	fMask     uint32
	hwnd      uintptr
	verb      *uint16
	file      *uint16
	params    *uint16
	dir       *uint16
	nShow     int32
	hInstApp  uintptr
	idList    uintptr
	class     *uint16
	hkeyClass uintptr
	hotKey    uint32
	hIcon     uintptr
	hProcess  windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// errCancelled: the user said no to UAC's prompt.
var errCancelled = errors.New("cancelled")

// elevate runs one command through offlok.exe as an administrator: Windows shows UAC's prompt, as polkit's does on
// Linux. It waits for the command and reports its outcome.
func elevate(cmd, value string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"--raw", cmd}
	if value != "" {
		args = append(args, value)
	}
	for i, a := range args {
		args[i] = windows.EscapeArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(filepath.Join(filepath.Dir(exe), "offlok.exe"))
	params, _ := windows.UTF16PtrFromString(strings.Join(args, " "))
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, verb: verb, file: file, params: params, nShow: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errCancelled
		}
		return err
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, 3*60*1000); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("offlok exited with %d", code)
	}
	return nil
}

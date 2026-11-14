package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// notRunningKey is the tray's tooltip when offlokd does not answer: how to start it here.
const notRunningKey = "err.not-running.macos"

// errCancelled: the user closed macOS's password prompt.
var errCancelled = errors.New("cancelled")

// offlokCommand is the command the package installs where only root may change it: what macOS's prompt runs as
// root must not be a file its user can replace (Homebrew makes /usr/local/bin and /opt/homebrew/bin its user's).
const offlokCommand = "/usr/local/libexec/offlok/offlok"

// rootsAlone checks that path and every folder above it belong to root and that nobody else may write to them.
func rootsAlone(path string) error {
	for p := path; ; p = filepath.Dir(p) {
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		sys, ok := st.Sys().(*syscall.Stat_t)
		if !ok || sys.Uid != 0 || st.Mode().Perm()&0o022 != 0 || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s may be changed by someone other than root, so OffLok does not run %s as an administrator (sudo %s still works)", p, path, path)
		}
		if p == "/" {
			return nil
		}
	}
}

// appleScriptString quotes s for AppleScript: backslashes and double quotes escaped.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// elevate runs one command through offlok as an administrator: macOS shows its own password prompt (Touch ID
// where the Mac has it), as polkit's does on Linux and UAC on Windows. It waits for the command and reports its
// outcome.
func elevate(cmd, value string) error {
	offlok := offlokCommand
	if err := rootsAlone(offlok); err != nil {
		return err
	}
	// The shell line is made of quoted words: AppleScript's `quoted form of` quotes each one for the shell.
	words := []string{offlok, "--raw", cmd}
	if value != "" {
		words = append(words, value)
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = "quoted form of " + appleScriptString(w)
	}
	script := "do shell script " + strings.Join(parts, ` & " " & `) + " with administrator privileges"
	out, err := exec.Command("/usr/bin/osascript", "-e", script).CombinedOutput()
	if err == nil {
		return nil
	}
	// AppleScript's "User canceled." is error -128.
	if strings.Contains(string(out), "(-128)") {
		return errCancelled
	}
	return fmt.Errorf("%s", strings.TrimSpace(string(out)))
}

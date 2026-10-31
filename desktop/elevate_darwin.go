package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// notRunningKey is the tray's tooltip when offlokd does not answer: how to start it here.
const notRunningKey = "err.not-running.macos"

// errCancelled: the user closed macOS's password prompt.
var errCancelled = errors.New("cancelled")

// offlokCommand finds the offlok command: the package's, or Homebrew's. An app opened from the Finder has a PATH
// with neither.
func offlokCommand() (string, error) {
	for _, p := range []string{"/usr/local/bin/offlok", "/opt/homebrew/bin/offlok"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("the offlok command is not installed")
}

// appleScriptString quotes s for AppleScript: backslashes and double quotes escaped.
func appleScriptString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// elevate runs one command through offlok as an administrator: macOS shows its own password prompt (Touch ID
// where the Mac has it), as polkit's does on Linux and UAC on Windows. It waits for the command and reports its
// outcome.
func elevate(cmd, value string) error {
	offlok, err := offlokCommand()
	if err != nil {
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

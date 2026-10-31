package main

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/update"
)

// updateLabel is the launchd job macOS's installer runs in during an update.
const updateLabel = "org.loklakh.offlokd.update"

// newUpdater installs a new release's .pkg. Installed with Homebrew, OffLok is Homebrew's to update: no updater.
func newUpdater(src *update.Source, dir string) *daemon.Updater {
	if fromHomebrew() {
		return nil
	}
	return &daemon.Updater{Source: src, Dir: dir, Files: pkgFile, Install: runInstaller}
}

// fromHomebrew: Homebrew's programs live in its Cellar, linked from its bin.
func fromHomebrew() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return strings.Contains(exe, "/Cellar/")
}

func pkgFile(r update.Release) ([]string, error) {
	name := fmt.Sprintf("offlok-%s.pkg", r.Version)
	if _, ok := r.Asset(name); !ok {
		return nil, fmt.Errorf("release %s has no %s", r.Version, name)
	}
	return []string{name}, nil
}

// runInstaller runs macOS's installer in a launchd job of its own, as Linux runs the package manager in a unit of
// its own: the package restarts offlokd (launchctl bootout, then bootstrap), which takes the processes of offlokd's
// job with it, and must not stop the installer halfway. The job writes the installer's exit status to a file, which
// offlokd waits for, so that a failure shows in the status; when the package restarts offlokd, the wait ends with
// this offlokd.
func runInstaller(paths []string) error {
	dir := filepath.Dir(paths[0])
	plist := filepath.Join(dir, updateLabel+".plist")
	out, status := filepath.Join(dir, "installer.log"), filepath.Join(dir, "installer.status")
	_ = launchctl("bootout", "system/"+updateLabel) // the last update's, ended
	for _, f := range []string{out, status} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.WriteFile(plist, installerPlist(paths[0], out, status), 0o644); err != nil {
		return err
	}
	if err := launchctl("bootstrap", "system", plist); err != nil {
		return err
	}
	for deadline := time.Now().Add(15 * time.Minute); time.Now().Before(deadline); time.Sleep(2 * time.Second) {
		b, err := os.ReadFile(status)
		if err != nil {
			continue
		}
		code, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			continue // being written
		}
		if code != 0 {
			log, _ := os.ReadFile(out)
			return fmt.Errorf("the installer ended with %d: %s", code, lastLines(string(log), 5))
		}
		return nil
	}
	return errors.New("the installer did not end within 15 minutes")
}

// installerPlist runs macOS's installer once, through sh, which writes its exit status where offlokd waits for it.
func installerPlist(pkg, out, status string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>-c</string>
		<string>/usr/sbin/installer -pkg "$0" -target / &gt;"$1" 2&gt;&amp;1; echo $? &gt;"$2"</string>
		<string>%s</string>
		<string>%s</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, updateLabel, html.EscapeString(pkg), html.EscapeString(out), html.EscapeString(status))
	return b.Bytes()
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

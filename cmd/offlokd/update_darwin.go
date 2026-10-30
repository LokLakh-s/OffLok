package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/update"
)

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

// runInstaller runs macOS's installer as offlokd (root), in a session of its own: the package restarts offlokd
// through launchd, which ends the service's process group, not another session. The wait mostly ends with offlokd
// itself; a package that fails before shows the installer's message.
func runInstaller(paths []string) error {
	cmd := exec.Command("/usr/sbin/installer", "-pkg", paths[0], "-target", "/")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("the installer: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

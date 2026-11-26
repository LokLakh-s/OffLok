package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/update"
	"golang.org/x/sys/windows"
)

func newUpdater(src *update.Source, dir string) *daemon.Updater {
	return &daemon.Updater{Source: src, Dir: dir, Files: installerFile, Install: runInstaller, Leftovers: removeOld}
}

// removeOld deletes the programs the installer set aside as .old next to offlokd.exe because they ran (the command,
// the tray app). One still running (an old tray app nobody closed) stays until the next start.
func removeOld() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	old, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "*.old"))
	for _, f := range old {
		_ = os.Remove(f)
	}
}

func installerFile(r update.Release) ([]string, error) {
	name := fmt.Sprintf("offlok-%s-setup.exe", r.Version)
	if _, ok := r.Asset(name); !ok {
		return nil, fmt.Errorf("release %s has no %s", r.Version, name)
	}
	return []string{name}, nil
}

// runInstaller runs the installer silently, as offlokd (SYSTEM), in a process group of its own. It stops this
// service to replace it and starts the new one, so the wait mostly ends with offlokd itself; an installer that
// gives up before (an unsupported Windows) shows its exit code.
func runInstaller(paths []string) error {
	cmd := exec.Command(paths[0], "/S")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the installer: %w", err)
	}
	return nil
}

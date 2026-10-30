package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/loklakh-s/offlok/internal/daemon"
	"github.com/loklakh-s/offlok/internal/update"
)

func newUpdater(src *update.Source, dir string) *daemon.Updater {
	return &daemon.Updater{Source: src, Dir: dir, Files: packageFiles, Install: installPackages}
}

// packageKind tells how OffLok was installed: "deb", "rpm", or "" by hand.
func packageKind() string {
	if out, err := exec.Command("dpkg-query", "-W", "-f=${db:Status-Status}", "offlok").Output(); err == nil && string(out) == "installed" {
		return "deb"
	}
	if exec.Command("rpm", "-q", "--quiet", "offlok").Run() == nil {
		return "rpm"
	}
	return ""
}

func installed(kind, pkg string) bool {
	if kind == "deb" {
		out, err := exec.Command("dpkg-query", "-W", "-f=${db:Status-Status}", pkg).Output()
		return err == nil && string(out) == "installed"
	}
	return exec.Command("rpm", "-q", "--quiet", pkg).Run() == nil
}

// packageFiles names the new release's packages for those installed: offlok, and offlok-desktop when it is there.
func packageFiles(r update.Release) ([]string, error) {
	kind := packageKind()
	var names []string
	switch kind {
	case "deb":
		names = append(names, fmt.Sprintf("offlok_%s_%s.deb", r.Version, runtime.GOARCH))
		if installed(kind, "offlok-desktop") {
			names = append(names, fmt.Sprintf("offlok-desktop_%s_%s.deb", r.Version, runtime.GOARCH))
		}
	case "rpm":
		arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
		names = append(names, fmt.Sprintf("offlok-%s-1.%s.rpm", r.Version, arch))
		if installed(kind, "offlok-desktop") {
			names = append(names, fmt.Sprintf("offlok-desktop-%s-1.%s.rpm", r.Version, arch))
		}
	default:
		return nil, errors.New("OffLok was not installed from a package: update it the way it was installed")
	}
	for _, n := range names {
		if _, ok := r.Asset(n); !ok {
			return nil, fmt.Errorf("release %s has no %s", r.Version, n)
		}
	}
	return names, nil
}

// installPackages runs the package manager in a unit of its own: the packages restart offlokd.service, which must
// not stop the package manager halfway. It waits for the unit, so a failure shows in the status.
func installPackages(paths []string) error {
	var cmd []string
	switch {
	case strings.HasSuffix(paths[0], ".deb") && hasCommand("apt-get"):
		cmd = append([]string{"apt-get", "install", "-y", "-o", "Dpkg::Options::=--force-confdef",
			"-o", "Dpkg::Options::=--force-confold"}, paths...)
	case strings.HasSuffix(paths[0], ".deb"):
		cmd = append([]string{"dpkg", "-i"}, paths...)
	case hasCommand("dnf"):
		cmd = append([]string{"dnf", "-y", "install"}, paths...)
	case hasCommand("zypper"):
		cmd = append([]string{"zypper", "--non-interactive", "install", "--allow-unsigned-rpm"}, paths...)
	default:
		cmd = append([]string{"rpm", "-U"}, paths...)
	}
	unit := fmt.Sprintf("offlok-update-%d", time.Now().Unix())
	args := append([]string{"--unit", unit, "--quiet", "--setenv=DEBIAN_FRONTEND=noninteractive", "--"}, cmd...)
	if out, err := exec.Command("systemd-run", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("systemd-run: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// A unit that succeeded is unloaded once it ends; one that failed stays, with its result, until reset.
	for {
		time.Sleep(2 * time.Second)
		out, err := exec.Command("systemctl", "show", unit, "--property=LoadState,ActiveState,Result").Output()
		if err != nil {
			return fmt.Errorf("systemctl show %s: %w", unit, err)
		}
		props := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if k, v, ok := strings.Cut(line, "="); ok {
				props[k] = v
			}
		}
		switch {
		case props["LoadState"] == "not-found":
			return nil
		case props["ActiveState"] == "failed" || props["Result"] != "success" && props["ActiveState"] == "inactive":
			_ = exec.Command("systemctl", "reset-failed", unit).Run()
			return fmt.Errorf("%s failed (%s): journalctl -u %s", cmd[0], props["Result"], unit)
		}
	}
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

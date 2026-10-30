package sysdns

import (
	"errors"
	"os"
	"path/filepath"
)

func resolvConfBackend(stateDir string) *ResolvConf {
	return &ResolvConf{StatePath: filepath.Join(stateDir, "resolv.conf.json")}
}

// Detect picks the backend for this system. A backend that still holds saved settings wins, so that the backend
// Apply used is the one that restores.
func Detect(stateDir string) Backend {
	rc := resolvConfBackend(stateDir)
	if _, err := os.Stat(rc.StatePath); err == nil {
		return rc
	}
	rd := &Resolved{}
	if _, err := os.Stat(rd.dropIn()); err == nil {
		return rd
	}
	if ResolvedActive() {
		return rd
	}
	return rc
}

// RestoreAll restores every backend that holds saved settings: what an uninstall runs.
func RestoreAll(stateDir string) error {
	return errors.Join(resolvConfBackend(stateDir).Restore(), (&Resolved{}).Restore())
}

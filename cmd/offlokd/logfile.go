//go:build windows || darwin

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// openLog writes to offlokd.log in the state directory, the previous run's log kept as offlokd.log.old when it grew
// past 1 MB. A service has no console; the log never holds a looked-up name.
func openLog(stateDir string) *log.Logger {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return log.New(io.Discard, "", 0)
	}
	path := filepath.Join(stateDir, "offlokd.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 1<<20 {
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(f, "", log.LstdFlags)
}

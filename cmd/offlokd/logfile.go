//go:build windows || darwin

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// logLimit is offlokd.log's size; past it, the log goes on in a new file and the full one is kept as
// offlokd.log.old.
const logLimit = 1 << 20

// openLog writes to offlokd.log in the state directory. A service has no console; the log never holds a looked-up
// name.
func openLog(stateDir string) *log.Logger {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return log.New(io.Discard, "", 0)
	}
	w := &cappedLog{path: filepath.Join(stateDir, "offlokd.log")}
	if err := w.open(); err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(w, "", log.LstdFlags)
}

// cappedLog is a log file that never grows past logLimit, while offlokd runs as at its start. The logger it
// serves calls Write one line at a time.
type cappedLog struct {
	path string
	f    *os.File
	size int64
}

func (w *cappedLog) open() error {
	if st, err := os.Stat(w.path); err == nil && st.Size() > logLimit {
		_ = os.Rename(w.path, w.path+".old")
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.f, w.size = f, 0
	if st, err := f.Stat(); err == nil {
		w.size = st.Size()
	}
	return nil
}

func (w *cappedLog) Write(b []byte) (int, error) {
	if w.size+int64(len(b)) > logLimit {
		w.f.Close()
		_ = os.Rename(w.path, w.path+".old")
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	n, err := w.f.Write(b)
	w.size += int64(n)
	return n, err
}

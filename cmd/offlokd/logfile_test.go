//go:build windows || darwin

package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogStaysCapped(t *testing.T) {
	dir := t.TempDir()
	w := &cappedLog{path: filepath.Join(dir, "offlokd.log")}
	if err := w.open(); err != nil {
		t.Fatal(err)
	}
	defer func() { w.f.Close() }() // Windows removes no open file
	l := log.New(w, "", log.LstdFlags)
	line := strings.Repeat("x", 1000)
	for i := 0; i < 3*logLimit/1000; i++ {
		l.Print(line)
	}
	for _, name := range []string{"offlokd.log", "offlokd.log.old"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil || st.Size() > logLimit {
			t.Errorf("%s: %v, %v", name, st.Size(), err)
		}
	}
}

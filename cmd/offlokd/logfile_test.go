//go:build windows || darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogStaysCapped(t *testing.T) {
	dir := t.TempDir()
	l := openLog(dir)
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

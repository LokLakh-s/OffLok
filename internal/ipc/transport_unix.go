//go:build linux || darwin

package ipc

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// listen opens the socket to every local user.
func listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

func dial(path string) (net.Conn, error) {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil && (errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)) {
		return nil, ErrNotRunning
	}
	return c, err
}

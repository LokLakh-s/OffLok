//go:build !windows

package main

import (
	"syscall"
	"time"
)

// waitExit waits for process pid to end, at most timeout.
func waitExit(pid int, timeout time.Duration) {
	for end := time.Now().Add(timeout); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
	}
}

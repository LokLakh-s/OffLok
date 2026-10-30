//go:build !linux

package unbound

import "syscall"

func sysProcAttr() *syscall.SysProcAttr { return nil }

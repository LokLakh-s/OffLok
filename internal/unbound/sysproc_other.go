//go:build darwin

package unbound

import "syscall"

func sysProcAttr() *syscall.SysProcAttr { return nil }

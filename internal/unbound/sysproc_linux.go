package unbound

import "syscall"

// Unbound dies with offlokd: a resolver left behind would keep answering after offlokd gave the system DNS back.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}

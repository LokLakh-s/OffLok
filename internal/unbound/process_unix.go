//go:build unix

package unbound

import (
	"os"
	"syscall"
)

func reload(p *os.Process) error { return p.Signal(syscall.SIGHUP) }

// afterStart has nothing to do: on Linux, Pdeathsig ties Unbound to offlokd (sysproc_linux.go).
func afterStart(*os.Process) (release func(), err error) { return func() {}, nil }

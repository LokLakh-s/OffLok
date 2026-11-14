//go:build unix

package unbound

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func reload(p *os.Process) error { return p.Signal(syscall.SIGHUP) }

// launch starts Unbound, which drops to its own account itself (its configuration's username). On Linux,
// Pdeathsig ties it to offlokd (sysproc_linux.go); on macOS, killStale ends what a dead offlokd left.
func launch(binary, conf string, _ func(string)) (*launched, error) {
	cmd := exec.Command(binary, "-d", "-c", conf)
	cmd.SysProcAttr = sysProcAttr()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = cmd.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start unbound: %w", err)
	}
	return &launched{proc: cmd.Process, stderr: stderr, wait: cmd.Wait, release: func() {}}, nil
}

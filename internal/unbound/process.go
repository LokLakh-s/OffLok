package unbound

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrNoReload means Unbound cannot re-read its configuration in place on this system (Windows, no SIGHUP): stop
// it and start it again.
var ErrNoReload = errors.New("unbound cannot reload here: restart it")

// FindBinary returns the path of the unbound executable.
func FindBinary() (string, error) {
	if p, err := exec.LookPath("unbound"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/usr/sbin/unbound", "/usr/local/sbin/unbound", "/opt/homebrew/sbin/unbound"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", errors.New("unbound is not installed")
}

// Version returns Unbound's version string, such as "1.19.2".
func Version(binary string) (string, error) {
	out, err := exec.Command(binary, "-V").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "Version "); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", errors.New("no version in unbound -V")
}

// Process is one Unbound running in the foreground under offlokd. It never restarts itself: offlokd decides.
type Process struct {
	Binary   string
	ConfPath string
	Log      func(line string) // Unbound's own log lines (it never logs queried names with our config)

	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

// Start launches Unbound. It returns once the process exists, not once it answers: the caller probes for that.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return errors.New("unbound already started")
	}
	cmd := exec.Command(p.Binary, "-d", "-c", p.ConfPath)
	cmd.SysProcAttr = sysProcAttr()
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	cmd.Stdout = cmd.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start unbound: %w", err)
	}
	release, err := afterStart(cmd.Process)
	if err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("tie unbound to offlokd: %w", err)
	}
	p.cmd, p.done, p.err = cmd, make(chan struct{}), nil
	go p.pipe(stderr)
	go func(done chan struct{}) {
		err := cmd.Wait()
		release()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(done)
	}(p.done)
	return nil
}

func (p *Process) pipe(r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		if p.Log != nil {
			p.Log(s.Text())
		}
	}
}

// Running reports whether Unbound is alive.
func (p *Process) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Exited returns a channel closed when Unbound stops, and nil when it was never started.
func (p *Process) Exited() <-chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

// Reload makes Unbound re-read its configuration. Its cache is emptied; listening addresses must not change.
func (p *Process) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		return errors.New("unbound not started")
	}
	return reload(p.cmd.Process)
}

// Stop ends Unbound, killing it if it has not stopped after timeout.
func (p *Process) Stop(timeout time.Duration) error {
	p.mu.Lock()
	cmd, done := p.cmd, p.done
	p.mu.Unlock()
	if cmd == nil {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil { // Windows has no SIGTERM
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
	}
	p.mu.Lock()
	p.cmd, p.done = nil, nil
	p.mu.Unlock()
	return nil
}

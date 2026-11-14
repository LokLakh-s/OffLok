package unbound

import (
	"bufio"
	"errors"
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
	proc *os.Process
	done chan struct{}
	err  error
}

// launched is one Unbound that the system's launch started: launch (process_unix.go, process_windows.go) runs
// it as Unbound's own account where it can, tied to offlokd so that it never outlives it.
type launched struct {
	proc    *os.Process
	stderr  io.Reader    // its log, stdout included
	wait    func() error // waits until it exits
	release func()       // once it has exited
}

// Start launches Unbound. It returns once the process exists, not once it answers: the caller probes for that.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.proc != nil {
		return errors.New("unbound already started")
	}
	killStale(p.Binary, p.ConfPath)
	l, err := launch(p.Binary, p.ConfPath, p.Log)
	if err != nil {
		return err
	}
	p.proc, p.done, p.err = l.proc, make(chan struct{}), nil
	go p.pipe(l.stderr)
	go func(done chan struct{}) {
		err := l.wait()
		l.release()
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(done)
	}(p.done)
	return nil
}

func (p *Process) pipe(r io.Reader) {
	if c, ok := r.(io.Closer); ok {
		defer c.Close() // once Unbound's log ends (exec's Wait closes its pipe again, which does nothing)
	}
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
	if p.proc == nil {
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
	if p.proc == nil {
		return errors.New("unbound not started")
	}
	return reload(p.proc)
}

// Stop ends Unbound, killing it if it has not stopped after timeout.
func (p *Process) Stop(timeout time.Duration) error {
	p.mu.Lock()
	proc, done := p.proc, p.done
	p.mu.Unlock()
	if proc == nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil { // Windows has no SIGTERM
		_ = proc.Kill()
	}
	select {
	case <-done:
	case <-time.After(timeout):
		_ = proc.Kill()
		<-done
	}
	p.mu.Lock()
	p.proc, p.done = nil, nil
	p.mu.Unlock()
	return nil
}

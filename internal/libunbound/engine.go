//go:build cgo && (android || libunbound)

package libunbound

/*
#cgo android,arm64 CFLAGS: -I${SRCDIR}/_native/arm64-v8a/include
#cgo android,arm64 LDFLAGS: -L${SRCDIR}/_native/arm64-v8a/lib
#cgo android,arm CFLAGS: -I${SRCDIR}/_native/armeabi-v7a/include
#cgo android,arm LDFLAGS: -L${SRCDIR}/_native/armeabi-v7a/lib
#cgo android,amd64 CFLAGS: -I${SRCDIR}/_native/x86_64/include
#cgo android,amd64 LDFLAGS: -L${SRCDIR}/_native/x86_64/lib
#cgo LDFLAGS: -lunbound
#cgo android LDFLAGS: -lssl -lcrypto -ldl
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/miekg/dns"
	"golang.org/x/sys/unix"

	"github.com/loklakh-s/offlok/internal/unbound"
)

// queryTimeout bounds one question: Unbound gives up on its servers sooner.
const queryTimeout = 10 * time.Second

// Version is the version of the libunbound linked in, such as "1.26.2".
func Version() string { return C.GoString(C.ub_version()) }

// Engine is libunbound, reading Unbound's configuration file, answering at Addr. One runs at a time: libunbound's
// log is the process's.
type Engine struct {
	Conf string
	Addr netip.AddrPort
	Log  func(line string)

	mu      sync.RWMutex // ctx: queries read it, the loop deletes it
	ctx     *C.struct_ub_ctx
	servers []*dns.Server
	wake    [2]int        // a pipe: Stop writes to it, and the loop ends
	done    chan struct{} // closed once the context is deleted
}

var _ unbound.Engine = (*Engine)(nil)

// New is an Engine for the daemon (daemon.Daemon's Engine).
func New(conf string, addr netip.AddrPort, log func(line string)) unbound.Engine {
	return &Engine{Conf: conf, Addr: addr, Log: log}
}

// Start reads the configuration, starts libunbound's thread and answers at Addr.
func (e *Engine) Start() error {
	e.mu.Lock()
	if e.ctx != nil {
		e.mu.Unlock()
		return errors.New("libunbound already started")
	}
	ctx, logFile, err := e.newContext()
	if err != nil {
		e.mu.Unlock()
		return err
	}
	var wake [2]int
	if err := unix.Pipe2(wake[:], unix.O_CLOEXEC); err != nil {
		C.ub_ctx_delete(ctx)
		C.fclose(logFile)
		e.mu.Unlock()
		return err
	}
	e.ctx, e.wake, e.done = ctx, wake, make(chan struct{})
	go e.loop(ctx, logFile, wake[0], e.done)
	e.mu.Unlock()

	// libunbound reads its configuration at the first question: one it answers by itself says whether it can.
	if _, err := e.resolve("localhost.", dns.TypeA, dns.ClassINET); err != nil {
		_ = e.Stop(0)
		return fmt.Errorf("libunbound: %w", err)
	}
	if err := e.serve(); err != nil {
		_ = e.Stop(0)
		return err
	}
	return nil
}

// newContext is a libunbound context with its own thread, logging to Log, reading Conf.
func (e *Engine) newContext() (*C.struct_ub_ctx, *C.FILE, error) {
	ctx := C.ub_ctx_create()
	if ctx == nil {
		return nil, nil, errors.New("libunbound: cannot create a context")
	}
	var p [2]int
	if err := unix.Pipe2(p[:], unix.O_CLOEXEC); err != nil {
		C.ub_ctx_delete(ctx)
		return nil, nil, err
	}
	logFile := C.offlok_log_file(C.int(p[1]))
	if logFile == nil {
		unix.Close(p[0])
		unix.Close(p[1])
		C.ub_ctx_delete(ctx)
		return nil, nil, errors.New("libunbound: cannot open its log")
	}
	go e.pipe(os.NewFile(uintptr(p[0]), "libunbound-log"))
	fail := func(what string, rc C.int) (*C.struct_ub_ctx, *C.FILE, error) {
		C.ub_ctx_delete(ctx)
		C.fclose(logFile)
		return nil, nil, fmt.Errorf("libunbound: %s: %s", what, C.GoString(C.ub_strerror(rc)))
	}
	if rc := C.ub_ctx_debugout(ctx, unsafe.Pointer(logFile)); rc != 0 {
		return fail("log", rc)
	}
	// A thread, not a process: Android may kill an app's child processes.
	if rc := C.ub_ctx_async(ctx, 1); rc != 0 {
		return fail("thread", rc)
	}
	conf := C.CString(e.Conf)
	defer C.free(unsafe.Pointer(conf))
	if rc := C.ub_ctx_config(ctx, conf); rc != 0 {
		return fail(e.Conf, rc)
	}
	return ctx, logFile, nil
}

func (e *Engine) pipe(f *os.File) {
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if e.Log != nil {
			e.Log(s.Text())
		}
	}
}

// loop hands libunbound's answers to the queries waiting for them, until Stop writes to wake; then it deletes the
// context, which no query can touch any more.
func (e *Engine) loop(ctx *C.struct_ub_ctx, logFile *C.FILE, wake int, done chan struct{}) {
	fds := []unix.PollFd{{Fd: int32(C.ub_fd(ctx)), Events: unix.POLLIN}, {Fd: int32(wake), Events: unix.POLLIN}}
	for {
		if _, err := unix.Poll(fds, -1); err != nil && err != unix.EINTR {
			e.logf("libunbound: poll: %v", err)
			break
		}
		if fds[1].Revents != 0 {
			break
		}
		if fds[0].Revents != 0 {
			C.ub_process(ctx)
		}
	}
	e.mu.Lock()
	e.ctx = nil
	C.ub_ctx_delete(ctx)
	e.mu.Unlock()
	C.fclose(logFile)
	close(done)
}

func (e *Engine) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(fmt.Sprintf(format, args...))
	}
}

// serve answers DNS at Addr, over UDP and TCP, as the unbound program does.
func (e *Engine) serve() error {
	pc, err := net.ListenPacket("udp", e.Addr.String())
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", e.Addr.String())
	if err != nil {
		pc.Close()
		return err
	}
	// Shutdown stops a server only once it serves: Stop may come at once.
	var started sync.WaitGroup
	started.Add(2)
	servers := []*dns.Server{
		{PacketConn: pc, Handler: e, NotifyStartedFunc: started.Done},
		{Listener: l, Handler: e, NotifyStartedFunc: started.Done},
	}
	for _, s := range servers {
		go func(s *dns.Server) { _ = s.ActivateAndServe() }(s)
	}
	started.Wait()
	e.mu.Lock()
	e.servers = servers
	e.mu.Unlock()
	return nil
}

// ServeDNS answers one query.
func (e *Engine) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	var r *dns.Msg
	switch qt := q.Question[0].Qtype; {
	case qt == dns.TypeAXFR, qt == dns.TypeIXFR, qt == dns.TypeMAILA, qt == dns.TypeMAILB:
		r = new(dns.Msg)
		r.SetRcode(q, dns.RcodeRefused)
	default:
		res, err := e.resolve(q.Question[0].Name, qt, q.Question[0].Qclass)
		if err != nil {
			res = nil
		}
		r = reply(q, res)
	}
	if _, tcp := w.RemoteAddr().(*net.TCPAddr); !tcp {
		fit(q, r)
	}
	r.Compress = true
	_ = w.WriteMsg(r)
}

// Reload is not done in place: libunbound's log and some of its locks are the process's, so that two contexts
// cannot overlap.
func (e *Engine) Reload() error { return unbound.ErrNoReload }

// Running reports whether libunbound answers.
func (e *Engine) Running() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ctx != nil
}

// Exited is closed once the engine stopped; nil before Start and after Stop.
func (e *Engine) Exited() <-chan struct{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.done
}

// Stop closes the sockets and deletes the context; the queries still waiting get a SERVFAIL.
func (e *Engine) Stop(time.Duration) error {
	e.mu.Lock()
	servers, wake, done := e.servers, e.wake, e.done
	e.servers, e.done = nil, nil
	e.mu.Unlock()
	if done == nil {
		return nil
	}
	for _, s := range servers {
		_ = s.Shutdown()
	}
	_, _ = unix.Write(wake[1], []byte{0})
	<-done
	unix.Close(wake[0])
	unix.Close(wake[1])
	return nil
}

// The queries waiting for their answer, by the key libunbound hands back.
var pending = struct {
	sync.Mutex
	next uintptr
	m    map[uintptr]chan answer
}{m: map[uintptr]chan answer{}}

type answer struct {
	res *Result
	err error
}

func addPending() (uintptr, chan answer) {
	pending.Lock()
	defer pending.Unlock()
	pending.next++
	ch := make(chan answer, 1)
	pending.m[pending.next] = ch
	return pending.next, ch
}

func takePending(key uintptr) chan answer {
	pending.Lock()
	defer pending.Unlock()
	ch := pending.m[key]
	delete(pending.m, key)
	return ch
}

var errStopped = errors.New("libunbound stopped")

// resolve asks libunbound one question and waits for its answer.
func (e *Engine) resolve(name string, qtype, qclass uint16) (*Result, error) {
	key, ch := addPending()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var id C.int
	e.mu.RLock()
	ctx, done := e.ctx, e.done
	rc := C.int(0)
	if ctx != nil {
		rc = C.offlok_resolve(ctx, cname, C.int(qtype), C.int(qclass), C.uintptr_t(key), &id)
	}
	e.mu.RUnlock()
	if ctx == nil || rc != 0 {
		takePending(key)
		if ctx == nil {
			return nil, errStopped
		}
		return nil, errors.New(C.GoString(C.ub_strerror(rc)))
	}
	t := time.NewTimer(queryTimeout)
	defer t.Stop()
	select {
	case a := <-ch:
		return a.res, a.err
	case <-t.C:
		// Cancelled, libunbound calls back no more; already answering, offlokAnswer finds no one waiting.
		takePending(key)
		e.mu.RLock()
		if e.ctx == ctx {
			C.ub_cancel(ctx, id)
		}
		e.mu.RUnlock()
		return nil, errors.New("no answer in time")
	case <-done:
		takePending(key)
		return nil, errStopped
	}
}

// offlokAnswer is libunbound's callback, in the loop's ub_process.
//
//export offlokAnswer
func offlokAnswer(key C.uintptr_t, rc C.int, res *C.struct_ub_result) {
	if res != nil {
		defer C.ub_resolve_free(res)
	}
	ch := takePending(uintptr(key))
	if ch == nil {
		return
	}
	if rc != 0 || res == nil {
		ch <- answer{err: errors.New(C.GoString(C.ub_strerror(rc)))}
		return
	}
	r := &Result{Rcode: int(res.rcode), Secure: res.secure != 0, Bogus: res.bogus != 0}
	if res.answer_packet != nil && res.answer_len > 0 {
		r.Packet = C.GoBytes(res.answer_packet, res.answer_len)
	}
	ch <- answer{res: r}
}

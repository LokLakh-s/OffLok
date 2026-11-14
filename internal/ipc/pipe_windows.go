package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultSocket is offlokd's named pipe on Windows.
const DefaultSocket = `\\.\pipe\offlok`

// pipeSDDL lets SYSTEM and administrators do anything with the pipe and every signed-in user read and write: the
// handler decides per caller. PIPE_REJECT_REMOTE_CLIENTS keeps the network out.
const pipeSDDL = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;AU)"

const pipeRejectRemoteClients = 0x00000008

// x/sys/windows has neither ImpersonateNamedPipeClient nor CancelSynchronousIo.
var (
	procImpersonateNamedPipeClient = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")
	procCancelSynchronousIo        = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelSynchronousIo")
)

func impersonateNamedPipeClient(h windows.Handle) error {
	if r, _, err := procImpersonateNamedPipeClient.Call(uintptr(h)); r == 0 {
		return err
	}
	return nil
}

type pipeListener struct {
	name      string
	sa        *windows.SecurityAttributes
	mu        sync.Mutex
	closed    bool
	accepting bool           // Accept waits on next: it, not Close, then closes the handles
	next      windows.Handle // the instance the next client connects to: one always waits, so the name never vanishes
}

func listen(name string) (net.Listener, error) {
	sd, err := windows.SecurityDescriptorFromString(pipeSDDL)
	if err != nil {
		return nil, err
	}
	l := &pipeListener{name: name}
	l.sa = &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	// The first instance fails if another process already owns the name: nobody can stand in for offlokd.
	if l.next, err = l.instance(windows.FILE_FLAG_FIRST_PIPE_INSTANCE); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *pipeListener) instance(extra uint32) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(l.name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|extra,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|pipeRejectRemoteClients,
		windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, l.sa)
}

// Accept waits for a client on the waiting instance, and makes the next one before handing the connection over.
func (l *pipeListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	h, closed := l.next, l.closed
	l.accepting = !closed
	l.mu.Unlock()
	if closed {
		return nil, net.ErrClosed
	}
	connErr := windows.ConnectNamedPipe(h, nil)
	if errors.Is(connErr, windows.ERROR_PIPE_CONNECTED) {
		connErr = nil
	}
	next, err := l.instance(0)
	l.mu.Lock()
	closed = l.closed
	l.accepting = false
	if err == nil {
		l.next = next
	}
	l.mu.Unlock()
	if connErr != nil && !closed {
		windows.CloseHandle(h)
		return nil, connErr
	}
	if closed {
		windows.CloseHandle(h)
		if err == nil {
			windows.CloseHandle(next)
		}
		return nil, net.ErrClosed
	}
	if err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &pipeConn{File: os.NewFile(uintptr(h), l.name), h: h}, nil
}

// Close stops Accept, which waits in ConnectNamedPipe: a client of our own lets it return.
func (l *pipeListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	accepting, next := l.accepting, l.next
	l.mu.Unlock()
	if accepting {
		// Accept wakes up, sees closed and closes both instances.
		if c, err := os.OpenFile(l.name, os.O_RDWR, 0); err == nil {
			c.Close()
		}
		return nil
	}
	windows.CloseHandle(next)
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr(l.name) }

// pipePeer reads the client's token by impersonating it for a moment: an elevated member of Administrators is an
// administrator (UAC's split token is not, until elevated).
func pipePeer(h windows.Handle) (Peer, error) {
	var pid uint32
	_ = windows.GetNamedPipeClientProcessId(h, &pid)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := impersonateNamedPipeClient(h); err != nil {
		return Peer{}, err
	}
	var tok windows.Token
	err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &tok)
	if rerr := windows.RevertToSelf(); rerr != nil {
		// Never go on as someone else: offlokd runs as SYSTEM.
		panic("RevertToSelf: " + rerr.Error())
	}
	if err != nil {
		return Peer{}, err
	}
	defer tok.Close()
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return Peer{}, err
	}
	member, err := tok.IsMember(admins)
	if err != nil {
		return Peer{}, err
	}
	system, _ := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	user, err := tok.GetTokenUser()
	isSystem := err == nil && system != nil && user.User.Sid.Equals(system)
	return Peer{PID: int(pid), Admin: (member && tok.IsElevated()) || isSystem}, nil
}

// pipeConn is the server's end of one client's pipe. Its handle does synchronous I/O, which no deadline reaches:
// a deadline cancels the read or write under way from outside (CancelSynchronousIo on the thread doing it), so
// that a client who connects and sends nothing does not hold offlokd's goroutine and handle for ever.
type pipeConn struct {
	*os.File
	h windows.Handle

	mu       sync.Mutex
	deadline time.Time
	timer    *time.Timer
	thread   windows.Handle // the thread in Read or Write, 0 when none
}

// Close lets the client read everything written before the pipe goes.
func (c *pipeConn) Close() error {
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
	}
	c.mu.Unlock()
	_ = windows.FlushFileBuffers(c.h)
	_ = windows.DisconnectNamedPipe(c.h)
	return c.File.Close()
}

func (c *pipeConn) Read(b []byte) (int, error) {
	return c.io(func() (int, error) { return c.File.Read(b) })
}
func (c *pipeConn) Write(b []byte) (int, error) {
	return c.io(func() (int, error) { return c.File.Write(b) })
}

// io runs one read or write on this goroutine's own thread, which the deadline's timer can then cancel.
func (c *pipeConn) io(f func() (int, error)) (int, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var th windows.Handle
	self := windows.CurrentProcess()
	if err := windows.DuplicateHandle(self, windows.CurrentThread(), self, &th, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, err
	}
	defer windows.CloseHandle(th)
	c.mu.Lock()
	if !c.deadline.IsZero() && !time.Now().Before(c.deadline) {
		c.mu.Unlock()
		return 0, os.ErrDeadlineExceeded
	}
	c.thread = th
	c.mu.Unlock()
	n, err := f()
	c.mu.Lock()
	c.thread = 0
	c.mu.Unlock()
	if errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		err = os.ErrDeadlineExceeded
	}
	return n, err
}

// expire cancels the read or write under way. One that has not quite started yet is cancelled a moment later.
func (c *pipeConn) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.thread == 0 || c.deadline.IsZero() || time.Now().Before(c.deadline) {
		return
	}
	if r, _, _ := procCancelSynchronousIo.Call(uintptr(c.thread)); r == 0 {
		c.timer = time.AfterFunc(50*time.Millisecond, c.expire)
	}
}

func (c *pipeConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if !t.IsZero() {
		c.timer = time.AfterFunc(time.Until(t), c.expire)
	}
	return nil
}

func (c *pipeConn) LocalAddr() net.Addr                { return pipeAddr(c.Name()) }
func (c *pipeConn) RemoteAddr() net.Addr               { return pipeAddr(c.Name()) }
func (c *pipeConn) SetReadDeadline(t time.Time) error  { return c.SetDeadline(t) }
func (c *pipeConn) SetWriteDeadline(t time.Time) error { return c.SetDeadline(t) }

// peerOf identifies the client; Windows allows it only once the request was read from the pipe.
func peerOf(c net.Conn) (Peer, error) {
	pc, ok := c.(*pipeConn)
	if !ok {
		return Peer{}, errors.New("not a pipe")
	}
	return pipePeer(pc.h)
}

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

type clientConn struct{ *os.File }

func (c clientConn) LocalAddr() net.Addr                { return pipeAddr(c.Name()) }
func (c clientConn) RemoteAddr() net.Addr               { return pipeAddr(c.Name()) }
func (c clientConn) SetDeadline(t time.Time) error      { return nil }
func (c clientConn) SetReadDeadline(t time.Time) error  { return nil }
func (c clientConn) SetWriteDeadline(t time.Time) error { return nil }

func dial(name string) (net.Conn, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		f, err := os.OpenFile(name, os.O_RDWR, 0)
		if err == nil {
			if err := checkServer(windows.Handle(f.Fd())); err != nil {
				f.Close()
				return nil, err
			}
			return clientConn{f}, nil
		}
		switch {
		case errors.Is(err, windows.ERROR_FILE_NOT_FOUND):
			return nil, ErrNotRunning
		case errors.Is(err, windows.ERROR_PIPE_BUSY) && time.Now().Before(deadline):
			time.Sleep(50 * time.Millisecond)
		default:
			return nil, err
		}
	}
}

// checkServer makes sure the pipe is offlokd's: while offlokd is not running (a restart, before the service starts
// at boot), any user may create a pipe of that name. Its owner is whoever made it, which nobody can forge: SYSTEM
// (the service), the administrators (offlokd run by hand, elevated), or this caller itself.
func checkServer(h windows.Handle) error {
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the owner of offlokd's pipe: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return nil
	}
	if me, err := windows.GetCurrentProcessToken().GetTokenUser(); err == nil && owner.Equals(me.User.Sid) {
		return nil
	}
	return fmt.Errorf("%s belongs to %s, not to offlokd: another program took its name", DefaultSocket, owner)
}

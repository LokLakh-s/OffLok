// Package ipc is the local channel between offlokd and the programs that drive it (the offlok command, the tray
// app): one JSON request per connection, one JSON response.
package ipc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"
)

// Request asks offlokd for one thing.
type Request struct {
	// status, on, off, strict, dot, access, signin, block, trust, untrust, check, pause, list
	Cmd   string `json:"cmd"`
	Value string `json:"value,omitempty"` // on/off, an address, admins/everyone, a site, "ads=on", "add <url>"
	// Interactive: the caller runs in a desktop session, so offlokd may have polkit ask for the administrator's
	// password instead of refusing (the tray app, offlok --polkit).
	Interactive bool `json:"interactive,omitempty"`
}

// Response is offlokd's answer. Status is set on success and on refusals, so callers can always show the state.
type Response struct {
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Denied bool            `json:"denied,omitempty"` // the caller lacks the rights to change settings
	Status json.RawMessage `json:"status,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"` // what a command answers beyond the status (check)
}

// Peer is the local user on the other end of a connection.
type Peer struct {
	UID   int
	PID   int
	Admin bool // root (Linux, macOS)
}

// A password prompt may stay open a while: interactive requests get this long.
const interactiveTimeout = 3 * time.Minute

// requestTimeout is how long a caller has to send its request, in nanoseconds (the tests shorten it).
var requestTimeout atomic.Int64

func init() { requestTimeout.Store(int64(30 * time.Second)) }

const (
	maxRequest  = 64 << 10 // a request is a few words; a caller sending more is cut off
	maxResponse = 16 << 20 // the status, lists included, is far smaller
	maxCallers  = 32       // connections served at once; more wait in the kernel's queue, or fail
)

// Handler answers one request.
type Handler func(Request, Peer) Response

// Serve listens on path until l is closed. The channel is open to every local user: the handler decides per peer.
func Serve(path string, h Handler) (net.Listener, error) {
	l, err := listen(path)
	if err != nil {
		return nil, err
	}
	busy := make(chan struct{}, maxCallers)
	go func() {
		for {
			// A local user opening connections without end gets the channel's attention, not the service's memory.
			busy <- struct{}{}
			c, err := l.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil {
				// A client gone at once (Windows's ERROR_NO_DATA): the channel stays open for the next.
				<-busy
				time.Sleep(50 * time.Millisecond)
				continue
			}
			go func() {
				defer func() { <-busy }()
				handle(c, h)
			}()
		}
	}()
	return l, nil
}

func handle(c net.Conn, h Handler) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Duration(requestTimeout.Load())))
	var req Request
	if err := json.NewDecoder(io.LimitReader(c, maxRequest)).Decode(&req); err != nil {
		_ = json.NewEncoder(c).Encode(Response{Error: "bad request: " + err.Error()})
		return
	}
	// After reading: Windows lets the server impersonate a pipe's client only once data came from it.
	peer, err := peerOf(c)
	if err != nil {
		_ = json.NewEncoder(c).Encode(Response{Error: "cannot identify the caller: " + err.Error()})
		return
	}
	if req.Interactive {
		_ = c.SetDeadline(time.Now().Add(interactiveTimeout))
	}
	_ = json.NewEncoder(c).Encode(h(req, peer))
}

// ErrNotRunning means nothing listens on the socket.
var ErrNotRunning = errors.New("offlokd is not running")

// Call sends req to offlokd at path and returns its response.
func Call(path string, req Request) (Response, error) {
	c, err := dial(path)
	if err != nil {
		return Response{}, err
	}
	defer c.Close()
	timeout := 30 * time.Second
	if req.Interactive {
		timeout = interactiveTimeout
	}
	_ = c.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(io.LimitReader(c, maxResponse)).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("read offlokd's answer: %w", err)
	}
	return resp, nil
}

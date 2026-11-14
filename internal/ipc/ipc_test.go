package ipc

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testPath(t *testing.T) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\offlok-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	}
	return filepath.Join(t.TempDir(), "offlok.sock")
}

func TestRoundTrip(t *testing.T) {
	path := testPath(t)
	var got Peer
	l, err := Serve(path, func(req Request, p Peer) Response {
		got = p
		st, _ := json.Marshal(map[string]string{"echo": req.Cmd + " " + req.Value})
		return Response{OK: true, Status: st}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	for i := 0; i < 3; i++ { // several clients in a row: each gets a fresh pipe instance on Windows
		resp, err := Call(path, Request{Cmd: "check", Value: fmt.Sprint("example.org/", i)})
		if err != nil {
			t.Fatal(err)
		}
		var st map[string]string
		if !resp.OK || json.Unmarshal(resp.Status, &st) != nil || st["echo"] != fmt.Sprint("check example.org/", i) {
			t.Fatalf("response %d: %+v", i, resp)
		}
	}
	if got.PID != os.Getpid() {
		t.Errorf("the caller's pid: %d, want %d", got.PID, os.Getpid())
	}
	t.Logf("peer: %+v", got)
}

func TestCallNotRunning(t *testing.T) {
	if _, err := Call(testPath(t), Request{Cmd: "status"}); err != ErrNotRunning {
		t.Fatalf("nothing listens: want ErrNotRunning, got %v", err)
	}
}

// A caller who connects and sends nothing, or sends without end, is let go: offlokd keeps no goroutine and no
// handle for it (on Windows too, where the pipe's handle does synchronous I/O).
func TestIdleAndOversizedCallers(t *testing.T) {
	old := requestTimeout
	requestTimeout = 300 * time.Millisecond
	defer func() { requestTimeout = old }()
	path := testPath(t)
	l, err := Serve(path, func(req Request, p Peer) Response { return Response{OK: true} })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	idle, err := dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idle.Close()
	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(idle)
		done <- b
	}()
	select {
	case <-done: // closed, with or without a word: its deadline covers the answer too
	case <-time.After(5 * time.Second):
		t.Fatal("an idle caller still holds its connection")
	}

	big, err := dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer big.Close()
	go func() { _, _ = big.Write([]byte(`{"cmd":"` + strings.Repeat("a", maxRequest+1024) + `"}`)) }()
	go func() {
		b, _ := io.ReadAll(big)
		done <- b
	}()
	select {
	case b := <-done:
		if !strings.Contains(string(b), "bad request") {
			t.Errorf("an oversized request got %q", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an oversized request was read without end")
	}

	// The channel still answers.
	if resp, err := Call(path, Request{Cmd: "status"}); err != nil || !resp.OK {
		t.Fatalf("after them: %+v, %v", resp, err)
	}
}

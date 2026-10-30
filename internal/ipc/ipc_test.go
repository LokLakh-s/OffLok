package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

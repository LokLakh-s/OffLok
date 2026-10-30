package daemon

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/update"
)

// releases serves made-up releases signed with a key of the test's own.
type releases struct {
	mu    sync.Mutex
	files map[string][]byte
	priv  ed25519.PrivateKey
	pub   ed25519.PublicKey
}

// publish makes version the latest, with one installer whose content is body.
func (rs *releases) publish(version, body string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	name := "offlok-" + version + "-setup.exe"
	h := sha256.Sum256([]byte(body))
	sums := []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(h[:]), name))
	rs.files = map[string][]byte{
		"/releases/latest/download/SHA256SUMS":        sums,
		"/releases/latest/download/SHA256SUMS.sig":    ed25519.Sign(rs.priv, sums),
		"/releases/download/v" + version + "/" + name: []byte(body),
	}
}

func (rs *releases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rs.mu.Lock()
	data, ok := rs.files[r.URL.Path]
	rs.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(data)
}

func waitUpdate(t *testing.T, d *Daemon) UpdateStatus {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if u := d.Status().Update; u.State == "" {
			return u
		}
	}
	t.Fatal("the update never finished")
	return UpdateStatus{}
}

func TestUpdate(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	rs := &releases{pub: pub, priv: priv}
	rs.publish("0.5.0", "the 0.5.0 installer")
	srv := httptest.NewServer(rs)
	defer srv.Close()

	installed := make(chan []string, 1)
	d := testDaemon(t)
	d.Version = "0.4.0"
	d.Updater = &Updater{
		Source: &update.Source{Base: srv.URL + "/releases", Keys: []ed25519.PublicKey{pub}},
		Dir:    filepath.Join(t.TempDir(), "updates"),
		Files:  func(r update.Release) ([]string, error) { return []string{"offlok-" + r.Version + "-setup.exe"}, nil },
		Install: func(paths []string) error {
			installed <- paths
			return nil
		},
	}

	for _, req := range []ipc.Request{{Cmd: "update"}, {Cmd: "updates", Value: "off"}} {
		if r := d.Handle(req, plain); r.OK || !r.Denied {
			t.Fatalf("%+v is an administrator's: %+v", req, r)
		}
	}
	if err := d.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u := d.Status().Update; u.Available != "0.5.0" || !strings.HasSuffix(u.Page, "/releases/tag/v0.5.0") || u.CheckedAt == nil {
		t.Fatalf("status: %+v", u)
	}
	if d.updateDue() {
		t.Fatal("checked a moment ago: not due")
	}

	if r := d.Handle(ipc.Request{Cmd: "update"}, admin); !r.OK {
		t.Fatal(r.Error)
	}
	select {
	case paths := <-installed:
		if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "the 0.5.0 installer" {
			t.Fatalf("handed over %v: %q, %v", paths, data, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the installer was never run")
	}
	if u := waitUpdate(t, d); u.Error != "" {
		t.Fatalf("after the installer: %+v", u)
	}

	// The same release again, once offlokd is that version: nothing to offer.
	d.Version = "0.5.0"
	if err := d.CheckUpdate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if u := d.Status().Update; u.Available != "" {
		t.Fatalf("offered its own version: %+v", u)
	}
	if r := d.Handle(ipc.Request{Cmd: "update"}, admin); r.OK || !strings.Contains(r.Error, "latest") {
		t.Fatalf("nothing newer: %+v", r)
	}

	// A file that is not the signed one never reaches the installer.
	rs.publish("0.6.0", "the 0.6.0 installer")
	rs.mu.Lock()
	rs.files["/releases/download/v0.6.0/offlok-0.6.0-setup.exe"] = []byte("something else")
	rs.mu.Unlock()
	if r := d.Handle(ipc.Request{Cmd: "update"}, admin); !r.OK {
		t.Fatal(r.Error)
	}
	if u := waitUpdate(t, d); !strings.Contains(u.Error, "SHA-256") {
		t.Fatalf("a changed installer: %+v", u)
	}
	select {
	case paths := <-installed:
		t.Fatalf("handed over %v", paths)
	default:
	}

	// Turned off: no check is due, and the status shows nothing about updates.
	if r := d.Handle(ipc.Request{Cmd: "updates", Value: "off"}, admin); !r.OK {
		t.Fatal(r.Error)
	}
	if u := d.Status().Update; u != (UpdateStatus{}) || d.updateDue() {
		t.Fatalf("off: %+v", u)
	}
}

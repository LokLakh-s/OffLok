package blocklist

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestStore(t *testing.T) {
	var hits, notModified atomic.Int32
	failing := atomic.Bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if failing.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, "||ads.example.org^\n@@||ok.ads.example.org^\n0.0.0.0 track.example.net\n")
	}))
	defer srv.Close()

	s := &Store{Dir: t.TempDir(), Log: log.New(io.Discard, "", 0)}
	sel := Selection(map[Category]bool{Custom: true}, []string{srv.URL + "/list.txt"})
	if len(sel) != 1 || !s.Due(sel) {
		t.Fatalf("a list never downloaded is due: %+v", sel)
	}
	ctx := context.Background()
	if err := s.Refresh(ctx, sel); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(sel); err != nil {
		t.Fatal(err)
	}
	if m, ok := s.Lookup("cdn.ads.example.org"); !ok || m.Category != Custom || m.Entry != "ads.example.org" {
		t.Errorf("Lookup = %+v, %v", m, ok)
	}
	if _, ok := s.Lookup("ok.ads.example.org"); ok {
		t.Error("the list's exception must win")
	}
	if _, ok := s.Lookup("example.org"); ok {
		t.Error("the parent of a listed name is not blocked")
	}
	if s.Due(sel) {
		t.Error("just downloaded: not due")
	}
	if err := s.Refresh(ctx, sel); err != nil || hits.Load() != 1 {
		t.Fatalf("a list checked today is not downloaded again (hits %d, %v)", hits.Load(), err)
	}

	// A day later: the server says nothing changed.
	s.mu.Lock()
	m := s.meta[sel[0].Source.ID]
	m.CheckedAt = time.Now().Add(-25 * time.Hour)
	s.meta[sel[0].Source.ID] = m
	s.mu.Unlock()
	if err := s.Refresh(ctx, sel); err != nil || notModified.Load() != 1 {
		t.Fatalf("expected a conditional request (304): %v, %d", err, notModified.Load())
	}

	// The server fails: the copy on disk stays usable.
	s.mu.Lock()
	m = s.meta[sel[0].Source.ID]
	m.CheckedAt = time.Now().Add(-25 * time.Hour)
	s.meta[sel[0].Source.ID] = m
	s.mu.Unlock()
	failing.Store(true)
	if err := s.Refresh(ctx, sel); err == nil {
		t.Fatal("a failed download must be reported")
	}
	if err := s.Load(sel); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup("track.example.net"); !ok {
		t.Error("the previous copy must stay in use")
	}
	if info := s.Info(sel); len(info) != 1 || info[0].Error == "" || info[0].Names != 2 {
		t.Errorf("Info = %+v", info)
	}

	// Switching the category off empties memory.
	if err := s.Load(nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Lookup("ads.example.org"); ok {
		t.Error("nothing is blocked once no list is selected")
	}
}

func TestSelection(t *testing.T) {
	sel := Selection(map[Category]bool{Ads: true, Malicious: true}, []string{"https://x.example/l.txt"})
	if len(sel) != len(Sources[Ads])+len(Sources[Malicious]) {
		t.Errorf("custom lists come only with their category: %d", len(sel))
	}
	if a, b := CustomSource("https://a.example/1"), CustomSource("https://a.example/2"); a.ID == b.ID {
		t.Error("two custom lists share an ID")
	}
}

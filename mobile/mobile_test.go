//go:build cgo && libunbound

package mobile

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/loklakh-s/offlok/internal/probe"
)

func call(t *testing.T, cmd, value string) result {
	t.Helper()
	var r result
	if err := json.Unmarshal([]byte(Call(cmd, value)), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func state(t *testing.T) string {
	t.Helper()
	var st struct {
		State    string `json:"state"`
		Platform string `json:"platform"`
	}
	r := call(t, "status", "")
	if err := json.Unmarshal(r.Status, &st); err != nil {
		t.Fatal(err)
	}
	return st.State
}

// The whole core as the app runs it, on Linux: libunbound, the front, and a tunnel the test plays the phone's
// resolver on.
func TestCore(t *testing.T) {
	if !probe.RootReachable(context.Background(), []netip.AddrPort{netip.MustParseAddrPort("198.41.0.4:53"), netip.MustParseAddrPort("199.9.14.201:53")}) {
		t.Skip("the root servers are out of reach")
	}
	p := newPhone(t)
	if r := call(t, "status", ""); !r.NotRunning {
		t.Fatalf("status before Start: %+v", r)
	}
	if err := Start(t.TempDir(), "0.6.0", p); err != nil {
		t.Fatal(err)
	}
	defer Stop()
	SetNetwork("test network", "", "", "")
	wait := func(want string) {
		t.Helper()
		for i := 0; i < 60 && state(t) != want; i++ {
			time.Sleep(500 * time.Millisecond)
		}
		if got := state(t); got != want {
			t.Fatalf("state %s, want %s", got, want)
		}
	}
	wait("off") // a first start: off, until the user turns it on
	if r := call(t, "on", ""); !r.OK {
		t.Fatalf("on: %+v", r)
	}
	wait("direct")
	r := p.ask(t, "nlnetlabs.nl.")
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) == 0 {
		t.Errorf("through the tunnel: %v", r)
	}
	if r := call(t, "check", "nlnetlabs.nl"); !r.OK {
		t.Errorf("check: %+v", r)
	}

	// Blocking, with a list of one's own: the phone's lookup gets 0.0.0.0.
	lists := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("||blocked-by-me.example^\n"))
	}))
	defer lists.Close()
	for _, c := range [][2]string{{"block", "custom=on"}, {"list", "add " + lists.URL + "/mine.txt"}} {
		if r := call(t, c[0], c[1]); !r.OK {
			t.Fatalf("%s %s: %+v", c[0], c[1], r)
		}
	}
	var got net.IP
	for i := 0; i < 40; i++ {
		if r := p.ask(t, "blocked-by-me.example."); len(r.Answer) == 1 {
			if a, ok := r.Answer[0].(*dns.A); ok && a.A.Equal(net.IPv4zero) {
				got = a.A
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if got == nil {
		t.Error("a name on the user's list is not blocked through the tunnel")
	}
	if r := call(t, "off", ""); !r.OK {
		t.Fatalf("off: %+v", r)
	}
	wait("off")
	if p.closed.Load() == 0 {
		t.Error("the app was not told the tunnel closed")
	}
}

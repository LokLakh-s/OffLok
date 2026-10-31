//go:build cgo && libunbound

package mobile

import (
	"context"
	"encoding/json"
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
	if r := call(t, "off", ""); !r.OK {
		t.Fatalf("off: %+v", r)
	}
	wait("off")
	if p.closed.Load() == 0 {
		t.Error("the app was not told the tunnel closed")
	}
}

package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/loklakh-s/offlok/internal/blocklist"
	"github.com/loklakh-s/offlok/internal/ipc"
	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/unbound"
)

var box = sysdns.Network{Servers: []netip.Addr{netip.MustParseAddr("192.168.1.254")}, Domains: []string{"home"}}

func dot() *settings.DoT {
	return &settings.DoT{Name: "dns.example.org", Port: 853, Addrs: []netip.Addr{netip.MustParseAddr("192.0.2.1")}}
}

func TestDecide(t *testing.T) {
	strict := settings.Default()
	strict.Strict = true
	withDoT := settings.Default()
	withDoT.DoT = dot()
	strictDoT := strict
	strictDoT.DoT = dot()

	for _, tc := range []struct {
		name string
		s    situation
		want State
		mode unbound.Mode
	}{
		{"root answers", situation{settings: withDoT, rootOK: true, dotOK: true, network: box}, Direct, unbound.Recursive},
		{"root out of reach, personal DoT answers", situation{settings: withDoT, dotOK: true, network: box}, FallbackDoT, unbound.ForwardTLS},
		{"personal DoT down too: the network", situation{settings: withDoT, network: box}, FallbackNetwork, unbound.ForwardNetwork},
		{"no DoT: the network", situation{settings: settings.Default(), network: box}, FallbackNetwork, unbound.ForwardNetwork},
		{"strict: never the network", situation{settings: strict, network: box}, StrictBlocked, unbound.Recursive},
		{"strict still uses the personal DoT", situation{settings: strictDoT, dotOK: true, network: box}, FallbackDoT, unbound.ForwardTLS},
		{"strict, signing in to a captive portal: the network", situation{settings: strict, signin: true, network: box}, FallbackNetwork, unbound.ForwardNetwork},
		{"nothing at all", situation{settings: settings.Default()}, Offline, unbound.Recursive},
	} {
		got := decide(tc.s)
		if got.state != tc.want || got.mode != tc.mode {
			t.Errorf("%s: got %s/%s, want %s/%s", tc.name, got.state, got.mode, tc.want, tc.mode)
		}
	}

	d := decide(situation{settings: withDoT, dotOK: true})
	if len(d.forwarders) != 1 || d.forwarders[0].Addr.String() != "192.0.2.1:853" || d.forwarders[0].TLSName != "dns.example.org" {
		t.Errorf("DoT forwarders: %+v", d.forwarders)
	}
	d = decide(situation{settings: settings.Default(), network: box})
	if len(d.forwarders) != 1 || d.forwarders[0].Addr.String() != "192.168.1.254:53" {
		t.Errorf("network forwarders: %+v", d.forwarders)
	}
}

func TestUsableDropsZonedAddresses(t *testing.T) {
	n := usable(sysdns.Network{Servers: []netip.Addr{netip.MustParseAddr("fe80::1%eth0"), netip.MustParseAddr("::ffff:192.168.1.1")}})
	if len(n.Servers) != 1 || n.Servers[0] != netip.MustParseAddr("192.168.1.1") {
		t.Errorf("%+v", n.Servers)
	}
}

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	d := &Daemon{Paths: Paths{Settings: filepath.Join(t.TempDir(), "settings.json")}, Log: log.New(io.Discard, "", 0)}
	d.settings, d.wake = settings.Default(), make(chan struct{}, 1)
	return d
}

var (
	admin = ipc.Peer{UID: 0, Admin: true}
	plain = ipc.Peer{UID: 1000}
)

func TestHandleRights(t *testing.T) {
	d := testDaemon(t)
	if r := d.Handle(ipc.Request{Cmd: "status"}, plain); !r.OK {
		t.Fatalf("status must be open to everyone: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "off"}, plain); r.OK || !r.Denied {
		t.Fatalf("a user turned OffLok off: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "access", Value: "everyone"}, plain); r.OK || !r.Denied {
		t.Fatalf("a user opened the settings: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "access", Value: "everyone"}, admin); !r.OK {
		t.Fatalf("admin: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "strict", Value: "on"}, plain); !r.OK {
		t.Fatalf("settings opened to everyone: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "access", Value: "admins"}, plain); r.OK {
		t.Fatalf("closing the settings stays an administrator's decision: %+v", r)
	}
	saved, err := settings.Load(d.Paths.Settings)
	if err != nil || !saved.Strict || saved.Access != settings.Everyone {
		t.Fatalf("saved settings: %+v, %v", saved, err)
	}
	if !d.Status().Settings.Strict {
		t.Fatal("status does not show the new setting")
	}
}

func TestHandleBadValues(t *testing.T) {
	d := testDaemon(t)
	for _, req := range []ipc.Request{{Cmd: "strict", Value: "maybe"}, {Cmd: "access", Value: "kids"}, {Cmd: "dot", Value: "dns.example.org:99999"}, {Cmd: "reboot"}} {
		if r := d.Handle(req, admin); r.OK {
			t.Errorf("%+v accepted", req)
		}
	}
}

func TestParseHostPort(t *testing.T) {
	for in, want := range map[string]string{"dns.example.org": "dns.example.org:853", "dns.example.org:8853": "dns.example.org:8853"} {
		h, p, err := parseHostPort(in)
		if got := h + ":" + strconv.Itoa(int(p)); err != nil || got != want {
			t.Errorf("parseHostPort(%q) = %q, %v", in, got, err)
		}
	}
}

func TestHandlePolkit(t *testing.T) {
	d := testDaemon(t)
	asked := 0
	d.Authorize = func(p ipc.Peer) (bool, error) { asked++; return p.UID == 1000, nil }
	if r := d.Handle(ipc.Request{Cmd: "strict", Value: "on"}, plain); r.OK || asked != 0 {
		t.Fatalf("a request that is not interactive must not reach polkit: %+v, asked %d", r, asked)
	}
	if r := d.Handle(ipc.Request{Cmd: "strict", Value: "on", Interactive: true}, plain); !r.OK || asked != 1 {
		t.Fatalf("polkit said yes: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "strict", Value: "off", Interactive: true}, ipc.Peer{UID: 1001}); r.OK || !r.Denied {
		t.Fatalf("polkit said no: %+v", r)
	}
	if r := d.Handle(ipc.Request{Cmd: "access", Value: "everyone", Interactive: true}, plain); !r.OK {
		t.Fatalf("an administrator through polkit may open the settings: %+v", r)
	}
}

func TestSignin(t *testing.T) {
	d := testDaemon(t)
	if r := d.Handle(ipc.Request{Cmd: "signin"}, plain); r.OK {
		t.Fatal("signin outside strict mode must be refused")
	}
	if r := d.Handle(ipc.Request{Cmd: "strict", Value: "on"}, admin); !r.OK {
		t.Fatal(r.Error)
	}
	if r := d.Handle(ipc.Request{Cmd: "signin"}, plain); !r.OK {
		t.Fatalf("signin is open to everyone: %+v", r)
	}
	if st := d.Status(); st.Signin == nil || time.Until(*st.Signin) < 4*time.Minute {
		t.Fatalf("status: %+v", st.Signin)
	}
}

func TestBlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "||ads.example.org^\n")
	}))
	defer srv.Close()
	d := testDaemon(t)
	d.trusted = blocklist.NewSet(nil)
	d.Lists = &blocklist.Store{Dir: t.TempDir(), Log: d.Log}
	d.Backend = nullBackend{}

	if r := d.Handle(ipc.Request{Cmd: "block", Value: "custom=on"}, plain); r.OK || !r.Denied {
		t.Fatalf("switching blocking on is a setting: %+v", r)
	}
	for _, req := range []ipc.Request{{Cmd: "block", Value: "custom=on"}, {Cmd: "list", Value: "add " + srv.URL + "/l.txt"}} {
		if r := d.Handle(req, admin); !r.OK {
			t.Fatalf("%+v: %s", req, r.Error)
		}
	}
	sel := selection(d.Status().Settings)
	if err := d.Lists.Refresh(context.Background(), sel); err != nil {
		t.Fatal(err)
	}
	if err := d.Lists.Load(sel); err != nil {
		t.Fatal(err)
	}
	if !d.blocked("cdn.ads.example.org") || d.blocked("example.org") {
		t.Fatal("the custom list must block ads.example.org and what is under it, nothing else")
	}

	r := d.Handle(ipc.Request{Cmd: "check", Value: "https://cdn.ads.example.org/banner.png"}, plain)
	var c Check
	if !r.OK || json.Unmarshal(r.Data, &c) != nil || !c.Blocked || c.Match == nil || c.Match.Entry != "ads.example.org" {
		t.Fatalf("check is open to everyone and names the list: %+v, %+v", r, c)
	}

	if r := d.Handle(ipc.Request{Cmd: "pause"}, plain); !r.OK || d.blocked("ads.example.org") {
		t.Fatalf("anyone may pause, and a pause lets everything through: %+v", r)
	}
	if st := d.Status(); st.Blocking.PausedUntil == nil || time.Until(*st.Blocking.PausedUntil) < 9*time.Minute {
		t.Fatalf("status: %+v", st.Blocking)
	}
	d.Handle(ipc.Request{Cmd: "pause", Value: "off"}, plain)
	if !d.blocked("ads.example.org") {
		t.Fatal("the pause ended")
	}

	if r := d.Handle(ipc.Request{Cmd: "trust", Value: "www.ads.example.org"}, plain); r.OK {
		t.Fatal("trusting a site is a setting")
	}
	if r := d.Handle(ipc.Request{Cmd: "trust", Value: "WWW.ads.example.org."}, admin); !r.OK {
		t.Fatal(r.Error)
	}
	if d.blocked("www.ads.example.org") || d.blocked("img.www.ads.example.org") || !d.blocked("ads.example.org") {
		t.Fatal("a trusted site and what is under it pass; its parent stays blocked")
	}
	d.Handle(ipc.Request{Cmd: "untrust", Value: "www.ads.example.org"}, admin)
	if !d.blocked("www.ads.example.org") {
		t.Fatal("untrusted again")
	}

	for _, bad := range []ipc.Request{{Cmd: "block", Value: "kittens=on"}, {Cmd: "list", Value: "add ftp://x.example/l"}, {Cmd: "check", Value: "not a site"}} {
		if r := d.Handle(bad, admin); r.OK {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestSiteName(t *testing.T) {
	for in, want := range map[string]string{
		"Example.ORG": "example.org", "https://www.example.org/page?x=1": "www.example.org",
		"www.example.org:8443": "www.example.org", "example.org/path": "example.org", "example.org.": "example.org",
	} {
		if got, err := siteName(in); err != nil || got != want {
			t.Errorf("siteName(%q) = %q, %v", in, got, err)
		}
	}
}

// nullBackend sets nothing: the tests that need no system DNS, on every system.
type nullBackend struct{}

func (nullBackend) Name() string                       { return "none" }
func (nullBackend) Apply([]netip.Addr) error           { return nil }
func (nullBackend) Applied([]netip.Addr) (bool, error) { return true, nil }
func (nullBackend) Restore() error                     { return nil }
func (nullBackend) Network() (sysdns.Network, error)   { return sysdns.Network{}, nil }

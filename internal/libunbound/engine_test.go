//go:build cgo && libunbound

package libunbound

import (
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/loklakh-s/offlok/internal/probe"
	"github.com/loklakh-s/offlok/internal/root"
	"github.com/loklakh-s/offlok/internal/unbound"
)

// freePort is a port free on 127.0.0.1 for UDP and TCP.
func freePort(t *testing.T) uint16 {
	t.Helper()
	for i := 0; i < 20; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if pc, err := net.ListenPacket("udp", l.Addr().String()); err == nil {
			pc.Close()
			return uint16(port)
		}
	}
	t.Fatal("no free port")
	return 0
}

// start runs an Engine on OffLok's own configuration, recursive.
func start(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys")
	if err := os.MkdirAll(keys, 0o755); err != nil {
		t.Fatal(err)
	}
	anchor := filepath.Join(keys, "root.key")
	if err := os.WriteFile(anchor, []byte(root.TrustAnchors), 0o644); err != nil {
		t.Fatal(err)
	}
	addr := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), freePort(t))
	conf, err := unbound.Render(unbound.Config{Listen: []netip.Addr{addr.Addr()}, Port: addr.Port(), Directory: keys,
		TrustAnchorFile: anchor, Mode: unbound.Recursive})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "unbound.conf")
	if err := os.WriteFile(path, conf, 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(path, addr, func(l string) { t.Log("libunbound: " + l) }).(*Engine)
	if err := e.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Stop(0) })
	return e
}

func ask(t *testing.T, addr netip.AddrPort, name string, qtype uint16, mod func(*dns.Msg)) *dns.Msg {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion(name, qtype)
	if mod != nil {
		mod(q)
	}
	c := &dns.Client{Timeout: 15 * time.Second}
	r, _, err := c.Exchange(q, addr.String())
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func TestEngineAnswersByItself(t *testing.T) {
	e := start(t)
	if err := probe.Alive(context.Background(), e.Addr); err != nil {
		t.Fatalf("the probe offlokd waits with: %v", err)
	}
	if !e.Running() || e.Exited() == nil {
		t.Error("not running once started")
	}
	r := ask(t, e.Addr, "localhost.", dns.TypeA, nil)
	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Errorf("localhost: %v", r.Answer)
	}
	if r := ask(t, e.Addr, "example.org.", dns.TypeAXFR, func(q *dns.Msg) { q.Question[0].Qtype = dns.TypeAXFR }); r.Rcode != dns.RcodeRefused {
		t.Errorf("AXFR: %s", dns.RcodeToString[r.Rcode])
	}
}

func TestEngineStopsAndStartsAgain(t *testing.T) {
	e := start(t)
	addr := e.Addr
	if err := e.Stop(0); err != nil {
		t.Fatal(err)
	}
	if e.Running() || e.Exited() != nil {
		t.Error("still running once stopped")
	}
	// What offlokd does to read a new configuration: the same port, at once.
	e2 := New(e.Conf, addr, nil)
	if err := e2.Start(); err != nil {
		t.Fatalf("start again on %s: %v", addr, err)
	}
	defer e2.Stop(0)
	if err := probe.Alive(context.Background(), addr); err != nil {
		t.Fatal(err)
	}
}

// The root out of reach (a container without a network) skips what follows.
func online(t *testing.T) {
	if !probe.RootReachable(context.Background(), []netip.AddrPort{netip.MustParseAddrPort("198.41.0.4:53"), netip.MustParseAddrPort("199.9.14.201:53")}) {
		t.Skip("the root servers are out of reach")
	}
}

func TestEngineResolvesAndValidates(t *testing.T) {
	online(t)
	e := start(t)
	if err := probe.Resolver(context.Background(), e.Addr, true); err != nil {
		t.Fatalf("offlokd's self-test: %v", err)
	}
	do := func(q *dns.Msg) { q.SetEdns0(1232, true) }
	r := ask(t, e.Addr, "nlnetlabs.nl.", dns.TypeA, do)
	if r.Rcode != dns.RcodeSuccess || !r.AuthenticatedData || len(r.Answer) == 0 {
		t.Errorf("nlnetlabs.nl: %s, AD %v, %d records", dns.RcodeToString[r.Rcode], r.AuthenticatedData, len(r.Answer))
	}
	hasSig := false
	for _, rr := range r.Answer {
		hasSig = hasSig || rr.Header().Rrtype == dns.TypeRRSIG
	}
	if !hasSig {
		t.Error("no RRSIG for a client that asked with DO")
	}
	plain := ask(t, e.Addr, "nlnetlabs.nl.", dns.TypeA, nil)
	for _, rr := range plain.Answer {
		if rr.Header().Rrtype == dns.TypeRRSIG {
			t.Error("an RRSIG for a client that asked without DO")
		}
	}
	if r := ask(t, e.Addr, "dnssec-failed.org.", dns.TypeA, nil); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("a broken signature: %s, want SERVFAIL", dns.RcodeToString[r.Rcode])
	}
	cd := ask(t, e.Addr, "dnssec-failed.org.", dns.TypeA, func(q *dns.Msg) { q.CheckingDisabled = true })
	if cd.Rcode != dns.RcodeSuccess || cd.AuthenticatedData {
		t.Errorf("a broken signature, checking disabled: %s, AD %v; want the data, without AD", dns.RcodeToString[cd.Rcode], cd.AuthenticatedData)
	}
	if r := ask(t, e.Addr, "this-name-does-not-exist.nlnetlabs.nl.", dns.TypeA, nil); r.Rcode != dns.RcodeNameError {
		t.Errorf("a name that does not exist: %s", dns.RcodeToString[r.Rcode])
	}
}

func TestEngineOverTCP(t *testing.T) {
	online(t)
	e := start(t)
	q := new(dns.Msg)
	q.SetQuestion("nlnetlabs.nl.", dns.TypeA)
	c := &dns.Client{Net: "tcp", Timeout: 15 * time.Second}
	r, _, err := c.Exchange(q, e.Addr.String())
	if err != nil || r.Rcode != dns.RcodeSuccess || len(r.Answer) == 0 {
		t.Fatalf("over TCP: %v, %v", err, r)
	}
}

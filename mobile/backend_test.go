//go:build linux

package mobile

import (
	"io"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sys/unix"
)

// phone is the app in tests: OpenTunnel gives the core one end of a packet socket pair, as Android gives the
// VPN's; the test reads and writes the other end, as the phone's resolver would.
type phone struct {
	t       *testing.T
	end     *os.File // the phone's end of the last tunnel
	closed  atomic.Int32
	changed chan string
}

func newPhone(t *testing.T) *phone { return &phone{t: t, changed: make(chan string, 16)} }

func (p *phone) OpenTunnel() (int64, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return 0, err
	}
	p.end = os.NewFile(uintptr(fds[1]), "phone")
	p.t.Cleanup(func() { p.end.Close() })
	return int64(fds[0]), nil
}

func (p *phone) TunnelClosed()         { p.closed.Add(1) }
func (p *phone) Changed(status string) { p.changed <- status }
func (p *phone) Log(line string)       { p.t.Log(line) }

// ask sends a query through the tunnel, from the phone's address, and reads the answer.
func (p *phone) ask(t *testing.T, name string) *dns.Msg {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)
	b, _ := q.Pack()
	if _, err := p.end.Write(udp4(netip.MustParseAddr("10.111.222.2"), tunnelAddrs[0], 41000, 53, b)); err != nil {
		t.Fatal(err)
	}
	_ = p.end.SetReadDeadline(time.Now().Add(20 * time.Second))
	buf := make([]byte, 65535)
	n, err := p.end.Read(buf)
	if err != nil {
		t.Fatalf("%s: no answer through the tunnel: %v", name, err)
	}
	r := new(dns.Msg)
	if err := r.Unpack(buf[28:n]); err != nil { // an IPv4 header and a UDP header
		t.Fatal(err)
	}
	return r
}

// udp4 is an IPv4 UDP packet, checksums left at zero (the tunnel's reader does not check them).
func udp4(src, dst netip.Addr, sport, dport uint16, payload []byte) []byte {
	b := make([]byte, 28+len(payload))
	b[0], b[8], b[9] = 0x45, 64, 17
	put16(b[2:], uint16(len(b)))
	s, d := src.As4(), dst.As4()
	copy(b[12:], s[:])
	copy(b[16:], d[:])
	put16(b[20:], sport)
	put16(b[22:], dport)
	put16(b[24:], uint16(8+len(payload)))
	copy(b[28:], payload)
	return b
}

func put16(b []byte, v uint16) { b[0], b[1] = byte(v>>8), byte(v) }

func newBackend(t *testing.T, p *phone, lost func()) *backend {
	answer := func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		rr, _ := dns.NewRR(q.Question[0].Name + " 60 IN A 192.0.2.7")
		r.Answer = []dns.RR{rr}
		_ = w.WriteMsg(r)
	}
	return &backend{platform: p, answer: answer, log: log.New(io.Discard, "", 0), lost: lost}
}

func TestBackendTunnel(t *testing.T) {
	p := newPhone(t)
	b := newBackend(t, p, nil)
	if ok, _ := b.Applied(nil); ok {
		t.Fatal("applied before Apply")
	}
	if err := b.Apply(nil); err != nil {
		t.Fatal(err)
	}
	if ok, _ := b.Applied(nil); !ok {
		t.Fatal("not applied after Apply")
	}
	first := p.end
	if err := b.Apply(nil); err != nil || p.end != first {
		t.Fatal("a second Apply opened another tunnel")
	}
	if r := p.ask(t, "example.org."); len(r.Answer) != 1 {
		t.Errorf("answer through the tunnel: %v", r)
	}
	if err := b.Restore(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := b.Applied(nil); ok || p.closed.Load() != 1 {
		t.Errorf("after Restore: applied %v, the app told %d times", ok, p.closed.Load())
	}
	if w := b.Warning(); w != "" {
		t.Errorf("a warning after OffLok closed its own tunnel: %q", w)
	}
}

// Android closing the tunnel (another VPN app) turns OffLok off: it never takes the VPN back by itself.
func TestBackendTunnelLost(t *testing.T) {
	p := newPhone(t)
	lost := make(chan struct{}, 1)
	b := newBackend(t, p, func() { lost <- struct{}{} })
	if err := b.Apply(nil); err != nil {
		t.Fatal(err)
	}
	p.end.Close()
	select {
	case <-lost:
	case <-time.After(5 * time.Second):
		t.Fatal("the core was not told the tunnel went away")
	}
	if w := b.Warning(); w != warnOtherVPN {
		t.Errorf("warning %q, want %q", w, warnOtherVPN)
	}
	if err := b.Apply(nil); err != errOtherVPN {
		t.Errorf("Apply after the tunnel was lost: %v; want it refused", err)
	}
	b.turningOn()
	if err := b.Apply(nil); err != nil {
		t.Errorf("Apply once the user turns OffLok on: %v", err)
	}
	b.revoked()
	if ok, _ := b.Applied(nil); ok || b.Warning() != warnOtherVPN {
		t.Error("revoked: still applied, or no warning")
	}
}

func TestBackendNetwork(t *testing.T) {
	b := newBackend(t, newPhone(t), nil)
	n, _ := b.Network()
	if len(n.Servers) != 0 {
		t.Errorf("a network before the app told one: %v", n)
	}
	b.setNetwork("wifi 1", networkOf("192.168.1.254\nfd00::1", "home"), "dns.example.net")
	if n, _ := b.Network(); len(n.Servers) != 2 || n.Domains[0] != "home" || b.Fingerprint() != "wifi 1" {
		t.Errorf("network %v, fingerprint %q", n, b.Fingerprint())
	}
	if w := b.Warning(); w != warnPrivateDNS {
		t.Errorf("warning %q, want %q", w, warnPrivateDNS)
	}
}

func TestCABundle(t *testing.T) {
	dir := t.TempDir()
	certs := filepath.Join(dir, "cacerts")
	if err := os.Mkdir(certs, 0o755); err != nil {
		t.Fatal(err)
	}
	// Android's files: the certificate, then its text dump.
	pemCert := "-----BEGIN CERTIFICATE-----\nMAMCAQA=\n-----END CERTIFICATE-----\n"
	for i, f := range []string{"a1b2c3d4.0", "e5f6a7b8.0"} {
		if err := os.WriteFile(filepath.Join(certs, f), []byte(pemCert+"Certificate:\n    Data:\n        Version: "+string(rune('1'+i))+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	saved := caDirs
	caDirs = []string{filepath.Join(dir, "missing"), certs}
	defer func() { caDirs = saved }()
	out := filepath.Join(dir, "bundle.pem")
	if err := writeCABundle(out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if got := strings.Count(string(data), "BEGIN CERTIFICATE"); got != 2 || strings.Contains(string(data), "Data:") {
		t.Errorf("bundle: %d certificates, text kept: %q", got, data)
	}
}

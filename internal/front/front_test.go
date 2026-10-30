package front

import (
	"net"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/miekg/dns"
)

// upstream answers every A query with 192.0.2.1, AD set, and counts the queries it got.
func upstream(t *testing.T) (netip.AddrPort, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	h := dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		n.Add(1)
		r := new(dns.Msg)
		r.SetReply(q)
		r.AuthenticatedData = true
		if q.Question[0].Qtype == dns.TypeA {
			rr, _ := dns.NewRR(q.Question[0].Name + " 300 IN A 192.0.2.1")
			r.Answer = append(r.Answer, rr)
		}
		_ = w.WriteMsg(r)
	})
	// The same port for UDP and TCP; Windows may reserve one and not the other.
	var pc net.PacketConn
	var l net.Listener
	var addr netip.AddrPort
	for i := 0; ; i++ {
		var err error
		if pc, err = net.ListenPacket("udp", "127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		addr = netip.MustParseAddrPort(pc.LocalAddr().String())
		if l, err = net.Listen("tcp", addr.String()); err == nil {
			break
		}
		pc.Close()
		if i == 100 {
			t.Fatal(err)
		}
	}
	for _, srv := range []*dns.Server{{PacketConn: pc, Handler: h}, {Listener: l, Handler: h}} {
		go func(srv *dns.Server) { _ = srv.ActivateAndServe() }(srv)
		t.Cleanup(func() { _ = srv.Shutdown() })
	}
	return addr, &n
}

// freeAddr returns a loopback port free for both UDP and TCP: Windows may reserve one and not the other.
func freeAddr(t *testing.T) netip.AddrPort {
	t.Helper()
	for range 100 {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		a := netip.MustParseAddrPort(pc.LocalAddr().String())
		l, err := net.Listen("tcp", a.String())
		pc.Close()
		if err == nil {
			l.Close()
			return a
		}
	}
	t.Fatal("no loopback port free for both UDP and TCP")
	return netip.AddrPort{}
}

func ask(t *testing.T, network string, addr netip.AddrPort, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	m.SetEdns0(1232, true)
	r, _, err := (&dns.Client{Net: network}).Exchange(m, addr.String())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFront(t *testing.T) {
	up, count := upstream(t)
	addr := freeAddr(t)
	s := &Server{Listen: []netip.AddrPort{addr}, Upstream: up, Blocked: func(n string) bool { return n == "ads.example.org" }}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	for _, network := range []string{"udp", "tcp"} {
		r := ask(t, network, addr, "Ads.Example.Org.", dns.TypeA)
		if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "0.0.0.0" || r.Answer[0].Header().Ttl != blockedTTL {
			t.Errorf("%s: blocked A = %v", network, r.Answer)
		}
		if r.IsEdns0() == nil || !r.IsEdns0().Do() {
			t.Errorf("%s: the blocked answer keeps EDNS and the DO bit", network)
		}
	}
	if r := ask(t, "udp", addr, "ads.example.org.", dns.TypeAAAA); len(r.Answer) != 1 || r.Answer[0].(*dns.AAAA).AAAA.String() != "::" {
		t.Errorf("blocked AAAA = %v", r.Answer)
	}
	if r := ask(t, "udp", addr, "ads.example.org.", dns.TypeHTTPS); len(r.Answer) != 0 || r.Rcode != dns.RcodeSuccess {
		t.Errorf("blocked HTTPS must be empty NOERROR: %v", r)
	}
	if count.Load() != 0 {
		t.Errorf("blocked names must never reach Unbound (%d queries)", count.Load())
	}

	for _, network := range []string{"udp", "tcp"} {
		r := ask(t, network, addr, "www.example.org.", dns.TypeA)
		if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "192.0.2.1" || !r.AuthenticatedData {
			t.Errorf("%s: forwarded answer = %v (AD %v)", network, r.Answer, r.AuthenticatedData)
		}
	}
	if count.Load() != 2 {
		t.Errorf("forwarded %d queries, want 2", count.Load())
	}
}

func TestFrontUpstreamDown(t *testing.T) {
	addr := freeAddr(t)
	s := &Server{Listen: []netip.AddrPort{addr}, Upstream: freeAddr(t)}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if r := ask(t, "tcp", addr, "www.example.org.", dns.TypeA); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("Unbound down: want SERVFAIL, got %s", dns.RcodeToString[r.Rcode])
	}
}

func TestFrontPortTaken(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	s := &Server{Listen: []netip.AddrPort{netip.MustParseAddrPort(pc.LocalAddr().String())}}
	if err := s.Start(); err == nil {
		s.Stop()
		t.Fatal("a taken port must fail")
	}
	if s.Running() {
		t.Error("nothing listens after a failed start")
	}
}

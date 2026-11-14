package probe

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/miekg/dns"
)

// serve starts a UDP DNS server on the loopback that answers the root SOA with the given flags.
func serve(t *testing.T, authoritative, validated bool) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		r.Authoritative = authoritative
		r.AuthenticatedData = validated
		soa, _ := dns.NewRR(". 86400 IN SOA a.root-servers.net. nstld.verisign-grs.com. 2026102801 1800 900 604800 86400")
		r.Answer = append(r.Answer, soa)
		_ = w.WriteMsg(r)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}

func silent(t *testing.T) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}

func TestRootReachable(t *testing.T) {
	ctx := context.Background()
	if !RootReachable(ctx, []netip.AddrPort{serve(t, true, false)}) {
		t.Error("an authoritative answer means the root is reachable")
	}
	if RootReachable(ctx, []netip.AddrPort{serve(t, false, false)}) {
		t.Error("a non-authoritative answer comes from a redirecting resolver, not the root")
	}
	if RootReachable(ctx, []netip.AddrPort{silent(t)}) {
		t.Error("no answer means unreachable")
	}
	if !RootReachable(ctx, []netip.AddrPort{silent(t), serve(t, true, false)}) {
		t.Error("one answering root server is enough")
	}
}

func TestResolver(t *testing.T) {
	ctx := context.Background()
	if err := Resolver(ctx, serve(t, false, true), true); err != nil {
		t.Errorf("validated answer: %v", err)
	}
	if err := Resolver(ctx, serve(t, false, false), true); err == nil {
		t.Error("an answer without AD must fail when validation is required")
	}
	if err := Resolver(ctx, serve(t, false, false), false); err != nil {
		t.Errorf("unvalidated answer allowed: %v", err)
	}
}

// serveDS starts a UDP DNS server on the loopback that answers DS questions as a validating resolver would: a DS
// record set for the names in signed, SERVFAIL for those in broken, NXDOMAIN for the others.
func serveDS(t *testing.T, signed, broken map[string]bool) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		name := q.Question[0].Name
		switch {
		case broken[name]:
			r.Rcode = dns.RcodeServerFailure
		case signed[name] && q.Question[0].Qtype == dns.TypeDS:
			ds, _ := dns.NewRR(name + " 86400 IN DS 19718 13 2 8ACBB0CD28F41250A80A491389424D341522D946B0DA0C0291F2D3D771D7805A")
			r.Answer, r.AuthenticatedData = append(r.Answer, ds), true
		default:
			r.Rcode = dns.RcodeNameError
		}
		_ = w.WriteMsg(r)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}

func TestSigned(t *testing.T) {
	ctx := context.Background()
	addr := serveDS(t, map[string]bool{"com.": true}, map[string]bool{"stripped.example.": true})
	if signed, err := Signed(ctx, addr, "com"); err != nil || !signed {
		t.Errorf("com: signed=%v, %v; want signed", signed, err)
	}
	if signed, err := Signed(ctx, addr, "lan."); err != nil || signed {
		t.Errorf("lan: signed=%v, %v; want unsigned", signed, err)
	}
	if _, err := Signed(ctx, addr, "stripped.example"); err == nil {
		t.Error("a SERVFAIL (a denial that did not validate) must be an error, not an answer")
	}
	if _, err := Signed(ctx, silent(t), "com"); err == nil {
		t.Error("no answer must be an error")
	}
}

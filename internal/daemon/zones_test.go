package daemon

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/miekg/dns"

	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/unbound"
)

// fakeUnbound answers DS questions on the loopback as OffLok's Unbound would: a DS record set for the names in
// signed, no data for the others.
func fakeUnbound(t *testing.T, signed ...string) uint16 {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		if name := q.Question[0].Name; slices.Contains(signed, name) {
			ds, _ := dns.NewRR(name + " 86400 IN DS 19718 13 2 8ACBB0CD28F41250A80A491389424D341522D946B0DA0C0291F2D3D771D7805A")
			r.Answer, r.AuthenticatedData = append(r.Answer, ds), true
		}
		_ = w.WriteMsg(r)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return netip.MustParseAddrPort(pc.LocalAddr().String()).Port()
}

func TestNetworkDomains(t *testing.T) {
	d := testDaemon(t)
	d.UnboundPort = fakeUnbound(t, "com.", "bank.example.")
	// A hostile network declares a TLD and a signed domain next to its own names; one domain would inject a line.
	d.network = sysdns.Network{Servers: box.Servers,
		Domains: []string{"fritz.box", "com", "Bank.Example.", "lan", "corp.home.arpa", "evil\"\nserver:"}}

	// Not checked yet: only the private zones, unless every name goes to the network's DNS anyway.
	if got, want := d.localDomains(unbound.Recursive), []string{"lan.", "corp.home.arpa."}; !slices.Equal(got, want) {
		t.Errorf("before the check, recursing: %v, want %v", got, want)
	}
	if got := d.localDomains(unbound.ForwardNetwork); !slices.Equal(got, []string{"fritz.box.", "com.", "bank.example.", "lan.", "corp.home.arpa."}) {
		t.Errorf("before the check, in fallback to the network: %v", got)
	}

	if !d.checkZones(context.Background()) {
		t.Fatal("checkZones found no unsigned domain")
	}
	// Signed in the public DNS: never the network's, whatever the mode.
	for _, mode := range []unbound.Mode{unbound.Recursive, unbound.ForwardTLS, unbound.ForwardNetwork} {
		if got, want := d.localDomains(mode), []string{"fritz.box.", "lan.", "corp.home.arpa."}; !slices.Equal(got, want) {
			t.Errorf("after the check (%s): %v, want %v", mode, got, want)
		}
	}
	if d.checkZones(context.Background()) {
		t.Error("a second check asked again")
	}

	cfg, err := unbound.Render(unbound.Config{Listen: []netip.Addr{netip.MustParseAddr("127.0.0.1")}, Port: 53530,
		Directory: "/k", TrustAnchorFile: "/k/root.key", Mode: unbound.Recursive, NetworkDNS: box.Servers,
		LocalZones: append(unbound.PrivateZones(), d.localDomains(unbound.Recursive)...)})
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`"com."`, `"bank.example."`, "evil"} {
		if strings.Contains(string(cfg), bad) {
			t.Errorf("the configuration names %s:\n%s", bad, cfg)
		}
	}
	if !strings.Contains(string(cfg), `domain-insecure: "fritz.box."`) {
		t.Errorf("fritz.box, unsigned, is not the network's:\n%s", cfg)
	}
}

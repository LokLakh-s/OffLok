package libunbound

import (
	"testing"

	"github.com/miekg/dns"
)

func query(name string, qtype uint16, edns, do bool) *dns.Msg {
	q := new(dns.Msg)
	q.SetQuestion(name, qtype)
	if edns {
		q.SetEdns0(4096, do)
	}
	return q
}

// signed is libunbound's packet for a signed A record: the record, its signature, an NSEC and a DS in the
// authority section, and libunbound's OPT (it always asks with DO).
func signed(t *testing.T, rcode int) []byte {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion("www.example.org.", dns.TypeA)
	m.Rcode = rcode
	for _, s := range []string{
		"www.example.org. 300 IN A 192.0.2.1",
		"www.example.org. 300 IN RRSIG A 13 3 300 20301231000000 20201231000000 12345 example.org. AAAA",
	} {
		rr, err := dns.NewRR(s)
		if err != nil {
			t.Fatal(err)
		}
		m.Answer = append(m.Answer, rr)
	}
	for _, s := range []string{
		"example.org. 300 IN NSEC zzz.example.org. A RRSIG NSEC",
		"example.org. 300 IN NS ns.example.org.",
		"example.org. 300 IN DS 12345 13 2 0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
	} {
		rr, err := dns.NewRR(s)
		if err != nil {
			t.Fatal(err)
		}
		m.Ns = append(m.Ns, rr)
	}
	m.SetEdns0(4096, true)
	b, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func types(rrs []dns.RR) []uint16 {
	var out []uint16
	for _, rr := range rrs {
		out = append(out, rr.Header().Rrtype)
	}
	return out
}

func TestReplyWithoutDOLeavesDNSSECOut(t *testing.T) {
	q := query("www.example.org.", dns.TypeA, false, false)
	r := reply(q, &Result{Packet: signed(t, dns.RcodeSuccess), Secure: true})
	if got := types(r.Answer); len(got) != 1 || got[0] != dns.TypeA {
		t.Errorf("answer: %v, want the A record alone", got)
	}
	if got := types(r.Ns); len(got) != 1 || got[0] != dns.TypeNS {
		t.Errorf("authority: %v, want the NS record alone", got)
	}
	if r.AuthenticatedData {
		t.Error("AD set for a client that asked with neither DO nor AD")
	}
	if r.IsEdns0() != nil {
		t.Error("an OPT for a client that sent none")
	}
	if r.Id != q.Id || r.Question[0] != q.Question[0] || !r.RecursionAvailable || !r.Response {
		t.Errorf("header or question not the client's: %v", r)
	}
}

func TestReplyWithDOKeepsEverything(t *testing.T) {
	q := query("www.example.org.", dns.TypeA, true, true)
	r := reply(q, &Result{Packet: signed(t, dns.RcodeSuccess), Secure: true})
	if len(r.Answer) != 2 || len(r.Ns) != 3 {
		t.Errorf("answer %v, authority %v: want everything", types(r.Answer), types(r.Ns))
	}
	if !r.AuthenticatedData {
		t.Error("no AD on a validated answer, asked with DO")
	}
	if opt := r.IsEdns0(); opt == nil || !opt.Do() || opt.UDPSize() != maxUDP {
		t.Errorf("OPT: %v", opt)
	}
	if n := len(r.Extra); n != 1 {
		t.Errorf("additional section: %d records, want the OPT alone (libunbound's own is left out)", n)
	}
}

func TestReplyADForAClientThatAskedWithAD(t *testing.T) {
	q := query("www.example.org.", dns.TypeA, false, false)
	q.AuthenticatedData = true
	if r := reply(q, &Result{Packet: signed(t, dns.RcodeSuccess), Secure: true}); !r.AuthenticatedData {
		t.Error("no AD for a client that asked with AD (RFC 6840)")
	}
	if r := reply(q, &Result{Packet: signed(t, dns.RcodeSuccess)}); r.AuthenticatedData {
		t.Error("AD on an answer that is not validated")
	}
}

func TestReplyAskedTypeKeptWithoutDO(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("example.org.", dns.TypeDS)
	rr, _ := dns.NewRR("example.org. 300 IN DS 12345 13 2 0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF")
	m.Answer = []dns.RR{rr}
	b, _ := m.Pack()
	r := reply(query("example.org.", dns.TypeDS, false, false), &Result{Packet: b})
	if len(r.Answer) != 1 {
		t.Errorf("a DS asked for is left out: %v", r.Answer)
	}
}

func TestReplyBogus(t *testing.T) {
	q := query("www.example.org.", dns.TypeA, true, true)
	res := &Result{Packet: signed(t, dns.RcodeSuccess), Bogus: true}
	if r := reply(q, res); r.Rcode != dns.RcodeServerFailure || len(r.Answer) != 0 {
		t.Errorf("bogus: %s with %d records, want SERVFAIL and nothing", dns.RcodeToString[r.Rcode], len(r.Answer))
	}
	q.CheckingDisabled = true
	r := reply(q, res)
	if r.Rcode != dns.RcodeSuccess || len(r.Answer) == 0 || r.AuthenticatedData || !r.CheckingDisabled {
		t.Errorf("bogus, checking disabled: %s, %d records, AD %v, CD %v; want the data, without AD",
			dns.RcodeToString[r.Rcode], len(r.Answer), r.AuthenticatedData, r.CheckingDisabled)
	}
}

func TestReplyFailures(t *testing.T) {
	q := query("www.example.org.", dns.TypeA, false, false)
	if r := reply(q, nil); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("no result: %s", dns.RcodeToString[r.Rcode])
	}
	// libunbound's packet for a failure carries no question.
	e := new(dns.Msg)
	e.Rcode = dns.RcodeServerFailure
	b, _ := e.Pack()
	if r := reply(q, &Result{Packet: b, Rcode: dns.RcodeServerFailure}); r.Rcode != dns.RcodeServerFailure || len(r.Question) != 1 {
		t.Errorf("a failure: %s, question %v", dns.RcodeToString[r.Rcode], r.Question)
	}
	if r := reply(q, &Result{Packet: []byte{1, 2, 3}}); r.Rcode != dns.RcodeServerFailure {
		t.Errorf("a packet that does not parse: %s", dns.RcodeToString[r.Rcode])
	}
	if r := reply(q, &Result{Packet: signed(t, dns.RcodeNameError), Rcode: dns.RcodeNameError}); r.Rcode != dns.RcodeNameError || len(r.Ns) == 0 {
		t.Errorf("NXDOMAIN: %s, authority %v", dns.RcodeToString[r.Rcode], r.Ns)
	}
}

func TestFit(t *testing.T) {
	m := new(dns.Msg)
	m.SetQuestion("big.example.org.", dns.TypeTXT)
	for i := 0; i < 40; i++ {
		rr, _ := dns.NewRR(`big.example.org. 300 IN TXT "0123456789012345678901234567890123456789012345678901234567890123456789"`)
		m.Answer = append(m.Answer, rr)
	}
	b, _ := m.Pack()
	for _, c := range []struct {
		edns bool
		size int
	}{{false, 512}, {true, maxUDP}} {
		q := query("big.example.org.", dns.TypeTXT, c.edns, false)
		r := reply(q, &Result{Packet: b})
		fit(q, r)
		packed, _ := r.Pack()
		if !r.Truncated || len(packed) > c.size {
			t.Errorf("EDNS %v: %d bytes, TC %v; want at most %d and TC", c.edns, len(packed), r.Truncated, c.size)
		}
	}
}

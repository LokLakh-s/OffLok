// Package front is OffLok's DNS front: it answers the system on the loopback, blocks what the lists block, and
// hands everything else, untouched (DNSSEC bits included), to Unbound on its internal port.
package front

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// blockedTTL is short: a stub resolver's cache (systemd-resolved's) keeps a blocked answer that long after a pause
// or "Trust this site".
const blockedTTL = 10

// Server is the front.
type Server struct {
	Listen   []netip.AddrPort
	Upstream netip.AddrPort
	// Blocked reports whether a name (lowercase, no final dot) is to be blocked.
	Blocked func(name string) bool

	mu      sync.Mutex
	servers []*dns.Server
}

// Start binds UDP and TCP on every listen address, or none.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.servers) > 0 {
		return errors.New("front already started")
	}
	var started []*dns.Server
	for _, a := range s.Listen {
		pc, err := net.ListenPacket("udp", a.String())
		if err != nil {
			stopAll(started)
			return err
		}
		l, err := net.Listen("tcp", a.String())
		if err != nil {
			pc.Close()
			stopAll(started)
			return err
		}
		for _, srv := range []*dns.Server{{PacketConn: pc, Handler: s}, {Listener: l, Handler: s}} {
			started = append(started, srv)
			go func(srv *dns.Server) { _ = srv.ActivateAndServe() }(srv)
		}
	}
	s.servers = started
	return nil
}

// Stop closes every socket.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	stopAll(s.servers)
	s.servers = nil
}

// Running reports whether the front listens.
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.servers) > 0
}

func stopAll(servers []*dns.Server) {
	for _, srv := range servers {
		_ = srv.Shutdown()
	}
}

// ServeDNS answers one query.
func (s *Server) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	if len(q.Question) == 1 && q.Question[0].Qclass == dns.ClassINET && s.Blocked != nil {
		name := strings.ToLower(strings.TrimSuffix(q.Question[0].Name, "."))
		if name != "" && s.Blocked(name) {
			_ = w.WriteMsg(blockedAnswer(q))
			return
		}
	}
	network := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		network = "tcp"
	}
	c := &dns.Client{Net: network, Timeout: 10 * time.Second}
	r, _, err := c.Exchange(q, s.Upstream.String())
	if err != nil {
		fail := new(dns.Msg)
		fail.SetRcode(q, dns.RcodeServerFailure)
		fail.RecursionAvailable = true
		_ = w.WriteMsg(fail)
		return
	}
	_ = w.WriteMsg(r)
}

// blockedAnswer is 0.0.0.0 for A, :: for AAAA, and no data for the other types: programs give up at once, where a
// timeout would make them wait.
func blockedAnswer(q *dns.Msg) *dns.Msg {
	r := new(dns.Msg)
	r.SetReply(q)
	r.RecursionAvailable = true
	hdr := dns.RR_Header{Name: q.Question[0].Name, Class: dns.ClassINET, Ttl: blockedTTL}
	switch q.Question[0].Qtype {
	case dns.TypeA:
		hdr.Rrtype = dns.TypeA
		r.Answer = append(r.Answer, &dns.A{Hdr: hdr, A: net.IPv4zero})
	case dns.TypeAAAA:
		hdr.Rrtype = dns.TypeAAAA
		r.Answer = append(r.Answer, &dns.AAAA{Hdr: hdr, AAAA: net.IPv6zero})
	}
	if opt := q.IsEdns0(); opt != nil {
		r.SetEdns0(1232, opt.Do())
	}
	return r
}

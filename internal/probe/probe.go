// Package probe asks DNS servers simple questions to learn what works from where the machine is.
package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/netip"
	"time"

	"github.com/miekg/dns"
)

const timeout = 2 * time.Second

func rootSOA(recursionDesired bool) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(".", dns.TypeSOA)
	m.RecursionDesired = recursionDesired
	m.SetEdns0(1232, true)
	return m
}

func hasRootSOA(r *dns.Msg) bool {
	for _, rr := range r.Answer {
		if soa, ok := rr.(*dns.SOA); ok && soa.Hdr.Name == "." {
			return true
		}
	}
	return false
}

// RootReachable reports whether at least one of three randomly picked root servers answers over UDP. The answer
// must be authoritative: some networks redirect every port-53 packet to their own resolver, which answers too,
// but without the AA flag.
func RootReachable(ctx context.Context, servers []netip.AddrPort) bool {
	picked := append([]netip.AddrPort(nil), servers...)
	rand.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
	if len(picked) > 3 {
		picked = picked[:3]
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ok := make(chan bool, len(picked))
	for _, s := range picked {
		go func(s netip.AddrPort) {
			c := &dns.Client{Net: "udp", Timeout: timeout}
			r, _, err := c.ExchangeContext(ctx, rootSOA(false), s.String())
			ok <- err == nil && r.Rcode == dns.RcodeSuccess && r.Authoritative && hasRootSOA(r)
		}(s)
	}
	for range picked {
		if <-ok {
			return true
		}
	}
	return false
}

// Alive checks that a resolver listens at addr. It asks for localhost, which Unbound answers by itself: no network
// needed.
func Alive(ctx context.Context, addr netip.AddrPort) error {
	m := new(dns.Msg)
	m.SetQuestion("localhost.", dns.TypeA)
	c := &dns.Client{Net: "udp", Timeout: 500 * time.Millisecond}
	r, _, err := c.ExchangeContext(ctx, m, addr.String())
	if err != nil {
		return err
	}
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("answer code %s", dns.RcodeToString[r.Rcode])
	}
	return nil
}

// Resolver checks that the resolver at addr answers the root zone's SOA. With requireAD the answer must also be
// DNSSEC-validated: the root zone is always signed, so a validating resolver always sets AD.
func Resolver(ctx context.Context, addr netip.AddrPort, requireAD bool) error {
	c := &dns.Client{Net: "udp", Timeout: timeout}
	r, _, err := c.ExchangeContext(ctx, rootSOA(true), addr.String())
	if err != nil {
		return err
	}
	return checkAnswer(r, requireAD)
}

// TLS checks that the DNS-over-TLS server at addr, whose certificate must carry name, answers.
func TLS(ctx context.Context, addr netip.AddrPort, name string) error {
	c := &dns.Client{Net: "tcp-tls", Timeout: timeout, TLSConfig: &tls.Config{ServerName: name, MinVersion: tls.VersionTLS12}}
	r, _, err := c.ExchangeContext(ctx, rootSOA(true), addr.String())
	if err != nil {
		return err
	}
	return checkAnswer(r, false)
}

func checkAnswer(r *dns.Msg, requireAD bool) error {
	if r.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("answer code %s", dns.RcodeToString[r.Rcode])
	}
	if !hasRootSOA(r) {
		return errors.New("no root SOA in the answer")
	}
	if requireAD && !r.AuthenticatedData {
		return errors.New("answer not DNSSEC-validated")
	}
	return nil
}

// Lookup returns the IPv4 and IPv6 addresses of name, asked to the resolver at addr.
func Lookup(ctx context.Context, addr netip.AddrPort, name string) ([]netip.Addr, error) {
	c := &dns.Client{Net: "udp", Timeout: timeout}
	var out []netip.Addr
	var lastErr error
	for _, t := range []uint16{dns.TypeA, dns.TypeAAAA} {
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(name), t)
		r, _, err := c.ExchangeContext(ctx, m, addr.String())
		if err != nil {
			lastErr = err
			continue
		}
		for _, rr := range r.Answer {
			switch v := rr.(type) {
			case *dns.A:
				if a, ok := netip.AddrFromSlice(v.A.To4()); ok {
					out = append(out, a)
				}
			case *dns.AAAA:
				if a, ok := netip.AddrFromSlice(v.AAAA); ok {
					out = append(out, a)
				}
			}
		}
	}
	if len(out) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("%s has no address", name)
		}
		return nil, lastErr
	}
	return out, nil
}

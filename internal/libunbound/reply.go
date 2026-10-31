// Package libunbound runs Unbound inside offlokd, as the library NLnet Labs builds with it, where Unbound cannot run
// as a program of its own (Android). It answers DNS on a loopback address as the unbound program does, so that the
// front, the probes and the self-test ask it the same way.
package libunbound

import (
	"github.com/miekg/dns"
)

// Result is libunbound's answer to one question.
type Result struct {
	// Packet is the answer as libunbound encodes it: asked with the DO bit (signatures included), AD set when
	// validated. It carries no question when Rcode is not NOERROR.
	Packet []byte
	Rcode  int
	Secure bool // DNSSEC-validated
	Bogus  bool // DNSSEC validation failed
}

// maxUDP is the largest answer over UDP, Unbound's default (edns-buffer-size, max-udp-size).
const maxUDP = 1232

// reply turns libunbound's answer into the one the unbound program would give the client that asked q (Unbound's
// mesh_send_reply and reply_info_answer_encode): a bogus answer is a SERVFAIL unless the client turned checking off
// (CD); AD only for a validated answer, to a client that asked with DO or AD; no DNSSEC records for a client that
// did not ask with DO, unless it asked for their type.
func reply(q *dns.Msg, res *Result) *dns.Msg {
	r := new(dns.Msg)
	r.SetReply(q)
	r.RecursionAvailable = true
	r.CheckingDisabled = q.CheckingDisabled
	opt := q.IsEdns0()
	do := opt != nil && opt.Do()
	var a dns.Msg
	switch {
	case res == nil, res.Bogus && !q.CheckingDisabled:
		r.Rcode = dns.RcodeServerFailure
	case res.Packet != nil && a.Unpack(res.Packet) != nil:
		r.Rcode = dns.RcodeServerFailure
	default:
		qtype := q.Question[0].Qtype
		r.Rcode = res.Rcode
		r.Answer = keep(a.Answer, qtype, do, true)
		r.Ns = keep(a.Ns, qtype, do, false)
		r.Extra = keep(a.Extra, qtype, do, false)
		r.AuthenticatedData = res.Secure && !res.Bogus && (do || q.AuthenticatedData)
	}
	if opt != nil {
		r.SetEdns0(maxUDP, do)
	}
	return r
}

// keep returns the records of one section the client gets.
func keep(rrs []dns.RR, qtype uint16, do, answer bool) []dns.RR {
	var out []dns.RR
	for _, rr := range rrs {
		t := rr.Header().Rrtype
		switch {
		case t == dns.TypeOPT: // the client's own OPT is added back
		case do, answer && (qtype == dns.TypeANY || qtype == t), !dnssecType[t]:
			out = append(out, rr)
		}
	}
	return out
}

// dnssecType are the types Unbound leaves out for a client that did not ask with DO (rrset_belongs_in_reply).
var dnssecType = map[uint16]bool{dns.TypeSIG: true, dns.TypeKEY: true, dns.TypeNXT: true, dns.TypeDS: true,
	dns.TypeRRSIG: true, dns.TypeNSEC: true, dns.TypeDNSKEY: true, dns.TypeNSEC3: true, dns.TypeNSEC3PARAM: true}

// fit truncates an answer over UDP to what the client takes: 512 bytes without EDNS, its own size up to maxUDP
// with it.
func fit(q, r *dns.Msg) {
	size := dns.MinMsgSize
	if opt := q.IsEdns0(); opt != nil {
		size = max(dns.MinMsgSize, min(int(opt.UDPSize()), maxUDP))
	}
	r.Truncate(size)
}

// Package tun answers the DNS queries an Android VPN tunnel carries to OffLok's address. The tunnel's only route
// is that address: every packet read is meant for it. UDP to port 53 goes to a DNS handler (OffLok's front) and the
// answer goes back as a packet; TCP gets a reset, so that the phone gives up on TCP at once (Android's DNS over TCP,
// and the port 853 its "Automatic" Private DNS tries); anything else is dropped.
package tun

import (
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"slices"

	"github.com/miekg/dns"
)

const (
	protoTCP = 6
	protoUDP = 17
	// MTU is the tunnel's: a UDP answer of any size fits in one packet, so answers are never truncated.
	MTU = 65535
)

// Tunnel is one VPN tunnel's packets.
type Tunnel struct {
	Dev     io.ReadWriter // one packet per Read and per Write
	Addrs   []netip.Addr  // OffLok's addresses in the tunnel, IPv4 and IPv6
	Handler dns.Handler
}

// Serve answers until Dev fails (closed: the tunnel is down).
func (t *Tunnel) Serve() error {
	buf := make([]byte, MTU)
	for {
		n, err := t.Dev.Read(buf)
		if err != nil {
			return err
		}
		p, ok := parse(buf[:n])
		if !ok || !slices.Contains(t.Addrs, p.dst) {
			continue
		}
		switch {
		case p.proto == protoUDP && p.dstPort == 53:
			go t.answer(p, slices.Clone(p.payload))
		case p.proto == protoTCP:
			if r := reset(p); r != nil {
				_, _ = t.Dev.Write(r)
			}
		}
	}
}

// answer hands one query to the handler and writes its answer back. A truncated answer is asked again as over TCP,
// and goes back whole.
func (t *Tunnel) answer(p packet, payload []byte) {
	q := new(dns.Msg)
	if err := q.Unpack(payload); err != nil || q.Response {
		return
	}
	w := &writer{local: p.dst, remote: p.src, localPort: p.dstPort, remotePort: p.srcPort}
	t.Handler.ServeDNS(w, q)
	if w.reply != nil && w.reply.Truncated {
		full := &writer{local: p.dst, remote: p.src, localPort: p.dstPort, remotePort: p.srcPort, tcp: true}
		t.Handler.ServeDNS(full, q)
		if full.reply != nil && !full.reply.Truncated {
			w.reply = full.reply
		}
	}
	if w.reply == nil {
		return
	}
	b, err := w.reply.Pack()
	if err != nil || len(b) > MTU-48 {
		return
	}
	_, _ = t.Dev.Write(udpPacket(p.dst, p.src, p.dstPort, p.srcPort, b))
}

// packet is what Serve reads of one packet.
type packet struct {
	src, dst         netip.Addr
	proto            uint8
	srcPort, dstPort uint16
	payload          []byte // UDP: the datagram's payload; TCP: the whole segment
}

// parse reads an IPv4 or IPv6 packet carrying UDP or TCP, whole (no fragment, no IPv6 extension header).
func parse(b []byte) (packet, bool) {
	var p packet
	if len(b) < 1 {
		return p, false
	}
	var l4 []byte
	switch b[0] >> 4 {
	case 4:
		if len(b) < 20 {
			return p, false
		}
		ihl := int(b[0]&0x0f) * 4
		total := int(binary.BigEndian.Uint16(b[2:4]))
		if ihl < 20 || total < ihl || total > len(b) || binary.BigEndian.Uint16(b[6:8])&0x3fff != 0 {
			return p, false // fragments are left out: no DNS query needs one
		}
		p.proto = b[9]
		p.src, p.dst = netip.AddrFrom4([4]byte(b[12:16])), netip.AddrFrom4([4]byte(b[16:20]))
		l4 = b[ihl:total]
	case 6:
		if len(b) < 40 {
			return p, false
		}
		plen := int(binary.BigEndian.Uint16(b[4:6]))
		if 40+plen > len(b) {
			return p, false
		}
		p.proto = b[6]
		p.src, p.dst = netip.AddrFrom16([16]byte(b[8:24])), netip.AddrFrom16([16]byte(b[24:40]))
		l4 = b[40 : 40+plen]
	default:
		return p, false
	}
	switch p.proto {
	case protoUDP:
		if len(l4) < 8 {
			return p, false
		}
		ulen := int(binary.BigEndian.Uint16(l4[4:6]))
		if ulen < 8 || ulen > len(l4) {
			return p, false
		}
		p.payload = l4[8:ulen]
	case protoTCP:
		if len(l4) < 20 {
			return p, false
		}
		p.payload = l4
	default:
		return p, false
	}
	p.srcPort, p.dstPort = binary.BigEndian.Uint16(l4[0:2]), binary.BigEndian.Uint16(l4[2:4])
	return p, true
}

// ipHeader is an IPv4 or IPv6 header for a packet from src to dst carrying n bytes of proto.
func ipHeader(src, dst netip.Addr, proto uint8, n int) []byte {
	if src.Is4() {
		h := make([]byte, 20, 20+n)
		h[0] = 0x45
		binary.BigEndian.PutUint16(h[2:4], uint16(20+n))
		binary.BigEndian.PutUint16(h[6:8], 0x4000) // don't fragment
		h[8] = 64
		h[9] = proto
		s, d := src.As4(), dst.As4()
		copy(h[12:16], s[:])
		copy(h[16:20], d[:])
		binary.BigEndian.PutUint16(h[10:12], ^fold(sum(0, h)))
		return h
	}
	h := make([]byte, 40, 40+n)
	h[0] = 0x60
	binary.BigEndian.PutUint16(h[4:6], uint16(n))
	h[6] = proto
	h[7] = 64
	s, d := src.As16(), dst.As16()
	copy(h[8:24], s[:])
	copy(h[24:40], d[:])
	return h
}

// udpPacket is a whole IP packet carrying payload from src:sport to dst:dport.
func udpPacket(src, dst netip.Addr, sport, dport uint16, payload []byte) []byte {
	seg := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint16(seg[0:2], sport)
	binary.BigEndian.PutUint16(seg[2:4], dport)
	binary.BigEndian.PutUint16(seg[4:6], uint16(len(seg)))
	copy(seg[8:], payload)
	c := ^fold(sum(pseudo(src, dst, protoUDP, len(seg)), seg))
	if c == 0 {
		c = 0xffff // zero means "no checksum" in UDP
	}
	binary.BigEndian.PutUint16(seg[6:8], c)
	return append(ipHeader(src, dst, protoUDP, len(seg)), seg...)
}

// reset answers a TCP segment to OffLok's address with a reset (RFC 9293, 3.10.7.1): none to a reset.
func reset(p packet) []byte {
	in := p.payload
	flags := in[13]
	const fin, syn, rst, ack = 0x01, 0x02, 0x04, 0x10
	if flags&rst != 0 {
		return nil
	}
	seg := make([]byte, 20)
	binary.BigEndian.PutUint16(seg[0:2], p.dstPort)
	binary.BigEndian.PutUint16(seg[2:4], p.srcPort)
	if flags&ack != 0 {
		copy(seg[4:8], in[8:12]) // seq = their ack
		seg[13] = rst
	} else {
		dataOff := int(in[12]>>4) * 4
		n := uint32(max(0, len(in)-dataOff))
		if flags&syn != 0 {
			n++
		}
		if flags&fin != 0 {
			n++
		}
		binary.BigEndian.PutUint32(seg[8:12], binary.BigEndian.Uint32(in[4:8])+n)
		seg[13] = rst | ack
	}
	seg[12] = 5 << 4
	binary.BigEndian.PutUint16(seg[16:18], ^fold(sum(pseudo(p.dst, p.src, protoTCP, len(seg)), seg)))
	return append(ipHeader(p.dst, p.src, protoTCP, len(seg)), seg...)
}

// pseudo is the sum of the pseudo-header UDP and TCP checksums cover.
func pseudo(src, dst netip.Addr, proto uint8, n int) uint32 {
	var s uint32
	s = sum(s, src.AsSlice())
	s = sum(s, dst.AsSlice())
	return s + uint32(proto) + uint32(n)
}

// sum adds b to s as 16-bit big-endian words, the Internet checksum's way (RFC 1071).
func sum(s uint32, b []byte) uint32 {
	for len(b) >= 2 {
		s += uint32(b[0])<<8 | uint32(b[1])
		b = b[2:]
	}
	if len(b) == 1 {
		s += uint32(b[0]) << 8
	}
	return s
}

func fold(s uint32) uint16 {
	for s > 0xffff {
		s = s>>16 + s&0xffff
	}
	return uint16(s)
}

// writer is the handler's dns.ResponseWriter: it keeps the answer for answer to write.
type writer struct {
	local, remote         netip.Addr
	localPort, remotePort uint16
	tcp                   bool // the handler asks its upstream over TCP: no truncation
	reply                 *dns.Msg
}

func (w *writer) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: w.local.AsSlice(), Port: int(w.localPort)}
}

// RemoteAddr is a TCP address when the answer must come whole: the front then asks Unbound over TCP.
func (w *writer) RemoteAddr() net.Addr {
	if w.tcp {
		return &net.TCPAddr{IP: w.remote.AsSlice(), Port: int(w.remotePort)}
	}
	return &net.UDPAddr{IP: w.remote.AsSlice(), Port: int(w.remotePort)}
}

func (w *writer) WriteMsg(m *dns.Msg) error {
	w.reply = m
	return nil
}

func (w *writer) Write(b []byte) (int, error) {
	m := new(dns.Msg)
	if err := m.Unpack(b); err != nil {
		return 0, err
	}
	w.reply = m
	return len(b), nil
}

func (w *writer) Close() error        { return nil }
func (w *writer) TsigStatus() error   { return nil }
func (w *writer) TsigTimersOnly(bool) {}
func (w *writer) Hijack()             {}

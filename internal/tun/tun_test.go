package tun

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// device is a tunnel in memory: the test writes the packets the phone sends, and reads OffLok's.
type device struct {
	in  chan []byte
	out chan []byte
}

func newDevice() *device { return &device{in: make(chan []byte, 8), out: make(chan []byte, 8)} }

func (d *device) Read(b []byte) (int, error) {
	p, ok := <-d.in
	if !ok {
		return 0, io.EOF
	}
	return copy(b, p), nil
}

func (d *device) Write(b []byte) (int, error) {
	d.out <- append([]byte(nil), b...)
	return len(b), nil
}

func (d *device) next(t *testing.T) []byte {
	t.Helper()
	select {
	case p := <-d.out:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("no packet from the tunnel")
		return nil
	}
}

func (d *device) none(t *testing.T) {
	t.Helper()
	select {
	case p := <-d.out:
		t.Fatalf("a packet nobody asked for: %x", p)
	case <-time.After(100 * time.Millisecond):
	}
}

var (
	dns4  = netip.MustParseAddr("10.111.222.1")
	dns6  = netip.MustParseAddr("fd00:6f:6c6b::1")
	phone = netip.MustParseAddr("10.111.222.2")
	ph6   = netip.MustParseAddr("fd00:6f:6c6b::2")
)

// handler answers every A query with 192.0.2.1, records what it was asked, and answers "big." with more than fits
// in 512 bytes, truncated unless asked as over TCP.
type handler struct {
	remotes chan net.Addr
}

func (h *handler) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	h.remotes <- w.RemoteAddr()
	r := new(dns.Msg)
	r.SetReply(q)
	n := 1
	if q.Question[0].Name == "big." {
		n = 60
	}
	for i := 0; i < n; i++ {
		r.Answer = append(r.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.IPv4(192, 0, 2, byte(i+1))})
	}
	if _, tcp := w.RemoteAddr().(*net.TCPAddr); !tcp {
		r.Truncate(512)
	}
	_ = w.WriteMsg(r)
}

func serve(t *testing.T) (*device, *handler) {
	t.Helper()
	d, h := newDevice(), &handler{remotes: make(chan net.Addr, 4)}
	tn := &Tunnel{Dev: d, Addrs: []netip.Addr{dns4, dns6}, Handler: h}
	done := make(chan error)
	go func() { done <- tn.Serve() }()
	t.Cleanup(func() {
		close(d.in)
		if err := <-done; !errors.Is(err, io.EOF) {
			t.Errorf("Serve: %v", err)
		}
	})
	return d, h
}

func queryPacket(t *testing.T, src, dst netip.Addr, name string) []byte {
	t.Helper()
	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)
	b, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return udpPacket(src, dst, 40000, 53, b)
}

// verify checks a packet's checksums, independently of the code that wrote them, and returns it parsed.
func verify(t *testing.T, b []byte) packet {
	t.Helper()
	p, ok := parse(b)
	if !ok {
		t.Fatalf("does not parse: %x", b)
	}
	l4 := b[40:]
	if p.src.Is4() {
		if fold(sum(0, b[:20])) != 0xffff {
			t.Error("bad IPv4 header checksum")
		}
		l4 = b[20:]
	}
	if fold(sum(pseudo(p.src, p.dst, p.proto, len(l4)), l4)) != 0xffff {
		t.Errorf("bad %d checksum", p.proto)
	}
	return p
}

func TestAnswersUDP(t *testing.T) {
	for _, c := range []struct{ phone, offlok netip.Addr }{{phone, dns4}, {ph6, dns6}} {
		d, h := serve(t)
		d.in <- queryPacket(t, c.phone, c.offlok, "example.org.")
		p := verify(t, d.next(t))
		if p.src != c.offlok || p.dst != c.phone || p.proto != protoUDP || p.srcPort != 53 || p.dstPort != 40000 {
			t.Errorf("%v: answer from %v:%d to %v:%d", c.offlok, p.src, p.srcPort, p.dst, p.dstPort)
		}
		var r dns.Msg
		if err := r.Unpack(p.payload); err != nil || len(r.Answer) != 1 || !r.Response {
			t.Errorf("%v: answer %v (%v)", c.offlok, r, err)
		}
		if a, ok := (<-h.remotes).(*net.UDPAddr); !ok || !a.IP.Equal(c.phone.AsSlice()) || a.Port != 40000 {
			t.Errorf("%v: the handler saw the query from %v", c.offlok, a)
		}
	}
}

func TestTruncatedAnswerComesWhole(t *testing.T) {
	d, h := serve(t)
	d.in <- queryPacket(t, phone, dns4, "big.")
	p := verify(t, d.next(t))
	var r dns.Msg
	if err := r.Unpack(p.payload); err != nil || r.Truncated || len(r.Answer) != 60 {
		t.Errorf("TC %v, %d records (%v); want all 60", r.Truncated, len(r.Answer), err)
	}
	if _, ok := (<-h.remotes).(*net.UDPAddr); !ok {
		t.Error("first asked as over UDP")
	}
	if _, ok := (<-h.remotes).(*net.TCPAddr); !ok {
		t.Error("then as over TCP")
	}
}

func TestIgnoresOtherPackets(t *testing.T) {
	d, _ := serve(t)
	d.in <- queryPacket(t, phone, netip.MustParseAddr("192.0.2.53"), "example.org.") // not OffLok's address
	q := queryPacket(t, phone, dns4, "example.org.")
	binary.BigEndian.PutUint16(q[6:8], 0x2000) // a fragment
	d.in <- q
	short := queryPacket(t, phone, dns4, "example.org.")
	d.in <- short[:len(short)-5] // shorter than its header says
	d.in <- udpPacket(phone, dns4, 40000, 54, []byte("not DNS"))
	d.in <- udpPacket(phone, dns4, 40000, 53, []byte("not DNS"))
	d.in <- []byte{0x45}
	d.none(t)
}

func tcpSegment(src, dst netip.Addr, seq, ack uint32, flags byte, data []byte) []byte {
	seg := make([]byte, 20+len(data))
	binary.BigEndian.PutUint16(seg[0:2], 40001)
	binary.BigEndian.PutUint16(seg[2:4], 853)
	binary.BigEndian.PutUint32(seg[4:8], seq)
	binary.BigEndian.PutUint32(seg[8:12], ack)
	seg[12] = 5 << 4
	seg[13] = flags
	copy(seg[20:], data)
	binary.BigEndian.PutUint16(seg[16:18], ^fold(sum(pseudo(src, dst, protoTCP, len(seg)), seg)))
	return append(ipHeader(src, dst, protoTCP, len(seg)), seg...)
}

func TestTCPGetsAReset(t *testing.T) {
	d, _ := serve(t)
	d.in <- tcpSegment(ph6, dns6, 1000, 0, 0x02, nil) // SYN
	p := verify(t, d.next(t))
	seg := p.payload
	if p.src != dns6 || p.dst != ph6 || p.srcPort != 853 || p.dstPort != 40001 {
		t.Errorf("reset from %v:%d to %v:%d", p.src, p.srcPort, p.dst, p.dstPort)
	}
	if seg[13] != 0x14 || binary.BigEndian.Uint32(seg[8:12]) != 1001 {
		t.Errorf("SYN: flags %#x, ack %d; want RST+ACK, 1001", seg[13], binary.BigEndian.Uint32(seg[8:12]))
	}
	d.in <- tcpSegment(phone, dns4, 5000, 777, 0x18, []byte("data")) // PSH+ACK
	seg = verify(t, d.next(t)).payload
	if seg[13] != 0x04 || binary.BigEndian.Uint32(seg[4:8]) != 777 {
		t.Errorf("ACK: flags %#x, seq %d; want RST, 777", seg[13], binary.BigEndian.Uint32(seg[4:8]))
	}
	d.in <- tcpSegment(phone, dns4, 5000, 0, 0x04, nil) // RST: no answer
	d.none(t)
}

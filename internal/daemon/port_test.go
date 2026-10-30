package daemon

import (
	"net"
	"testing"
)

// freePort returns a loopback port free for both UDP and TCP: Windows may reserve one and not the other.
func freePort(t *testing.T) uint16 {
	t.Helper()
	for range 100 {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := pc.LocalAddr().(*net.UDPAddr).Port
		l, err := net.Listen("tcp", pc.LocalAddr().String())
		pc.Close()
		if err == nil {
			l.Close()
			return uint16(port)
		}
	}
	t.Fatal("no loopback port free for both UDP and TCP")
	return 0
}

// A port another program holds (or Windows reserved) makes Unbound take a spare one; the usual one comes first.
func TestPickUnboundPort(t *testing.T) {
	taken, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busy, spare := uint16(taken.LocalAddr().(*net.UDPAddr).Port), freePort(t)

	d := testDaemon(t)
	d.UnboundPort, d.UnboundSpare = busy, []uint16{busy, spare}
	if got := d.pickUnboundPort(); got != spare {
		t.Fatalf("usual port taken: got %d, want the spare %d", got, spare)
	}
	if d.ubPort = d.pickUnboundPort(); d.unboundAddr().Port() != spare {
		t.Fatalf("Unbound's address: %v", d.unboundAddr())
	}
	d.UnboundPort = freePort(t)
	if got := d.pickUnboundPort(); got != d.UnboundPort {
		t.Fatalf("usual port free: got %d, want %d", got, d.UnboundPort)
	}
	d.UnboundPort, d.UnboundSpare = busy, nil
	if got := d.pickUnboundPort(); got != busy {
		t.Fatalf("nothing free: got %d, want the usual %d (reported as taken)", got, busy)
	}
}

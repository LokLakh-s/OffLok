package mobile

import (
	"errors"
	"log"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"golang.org/x/sys/unix"

	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/tun"
)

// OffLok's addresses in its tunnel: the DNS servers the VPN gives the phone, and the tunnel's only routes. Made up,
// in ranges no network routes to the internet (fd4f:666c:6f6b is "Oflok" in ASCII).
var tunnelAddrs = []netip.Addr{netip.MustParseAddr("10.111.222.1"), netip.MustParseAddr("fd4f:666c:6f6b::1")}

// Warnings the window explains (daemon.Status.Warning).
const (
	warnPrivateDNS = "android-private-dns" // a Private DNS hostname sends every query around OffLok
	warnOtherVPN   = "android-other-vpn"   // another VPN app took OffLok's place: OffLok turned off
)

var errOtherVPN = errors.New("another VPN app took OffLok's place")

// Platform is what the app does for the core. The app implements it.
type Platform interface {
	// OpenTunnel starts the VPN whose DNS servers, and only routes, are 10.111.222.1 and fd4f:666c:6f6b::1, the app
	// itself left out of it, and returns the tunnel's file descriptor, detached: the core reads it and closes it.
	OpenTunnel() (int64, error)
	// TunnelClosed says the core closed the tunnel, or Android did: the VPN service may end.
	TunnelClosed()
	// Changed hands over the core's status (JSON) whenever its state changes: the notification and the Quick
	// Settings tile follow it.
	Changed(status string)
	// Log is one line of the core's log, for Android's.
	Log(line string)
}

// backend is Android's "system DNS": the VPN tunnel, whose DNS servers are OffLok's addresses. The app tells it
// the network's DNS (ConnectivityManager), which the phone's sandbox hides from the core.
type backend struct {
	platform Platform
	answer   func(dns.ResponseWriter, *dns.Msg) // the daemon's front
	log      *log.Logger
	lost     func() // the tunnel went away by itself: OffLok turns off

	mu         sync.Mutex
	network    sysdns.Network
	id         string // the network's identity, from the app
	privateDNS string // the network's Private DNS hostname, when one is set
	otherVPN   bool   // set when the tunnel went away by itself, until OffLok is turned on again
	tunnel     *os.File
	serving    chan struct{} // closed when the tunnel's reader ends
}

var _ interface {
	sysdns.Backend
	sysdns.Fingerprinter
	sysdns.Warner
} = (*backend)(nil)

func (b *backend) Name() string { return "android" }

// Apply opens the tunnel, once.
func (b *backend) Apply([]netip.Addr) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.otherVPN {
		return errOtherVPN
	}
	if b.tunnel != nil {
		return nil
	}
	fd, err := b.platform.OpenTunnel()
	if err != nil {
		return err
	}
	// Non-blocking, the tunnel's reads wait in Go's poller: closing the file ends them.
	if err := unix.SetNonblock(int(fd), true); err != nil {
		unix.Close(int(fd))
		return err
	}
	f := os.NewFile(uintptr(fd), "tunnel")
	t := &tun.Tunnel{Dev: f, Addrs: tunnelAddrs, Handler: dns.HandlerFunc(b.answer)}
	done := make(chan struct{})
	go func() {
		err := t.Serve()
		b.mu.Lock()
		closed := b.tunnel != f
		b.mu.Unlock()
		if !closed {
			b.log.Printf("the tunnel went away: %v", err)
			b.tunnelLost(f)
		}
		close(done)
	}()
	b.tunnel, b.serving = f, done
	b.log.Print("tunnel open")
	return nil
}

// tunnelLost handles a tunnel Android closed: another VPN app took its place (Android runs one at a time), or the
// user revoked OffLok's VPN. OffLok turns off rather than take the VPN back.
func (b *backend) tunnelLost(f *os.File) {
	b.mu.Lock()
	if b.tunnel != f {
		b.mu.Unlock()
		return
	}
	b.tunnel, b.otherVPN = nil, true
	b.mu.Unlock()
	f.Close()
	b.platform.TunnelClosed()
	if b.lost != nil {
		b.lost()
	}
}

// revoked is the app's word that Android took the VPN from OffLok (VpnService.onRevoke).
func (b *backend) revoked() {
	b.mu.Lock()
	f := b.tunnel
	b.mu.Unlock()
	if f != nil {
		b.tunnelLost(f)
	}
}

// turningOn clears what a lost tunnel left: the user wants OffLok on.
func (b *backend) turningOn() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.otherVPN = false
}

// Applied reports whether the tunnel is open and read.
func (b *backend) Applied([]netip.Addr) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tunnel == nil {
		return false, nil
	}
	select {
	case <-b.serving:
		return false, nil
	default:
		return true, nil
	}
}

// Restore closes the tunnel: the phone has its network's DNS back.
func (b *backend) Restore() error {
	b.mu.Lock()
	f, done := b.tunnel, b.serving
	b.tunnel = nil
	b.mu.Unlock()
	if f == nil {
		return nil
	}
	f.Close()
	<-done
	b.log.Print("tunnel closed")
	b.platform.TunnelClosed()
	return nil
}

func (b *backend) Network() (sysdns.Network, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.network, nil
}

func (b *backend) Fingerprint() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.id
}

func (b *backend) Warning() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.otherVPN:
		return warnOtherVPN
	case b.privateDNS != "":
		return warnPrivateDNS
	}
	return ""
}

// setNetwork is what the app knows of the network the phone uses.
func (b *backend) setNetwork(id string, n sysdns.Network, privateDNS string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.id, b.network, b.privateDNS = id, n, privateDNS
}

// networkOf reads the network's DNS servers and search domains as the app sends them, separated by spaces or
// lines.
func networkOf(servers, domains string) sysdns.Network {
	var n sysdns.Network
	for _, s := range strings.Fields(servers) {
		if a, err := netip.ParseAddr(s); err == nil {
			n.Servers = append(n.Servers, a)
		}
	}
	n.Domains = strings.Fields(domains)
	return n
}

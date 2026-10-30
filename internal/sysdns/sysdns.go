// Package sysdns points the operating system's resolver at OffLok, and puts it back exactly as it was.
package sysdns

import (
	"bufio"
	"net/netip"
	"slices"
	"strings"
)

// Network is what the network (DHCP, router advertisements, a VPN) provides for DNS.
type Network struct {
	Servers []netip.Addr
	Domains []string // search and routing domains
}

// Equal reports whether n and o list the same servers and domains in the same order.
func (n Network) Equal(o Network) bool {
	return slices.Equal(n.Servers, o.Servers) && slices.Equal(n.Domains, o.Domains)
}

// Backend is one way of setting the system DNS.
type Backend interface {
	Name() string
	// Apply points the system at servers, OffLok's loopback addresses, saving the original settings first.
	Apply(servers []netip.Addr) error
	// Applied reports whether the system still points at servers only.
	Applied(servers []netip.Addr) (bool, error)
	// Restore puts back the settings saved by Apply. It does nothing when nothing was applied.
	Restore() error
	// Network returns the DNS servers and domains the network provides, never OffLok's own addresses.
	Network() (Network, error)
}

// Flusher is a backend whose resolver keeps a cache of its own (systemd-resolved): emptied when the blocking
// changes, so that a site just trusted answers at once.
type Flusher interface {
	FlushCaches() error
}

// ParseResolvConf reads the nameserver and search lines of a resolv.conf.
func ParseResolvConf(content string) Network {
	var n Network
	s := bufio.NewScanner(strings.NewReader(content))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) < 2 || strings.HasPrefix(f[0], "#") || strings.HasPrefix(f[0], ";") {
			continue
		}
		switch f[0] {
		case "nameserver":
			// A zone suffix (fe80::1%eth0) is valid in resolv.conf; netip keeps it.
			if a, err := netip.ParseAddr(f[1]); err == nil {
				n.Servers = append(n.Servers, a)
			}
		case "search", "domain":
			n.Domains = nil
			for _, d := range f[1:] {
				if d != "." {
					n.Domains = append(n.Domains, d)
				}
			}
		}
	}
	return n
}

// WithoutLoopback drops loopback servers: they are OffLok itself or another local resolver, never the network's.
func (n Network) WithoutLoopback() Network {
	out := Network{Domains: n.Domains}
	for _, a := range n.Servers {
		if !a.IsLoopback() && !a.IsUnspecified() {
			out.Servers = append(out.Servers, a)
		}
	}
	return out
}

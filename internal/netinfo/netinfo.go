// Package netinfo reads the machine's network interfaces.
package netinfo

import (
	"net"
	"net/netip"
	"slices"
	"strings"
)

type iface struct {
	name  string
	up    bool
	loop  bool
	addrs []netip.Addr
}

func interfaces() []iface {
	list, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []iface
	for _, i := range list {
		it := iface{name: i.Name, up: i.Flags&net.FlagUp != 0, loop: i.Flags&net.FlagLoopback != 0}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if p, err := netip.ParsePrefix(a.String()); err == nil {
				it.addrs = append(it.addrs, p.Addr())
			}
		}
		out = append(out, it)
	}
	return out
}

// Loopback returns the addresses OffLok listens on: 127.0.0.1, and ::1 when IPv6 is enabled on the loopback.
func Loopback() []netip.Addr {
	out := []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	for _, i := range interfaces() {
		if i.loop && slices.Contains(i.addrs, netip.IPv6Loopback()) {
			return append(out, netip.IPv6Loopback())
		}
	}
	return out
}

// HasIPv6Route reports whether the machine has a route to the IPv6 internet. Connecting a UDP socket sends
// nothing: it only asks the kernel for a route. A unique local address behind NAT66 (Docker does this) counts too,
// so the root probe and Unbound always use the same address families.
func HasIPv6Route() bool {
	c, err := net.Dial("udp6", "[2001:503:ba3e::2:30]:53") // a.root-servers.net
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Fingerprint changes whenever an interface goes up or down or its addresses change: the sign of a new network.
func Fingerprint() string {
	var parts []string
	for _, i := range interfaces() {
		if !i.up || i.loop {
			continue
		}
		s := make([]string, len(i.addrs))
		for k, a := range i.addrs {
			s[k] = a.String()
		}
		slices.Sort(s)
		parts = append(parts, i.name+"="+strings.Join(s, ","))
	}
	slices.Sort(parts)
	return strings.Join(parts, ";")
}

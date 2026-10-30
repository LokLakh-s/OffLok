package daemon

import (
	"net/netip"

	"github.com/loklakh-s/offlok/internal/settings"
	"github.com/loklakh-s/offlok/internal/sysdns"
	"github.com/loklakh-s/offlok/internal/unbound"
)

// State is what OffLok is doing, as the status shows it.
type State string

const (
	Starting        State = "starting"
	Direct          State = "direct"           // recursing from the root servers
	FallbackDoT     State = "fallback-dot"     // root out of reach: the personal DNS-over-TLS resolver
	FallbackNetwork State = "fallback-network" // root out of reach: the network's DNS (reduced protection)
	StrictBlocked   State = "strict-blocked"   // root out of reach and strict mode: no public names
	Offline         State = "offline"          // no root, no network DNS: answers from the cache only
	Off             State = "off"              // turned off: the system DNS is the original one
	Failed          State = "error"            // something broke: the system DNS was given back (not in strict mode)
)

type situation struct {
	settings settings.Settings
	rootOK   bool
	dotOK    bool
	signin   bool // strict mode lets the network's DNS through for a few minutes, to sign in to a captive portal
	network  sysdns.Network
}

type decision struct {
	state      State
	mode       unbound.Mode
	forwarders []unbound.Forwarder
}

// decide picks where answers come from. Recursion stays configured whenever no fallback applies, so that Unbound
// answers again by itself as soon as the root is back.
func decide(s situation) decision {
	switch {
	case s.rootOK:
		return decision{state: Direct, mode: unbound.Recursive}
	case s.settings.DoT != nil && len(s.settings.DoT.Addrs) > 0 && s.dotOK:
		d := decision{state: FallbackDoT, mode: unbound.ForwardTLS}
		for _, a := range s.settings.DoT.Addrs {
			d.forwarders = append(d.forwarders, unbound.Forwarder{Addr: netip.AddrPortFrom(a, s.settings.DoT.Port), TLSName: s.settings.DoT.Name})
		}
		return d
	case s.settings.Strict && !s.signin:
		return decision{state: StrictBlocked, mode: unbound.Recursive}
	case len(s.network.Servers) > 0:
		d := decision{state: FallbackNetwork, mode: unbound.ForwardNetwork}
		for _, a := range s.network.Servers {
			d.forwarders = append(d.forwarders, unbound.Forwarder{Addr: netip.AddrPortFrom(a, 53)})
		}
		return d
	default:
		return decision{state: Offline, mode: unbound.Recursive}
	}
}

// usable keeps the network servers Unbound can be pointed at: link-local addresses with a zone (fe80::1%eth0) are
// left out, Unbound's forward-addr takes no interface scope.
func usable(n sysdns.Network) sysdns.Network {
	out := sysdns.Network{Domains: n.Domains}
	for _, a := range n.Servers {
		if a.Zone() == "" {
			out.Servers = append(out.Servers, a.Unmap())
		}
	}
	return out
}

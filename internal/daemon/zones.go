package daemon

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/loklakh-s/offlok/internal/probe"
	"github.com/loklakh-s/offlok/internal/unbound"
)

// zoneVerdict is what the public DNS says of a domain the network declares (a search domain from DHCP, a
// connection's DNS suffix). Such a domain goes to the network's DNS with DNSSEC validation off for it, so the
// network must not be able to claim a public, signed one: a café's Wi-Fi declaring "com" would answer every .com
// name itself, unchecked.
type zoneVerdict int

const (
	zoneUnknown zoneVerdict = iota // not asked yet, or no answer: left to the network only in fallback
	zoneLocal                      // unsigned in the public DNS, or absent from it: the network's to answer
	zoneSigned                     // signed in the public DNS (a TLD, a bank's domain...): never the network's
	zoneInvalid                    // not a host name (a quote, a newline): never in Unbound's configuration
)

// zonesPerStep bounds the questions one step asks, each up to 2 seconds.
const zonesPerStep = 4

// builtinZone reports whether name is at or under one of the private zones OffLok forwards anyway.
func builtinZone(name string) bool {
	for _, z := range unbound.PrivateZones() {
		if name == z || strings.HasSuffix(name, "."+z) {
			return true
		}
	}
	return false
}

// localDomains are the network's domains Unbound forwards to the network's DNS: the private zones, those the
// public DNS showed unsigned, and, while every name goes to the network's DNS anyway, those not checked yet.
func (d *Daemon) localDomains(mode unbound.Mode) []string {
	var out []string
	for _, n := range d.network.Domains {
		name, err := unbound.CanonicalName(n)
		if err != nil {
			if d.zoneVerdict(n) != zoneInvalid {
				d.setZone(n, zoneInvalid)
				d.Log.Printf("ignoring the network's domain: %v", err)
			}
			continue
		}
		switch v := d.zoneVerdict(name); {
		case builtinZone(name), v == zoneLocal, v == zoneUnknown && mode == unbound.ForwardNetwork:
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	return out
}

func (d *Daemon) zoneVerdict(name string) zoneVerdict { return d.zones[name] }

func (d *Daemon) setZone(name string, v zoneVerdict) {
	if d.zones == nil {
		d.zones = map[string]zoneVerdict{}
	}
	d.zones[name] = v
}

// checkZones asks Unbound, recursing from the root or through the personal DNS-over-TLS resolver and validating,
// whether the network's domains not checked yet are signed in the public DNS. It asks Unbound itself, not the
// front, which would answer a blocked name with no data. A domain found unsigned is forwarded from the next step on.
// It reports whether one was.
func (d *Daemon) checkZones(ctx context.Context) bool {
	asked, found := 0, false
	for _, n := range d.network.Domains {
		name, err := unbound.CanonicalName(n)
		if err != nil || builtinZone(name) || d.zoneVerdict(name) != zoneUnknown {
			continue
		}
		if asked == zonesPerStep {
			break
		}
		asked++
		qctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		signed, err := probe.Signed(qctx, d.unboundAddr(), name)
		cancel()
		switch {
		case err != nil:
			if d.Debug {
				d.Log.Printf("debug: the network's domain %s: %v", name, err)
			}
		case signed:
			d.setZone(name, zoneSigned)
			d.Log.Printf("the network's domain %s is signed in the public DNS: it stays OffLok's, never the network's DNS", name)
		default:
			d.setZone(name, zoneLocal)
			found = true
		}
	}
	return found
}

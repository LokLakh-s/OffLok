package sysdns

import (
	"net/netip"
	"strings"
)

// savedServers keeps, of an interface's DNS list, only what can be its own: never a loopback address (OffLok's
// own, which a failed restore, a crash or a deleted state file leaves on the interface, and which would then be
// given back as the original), never a link-local address (Windows lists the router's fe80:: without the zone
// Unbound would need), never what does not parse. An empty result means automatic (DHCP, router advertisements).
func savedServers(servers string) string {
	var kept []string
	for _, f := range strings.FieldsFunc(servers, func(r rune) bool { return r == ',' || r == ' ' }) {
		a, err := netip.ParseAddr(f)
		if err != nil || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() {
			continue
		}
		kept = append(kept, a.String())
	}
	return strings.Join(kept, ",")
}

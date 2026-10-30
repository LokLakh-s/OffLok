package sysdns

import (
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

// What macOS's networksetup and scutil print, read here so that the tests run anywhere (macos.go runs them).

// parseServices reads `networksetup -listallnetworkservices`: a notice line, then one service per line, a disabled
// one starting with an asterisk.
func parseServices(out string) []string {
	var services []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "An asterisk (*)") {
			continue
		}
		services = append(services, strings.TrimPrefix(line, "*"))
	}
	return services
}

// parseDNSServers reads `networksetup -getdnsservers <service>`: one address per line, or a sentence when the
// service has none of its own (it then takes the network's).
func parseDNSServers(out string) []netip.Addr {
	var servers []netip.Addr
	for _, line := range strings.Split(out, "\n") {
		if a, err := netip.ParseAddr(strings.TrimSpace(line)); err == nil {
			servers = append(servers, a)
		}
	}
	return servers
}

// scutilDict is one dictionary scutil shows: its plain values and its arrays.
type scutilDict struct {
	Values map[string]string
	Arrays map[string][]string
}

var (
	scutilArray = regexp.MustCompile(`^\s*(\S+) : <array> \{$`)
	scutilItem  = regexp.MustCompile(`^\s*\d+ : (.*)$`)
	scutilValue = regexp.MustCompile(`^\s*(\S+) : (.*)$`)
)

// parseScutil reads the output of several `show <key>` given to one scutil, in order: a dictionary for each key,
// or nil for one that does not exist ("No such key").
func parseScutil(out string) []*scutilDict {
	var dicts []*scutilDict
	var cur *scutilDict
	array := ""
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "No such key":
			dicts = append(dicts, nil)
		case strings.HasPrefix(line, "<dictionary> {"):
			cur = &scutilDict{Values: map[string]string{}, Arrays: map[string][]string{}}
			dicts = append(dicts, cur)
		case cur == nil:
		case line == "}":
			cur, array = nil, ""
		case array != "" && trimmed == "}":
			array = ""
		case array != "":
			if m := scutilItem.FindStringSubmatch(line); m != nil {
				cur.Arrays[array] = append(cur.Arrays[array], m[1])
			}
		default:
			if m := scutilArray.FindStringSubmatch(line); m != nil {
				array = m[1]
				cur.Arrays[array] = []string{}
			} else if m := scutilValue.FindStringSubmatch(line); m != nil {
				cur.Values[m[1]] = m[2]
			}
		}
	}
	return dicts
}

// parseScutilList reads `list <pattern>`: "  subKey [0] = State:/Network/Service/<id>/DNS" lines.
func parseScutilList(out string) []string {
	var keys []string
	for _, line := range strings.Split(out, "\n") {
		if _, key, ok := strings.Cut(line, "] = "); ok {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	return keys
}

// networkFromDicts gathers the DNS that the network services were given (DHCP, router advertisements), the
// primary service's first. A dictionary with match domains is a VPN's split DNS, which macOS sends those domains
// to by itself: it is not the network's DNS for every name.
func networkFromDicts(dicts []*scutilDict) Network {
	var n Network
	for _, d := range dicts {
		if d == nil || len(d.Arrays["SupplementalMatchDomains"]) > 0 {
			continue
		}
		for _, s := range d.Arrays["ServerAddresses"] {
			if a, err := netip.ParseAddr(s); err == nil && !slices.Contains(n.Servers, a) {
				n.Servers = append(n.Servers, a)
			}
		}
		domains := d.Arrays["SearchDomains"]
		if name := d.Values["DomainName"]; name != "" {
			domains = append([]string{name}, domains...)
		}
		for _, dom := range domains {
			if dom != "" && !slices.Contains(n.Domains, dom) {
				n.Domains = append(n.Domains, dom)
			}
		}
	}
	return n
}

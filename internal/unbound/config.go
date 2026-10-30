// Package unbound writes Unbound's configuration and runs Unbound.
package unbound

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"text/template"
)

// Mode says where Unbound gets its answers from.
type Mode string

const (
	Recursive      Mode = "recursive"       // from the root servers down
	ForwardTLS     Mode = "forward-tls"     // everything to the personal DNS-over-TLS resolver
	ForwardNetwork Mode = "forward-network" // everything to the network's DNS
)

// Forwarder is an upstream resolver for the forward modes.
type Forwarder struct {
	Addr    netip.AddrPort
	TLSName string // DNS-over-TLS only: the name the server's certificate must carry
}

// Config is everything the generated unbound.conf depends on.
type Config struct {
	Listen          []netip.Addr // loopback addresses only
	Port            uint16
	Username        string // account Unbound drops to after binding; empty keeps the current one
	Directory       string // Unbound's working directory, writable by Username
	TrustAnchorFile string // inside Directory: Unbound rewrites it on key rollovers
	CABundle        string // CA certificates for DNS-over-TLS upstreams
	// Windows: Unbound takes no chroot, username or pidfile there, and checks DNS-over-TLS servers against the
	// system's certificate store (tls-win-cert) instead of a CA bundle.
	Windows    bool
	IPv6       bool // query servers over IPv6 too; only when the machine has global IPv6
	Mode       Mode
	Forwarders []Forwarder
	NetworkDNS []netip.Addr // the network's own resolvers, asked for LocalZones
	LocalZones []string     // names only the network's DNS knows (search domains, .lan, private reverse zones)
}

// unboundDefaultZones are local zones Unbound answers by itself (RFC 6303, RFC 8375). Forwarding one of them to the
// network needs a "nodefault" line first.
var unboundDefaultZones = map[string]bool{"home.arpa.": true, "10.in-addr.arpa.": true, "168.192.in-addr.arpa.": true,
	"254.169.in-addr.arpa.": true, "d.f.ip6.arpa.": true, "8.e.f.ip6.arpa.": true, "9.e.f.ip6.arpa.": true,
	"a.e.f.ip6.arpa.": true, "b.e.f.ip6.arpa.": true}

func init() {
	for i := 16; i <= 31; i++ {
		unboundDefaultZones[fmt.Sprintf("%d.172.in-addr.arpa.", i)] = true
	}
}

// PrivateZones are names that never exist on the public internet and that home and office networks commonly
// answer: forwarded to the network's DNS whenever one is known.
func PrivateZones() []string {
	zones := []string{"lan.", "home.", "internal.", "localdomain.", "home.arpa.", "10.in-addr.arpa.",
		"168.192.in-addr.arpa.", "d.f.ip6.arpa."}
	for i := 16; i <= 31; i++ {
		zones = append(zones, fmt.Sprintf("%d.172.in-addr.arpa.", i))
	}
	return zones
}

// CanonicalName lowercases a domain name, adds the final dot and rejects anything that is not a plain host name.
// Search domains come from DHCP: a name with a quote or a newline would otherwise inject lines into the config.
func CanonicalName(name string) (string, error) {
	n := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if n == "" || len(n) > 253 {
		return "", fmt.Errorf("invalid domain name %q", name)
	}
	for _, label := range strings.Split(n, ".") {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("invalid domain name %q", name)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return "", fmt.Errorf("invalid domain name %q", name)
			}
		}
	}
	return n + ".", nil
}

type localZone struct {
	Name      string
	NoDefault bool
}

type view struct {
	Config
	Forwards     []string
	LocalZoneSet []localZone
	ListenAddrs  []string
	NetworkAddrs []string
	Permissive   bool
}

// Render returns the unbound.conf for c.
func Render(c Config) ([]byte, error) {
	if len(c.Listen) == 0 {
		return nil, errors.New("no listen address")
	}
	v := view{Config: c, Permissive: c.Mode == ForwardNetwork}
	if c.Windows {
		// Unbound takes forward slashes on Windows too, and never mistakes them for escapes.
		v.Directory, v.TrustAnchorFile = filepath.ToSlash(c.Directory), filepath.ToSlash(c.TrustAnchorFile)
	}
	for _, a := range c.Listen {
		if !a.IsLoopback() {
			return nil, fmt.Errorf("refusing to listen on %s: loopback addresses only", a)
		}
		v.ListenAddrs = append(v.ListenAddrs, a.String())
	}
	switch c.Mode {
	case Recursive:
	case ForwardTLS, ForwardNetwork:
		if len(c.Forwarders) == 0 {
			return nil, fmt.Errorf("mode %s without forwarders", c.Mode)
		}
		for _, f := range c.Forwarders {
			s := fmt.Sprintf("%s@%d", f.Addr.Addr(), f.Addr.Port())
			if c.Mode == ForwardTLS {
				name, err := CanonicalName(f.TLSName)
				if err != nil {
					return nil, err
				}
				if c.CABundle == "" && !c.Windows {
					return nil, errors.New("DNS-over-TLS needs a CA bundle")
				}
				s += "#" + strings.TrimSuffix(name, ".")
			}
			v.Forwards = append(v.Forwards, s)
		}
	default:
		return nil, fmt.Errorf("unknown mode %q", c.Mode)
	}
	if len(c.NetworkDNS) > 0 {
		for _, a := range c.NetworkDNS {
			v.NetworkAddrs = append(v.NetworkAddrs, a.String())
		}
		seen := map[string]bool{}
		for _, z := range c.LocalZones {
			name, err := CanonicalName(z)
			if err != nil {
				return nil, err
			}
			if !seen[name] {
				seen[name] = true
				v.LocalZoneSet = append(v.LocalZoneSet, localZone{Name: name, NoDefault: unboundDefaultZones[name]})
			}
		}
	}
	var b bytes.Buffer
	if err := confTemplate.Execute(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

var confTemplate = template.Must(template.New("unbound.conf").Parse(`# Written by offlokd; edits are overwritten.
server:
    verbosity: 1
    use-syslog: no
    logfile: ""
    log-queries: no
    log-replies: no
    log-servfail: no
{{- if not .Windows}}
    chroot: ""
    pidfile: ""
    username: "{{.Username}}"
{{- end}}
    directory: "{{.Directory}}"
{{- range .ListenAddrs}}
    interface: {{.}}
{{- end}}
    port: {{.Port}}
    access-control: 0.0.0.0/0 refuse
    access-control: ::/0 refuse
    access-control: 127.0.0.0/8 allow
    access-control: ::1/128 allow
    do-ip4: yes
    do-ip6: {{if .IPv6}}yes{{else}}no{{end}}
    do-udp: yes
    do-tcp: yes
    num-threads: 1
    hide-identity: yes
    hide-version: yes
    qname-minimisation: yes
    harden-glue: yes
    harden-dnssec-stripped: yes
    harden-below-nxdomain: yes
    aggressive-nsec: yes
    prefetch: yes
    prefetch-key: yes
    serve-expired: yes
    serve-expired-ttl: 86400
    serve-expired-client-timeout: 1800
    msg-cache-size: 16m
    rrset-cache-size: 32m
    module-config: "validator iterator"
    auto-trust-anchor-file: "{{.TrustAnchorFile}}"
    val-permissive-mode: {{if .Permissive}}yes{{else}}no{{end}}
{{- if .Windows}}
    tls-win-cert: yes
{{- else if .CABundle}}
    tls-cert-bundle: "{{.CABundle}}"
{{- end}}
    private-address: 10.0.0.0/8
    private-address: 172.16.0.0/12
    private-address: 192.168.0.0/16
    private-address: 169.254.0.0/16
    private-address: fc00::/7
    private-address: fe80::/10
    private-address: ::ffff:0:0/96
{{- range .LocalZoneSet}}
    private-domain: "{{.Name}}"
    domain-insecure: "{{.Name}}"
{{- if .NoDefault}}
    local-zone: "{{.Name}}" nodefault
{{- end}}
{{- end}}

remote-control:
    control-enable: no
{{- range .LocalZoneSet}}

forward-zone:
    name: "{{.Name}}"
{{- range $.NetworkAddrs}}
    forward-addr: {{.}}
{{- end}}
{{- end}}
{{- if .Forwards}}

forward-zone:
    name: "."
{{- if eq .Mode "forward-tls"}}
    forward-tls-upstream: yes
{{- end}}
{{- range .Forwards}}
    forward-addr: {{.}}
{{- end}}
{{- end}}
`))

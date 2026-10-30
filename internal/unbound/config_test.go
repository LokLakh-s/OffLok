package unbound

import (
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func baseConfig() Config {
	return Config{
		Listen:          []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")},
		Port:            53,
		Directory:       "/var/lib/unbound/offlok",
		TrustAnchorFile: "/var/lib/unbound/offlok/root.key",
		Mode:            Recursive,
	}
}

func TestRenderRecursive(t *testing.T) {
	out, err := Render(baseConfig())
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"interface: 127.0.0.1\n", "interface: ::1\n", "log-queries: no", "do-ip6: no",
		"val-permissive-mode: no", "qname-minimisation: yes", "access-control: 0.0.0.0/0 refuse"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "forward-zone") {
		t.Errorf("recursive mode must not forward:\n%s", s)
	}
}

func TestRenderRefusesNonLoopback(t *testing.T) {
	c := baseConfig()
	c.Listen = append(c.Listen, netip.MustParseAddr("0.0.0.0"))
	if _, err := Render(c); err == nil {
		t.Fatal("listening on 0.0.0.0 must be refused")
	}
	c.Listen = []netip.Addr{netip.MustParseAddr("192.168.1.10")}
	if _, err := Render(c); err == nil {
		t.Fatal("listening on a LAN address must be refused")
	}
}

func TestRenderForwardTLS(t *testing.T) {
	c := baseConfig()
	c.Mode = ForwardTLS
	c.CABundle = "/etc/ssl/certs/ca-certificates.crt"
	addForwarder(t, &c, "192.0.2.1", 853, "Dns.Example.org.")
	out, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"forward-tls-upstream: yes", "forward-addr: 192.0.2.1@853#dns.example.org\n",
		`tls-cert-bundle: "/etc/ssl/certs/ca-certificates.crt"`, "val-permissive-mode: no"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	c.CABundle = ""
	if _, err := Render(c); err == nil {
		t.Fatal("DNS-over-TLS without a CA bundle must be refused")
	}
}

func TestRenderForwardNetworkIsPermissive(t *testing.T) {
	c := baseConfig()
	c.Mode = ForwardNetwork
	addForwarder(t, &c, "192.168.1.254", 53, "")
	out, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "val-permissive-mode: yes") || strings.Contains(s, "forward-tls-upstream") {
		t.Errorf("unexpected config:\n%s", s)
	}
	if !strings.Contains(s, "forward-addr: 192.168.1.254@53\n") {
		t.Errorf("missing forwarder:\n%s", s)
	}
}

func TestRenderLocalZones(t *testing.T) {
	c := baseConfig()
	c.NetworkDNS = []netip.Addr{netip.MustParseAddr("192.168.1.254")}
	c.LocalZones = append(PrivateZones(), "Corp.Example", "lan")
	out, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`name: "corp.example."`, `private-domain: "lan."`, `domain-insecure: "home.arpa."`,
		`local-zone: "168.192.in-addr.arpa." nodefault`, `local-zone: "20.172.in-addr.arpa." nodefault`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Count(s, `name: "lan."`) != 1 {
		t.Errorf("lan. must be forwarded once:\n%s", s)
	}
	if strings.Contains(s, `local-zone: "lan." nodefault`) {
		t.Errorf("lan. is not an Unbound default zone:\n%s", s)
	}

	c.NetworkDNS = nil
	out, _ = Render(c)
	if strings.Contains(string(out), "forward-zone") {
		t.Errorf("no local zones without a network DNS:\n%s", out)
	}
}

func TestCanonicalNameRejectsInjection(t *testing.T) {
	for _, bad := range []string{"", ".", "a..b", "evil\"\nserver:", "a b", "x/y", strings.Repeat("a", 64) + ".com"} {
		if _, err := CanonicalName(bad); err == nil {
			t.Errorf("CanonicalName(%q) accepted", bad)
		}
	}
	if got, err := CanonicalName(" Home.ARPA. "); err != nil || got != "home.arpa." {
		t.Errorf("CanonicalName = %q, %v", got, err)
	}
}

// TestRenderPassesCheckconf runs Unbound's own checker when it is installed (the integration image has it).
func TestRenderPassesCheckconf(t *testing.T) {
	checkconf, err := exec.LookPath("unbound-checkconf")
	if err != nil {
		t.Skip("unbound-checkconf not installed")
	}
	dir := t.TempDir()
	for name, c := range map[string]Config{"recursive": baseConfig(), "tls": baseConfig(), "network": baseConfig()} {
		c.Directory = dir
		c.TrustAnchorFile = filepath.Join(dir, "root.key")
		c.NetworkDNS = []netip.Addr{netip.MustParseAddr("192.168.1.254")}
		c.LocalZones = PrivateZones()
		switch name {
		case "tls":
			c.Mode, c.CABundle = ForwardTLS, "/etc/ssl/certs/ca-certificates.crt"
			addForwarder(t, &c, "192.0.2.1", 853, "dns.example.org")
		case "network":
			c.Mode = ForwardNetwork
			addForwarder(t, &c, "192.168.1.254", 53, "")
		}
		out, err := Render(c)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name+".conf")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		if msg, err := exec.Command(checkconf, path).CombinedOutput(); err != nil {
			t.Errorf("%s: unbound-checkconf: %v\n%s\n%s", name, err, msg, out)
		}
	}
}

func addForwarder(t *testing.T, c *Config, addr string, port uint16, tlsName string) {
	t.Helper()
	c.Forwarders = append(c.Forwarders, Forwarder{Addr: netip.AddrPortFrom(netip.MustParseAddr(addr), port), TLSName: tlsName})
}

func TestRenderWindows(t *testing.T) {
	c := baseConfig()
	c.Windows = true
	c.Directory, c.TrustAnchorFile = `C:\ProgramData\OffLok\unbound\keys`, `C:\ProgramData\OffLok\unbound\keys\root.key`
	c.Mode = ForwardTLS
	addForwarder(t, &c, "192.0.2.1", 853, "dns.example.org")
	out, err := Render(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, unwanted := range []string{"chroot:", "username:", "pidfile:", "tls-cert-bundle"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("Windows config has %q:\n%s", unwanted, s)
		}
	}
	if !strings.Contains(s, "tls-win-cert: yes") {
		t.Errorf("Windows checks DNS-over-TLS against its own store:\n%s", s)
	}
}

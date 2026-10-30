package sysdns

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	resolvedBus     = "org.freedesktop.resolve1"
	resolvedPath    = "/org/freedesktop/resolve1"
	resolvedManager = "org.freedesktop.resolve1.Manager"
	resolvedDropIn  = "/etc/systemd/resolved.conf.d/90-offlok.conf"
	resolvedMarker  = "# Written by OffLok"
)

// Resolved sets the global DNS of systemd-resolved. With the "~." routing domain on the global servers, resolved
// sends every name to them; a link's own servers only get the names of that link's domains (checked on systemd 255,
// see test/integration).
type Resolved struct {
	DropIn string // the drop-in file; resolvedDropIn unless testing
}

func (r *Resolved) Name() string { return "systemd-resolved" }

func (r *Resolved) dropIn() string {
	if r.DropIn != "" {
		return r.DropIn
	}
	return resolvedDropIn
}

func renderDropIn(servers []netip.Addr) string {
	addrs := make([]string, len(servers))
	for i, a := range servers {
		addrs[i] = a.String()
	}
	// DNSSEC=no: Unbound validates; resolved checking again would only add its own failure modes.
	return fmt.Sprintf("%s: removed when OffLok is turned off or uninstalled.\n[Resolve]\nDNS=%s\nDomains=~.\nDNSSEC=no\nDNSOverTLS=no\n",
		resolvedMarker, strings.Join(addrs, " "))
}

func (r *Resolved) Apply(servers []netip.Addr) error {
	want := renderDropIn(servers)
	if cur, err := os.ReadFile(r.dropIn()); err == nil && string(cur) == want {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.dropIn()), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(r.dropIn(), []byte(want), 0o644); err != nil {
		return err
	}
	return restartResolved(true)
}

func (r *Resolved) Applied(servers []netip.Addr) (bool, error) {
	entries, err := resolvedServers()
	if err != nil {
		return false, err
	}
	var global []netip.Addr
	for _, e := range entries {
		if e.ifindex == 0 {
			global = append(global, e.addr)
		}
	}
	return slices.Equal(global, servers), nil
}

func (r *Resolved) Restore() error {
	cur, err := os.ReadFile(r.dropIn())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(cur), resolvedMarker) {
		return fmt.Errorf("%s was not written by OffLok: left alone", r.dropIn())
	}
	if err := os.Remove(r.dropIn()); err != nil {
		return err
	}
	// Without waiting: when offlokd's own unit is stopping, systemd orders resolved's restart after that stop
	// (After=systemd-resolved.service), so waiting here would deadlock until the stop timeout.
	return restartResolved(false)
}

func (r *Resolved) Network() (Network, error) {
	entries, err := resolvedServers()
	if err != nil {
		return Network{}, err
	}
	var n Network
	for _, e := range entries {
		if e.ifindex != 0 && !slices.Contains(n.Servers, e.addr) {
			n.Servers = append(n.Servers, e.addr)
		}
	}
	var domains []struct {
		Ifindex   int32
		Domain    string
		RouteOnly bool
	}
	if err := resolvedProperty("Domains", &domains); err != nil {
		return Network{}, err
	}
	for _, d := range domains {
		if d.Ifindex != 0 && d.Domain != "." && d.Domain != "" && !slices.Contains(n.Domains, d.Domain) {
			n.Domains = append(n.Domains, d.Domain)
		}
	}
	return n.WithoutLoopback(), nil
}

type resolvedServer struct {
	ifindex int32
	addr    netip.Addr
}

func resolvedServers() ([]resolvedServer, error) {
	var raw []struct {
		Ifindex int32
		Family  int32
		Addr    []byte
	}
	if err := resolvedProperty("DNS", &raw); err != nil {
		return nil, err
	}
	out := make([]resolvedServer, 0, len(raw))
	for _, s := range raw {
		if a, ok := netip.AddrFromSlice(s.Addr); ok {
			out = append(out, resolvedServer{ifindex: s.Ifindex, addr: a.Unmap()})
		}
	}
	return out, nil
}

func resolvedProperty(name string, dst any) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	v, err := conn.Object(resolvedBus, resolvedPath).GetProperty(resolvedManager + "." + name)
	if err != nil {
		return fmt.Errorf("systemd-resolved %s: %w", name, err)
	}
	return dbus.Store([]any{v.Value()}, dst)
}

// FlushCaches empties systemd-resolved's cache.
func (r *Resolved) FlushCaches() error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	return conn.Object(resolvedBus, resolvedPath).Call(resolvedManager+".FlushCaches", 0).Err
}

// ResolvedActive reports whether systemd-resolved runs and /etc/resolv.conf sends programs to its stub.
func ResolvedActive() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		return false
	}
	var owned bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, resolvedBus).Store(&owned); err != nil || !owned {
		return false
	}
	content, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return false
	}
	n := ParseResolvConf(string(content))
	if len(n.Servers) == 0 {
		return false
	}
	for _, a := range n.Servers {
		if a != netip.MustParseAddr("127.0.0.53") && a != netip.MustParseAddr("127.0.0.54") {
			return false
		}
	}
	return true
}

// restartResolved restarts systemd-resolved: up to systemd 255 at least, it cannot reload its configuration.
// Network managers hand their links' DNS back to it when it comes back.
func restartResolved(wait bool) error {
	args := []string{"restart", "systemd-resolved.service"}
	if !wait {
		args = append(args, "--no-block")
	}
	if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("restart systemd-resolved: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

//go:build darwin

package sysdns

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// MacOS sets the DNS of every network service (Wi-Fi, Ethernet, a USB adapter…) with networksetup, as System
// Settings does. A service's own setting goes to Setup:/Network/Service/<id>/DNS; what DHCP and router
// advertisements gave stays apart in State:, where Network reads it (checked on GitHub's macOS 26).
type MacOS struct {
	StatePath string // each service's own DNS before OffLok, by name; none means the network's
}

// macState is MacOS's saved file: a service with no servers had none of its own.
type macState map[string][]netip.Addr

func (*MacOS) Name() string { return "macOS network services" }

func networksetup(args ...string) (string, error) {
	out, err := exec.Command("/usr/sbin/networksetup", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("networksetup %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func services() ([]string, error) {
	out, err := networksetup("-listallnetworkservices")
	if err != nil {
		return nil, err
	}
	return parseServices(out), nil
}

func dnsOf(service string) ([]netip.Addr, error) {
	out, err := networksetup("-getdnsservers", service)
	if err != nil {
		return nil, err
	}
	return parseDNSServers(out), nil
}

// setDNS sets a service's own servers; none gives it the network's again.
func setDNS(service string, servers []netip.Addr) error {
	args := []string{"-setdnsservers", service}
	for _, a := range servers {
		args = append(args, a.String())
	}
	if len(servers) == 0 {
		args = append(args, "Empty")
	}
	_, err := networksetup(args...)
	return err
}

func (m *MacOS) load() (macState, error) {
	data, err := os.ReadFile(m.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st macState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", m.StatePath, err)
	}
	return st, nil
}

func (m *MacOS) save(st macState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.StatePath), 0o755); err != nil {
		return err
	}
	tmp := m.StatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.StatePath)
}

// Apply points every service at servers. A service's setting is saved first, unless it is already OffLok's: a
// service set by hand since (System Settings), or new (an adapter plugged in), gets its setting saved then.
func (m *MacOS) Apply(servers []netip.Addr) error {
	names, err := services()
	if err != nil {
		return err
	}
	st, err := m.load()
	if err != nil {
		return err
	}
	if st == nil {
		st = macState{}
	}
	var todo []string
	for _, name := range names {
		cur, err := dnsOf(name)
		if err != nil {
			return err
		}
		// Loopback only is OffLok's own (or its other addresses, when IPv6 came or went): never an original.
		if isOffLok(cur) {
			if _, ok := st[name]; !ok {
				st[name] = nil // its original unknown: the network's is the safe one
			}
		} else {
			st[name] = cur
		}
		if !slices.Equal(cur, servers) {
			todo = append(todo, name)
		}
	}
	if err := m.save(st); err != nil {
		return err
	}
	for _, name := range todo {
		if err := setDNS(name, servers); err != nil {
			return err
		}
	}
	return flush()
}

// Applied reports whether every service points at servers.
func (m *MacOS) Applied(servers []netip.Addr) (bool, error) {
	names, err := services()
	if err != nil {
		return false, err
	}
	for _, name := range names {
		cur, err := dnsOf(name)
		if err != nil {
			return false, err
		}
		if !slices.Equal(cur, servers) {
			return false, nil
		}
	}
	return len(names) > 0, nil
}

// Restore gives each service its saved setting back, unless it was changed since OffLok set it: then the newer
// setting stays. A service removed meanwhile is skipped.
func (m *MacOS) Restore() error {
	st, err := m.load()
	if err != nil || st == nil {
		return err
	}
	names, err := services()
	if err != nil {
		return err
	}
	var errs []error
	for name, orig := range st {
		if !slices.Contains(names, name) {
			continue
		}
		cur, err := dnsOf(name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !isOffLok(cur) {
			continue
		}
		if err := setDNS(name, orig); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if err := os.Remove(m.StatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return flush()
}

// isOffLok: OffLok's setting is loopback addresses only.
func isOffLok(servers []netip.Addr) bool {
	if len(servers) == 0 {
		return false
	}
	for _, a := range servers {
		if !a.IsLoopback() {
			return false
		}
	}
	return true
}

// Network reads what DHCP and router advertisements gave each service, which networksetup leaves in place.
func (m *MacOS) Network() (Network, error) {
	listing, err := scutil("list State:/Network/Service/[^/]+/DNS\nshow State:/Network/Global/IPv4\n")
	if err != nil {
		return Network{}, err
	}
	keys := parseScutilList(listing)
	primary := ""
	if dicts := parseScutil(listing); len(dicts) > 0 && dicts[len(dicts)-1] != nil {
		primary = dicts[len(dicts)-1].Values["PrimaryService"]
	}
	slices.SortStableFunc(keys, func(a, b string) int {
		ap, bp := strings.Contains(a, "/"+primary+"/"), strings.Contains(b, "/"+primary+"/")
		switch {
		case primary != "" && ap && !bp:
			return -1
		case primary != "" && bp && !ap:
			return 1
		}
		return 0
	})
	if len(keys) == 0 {
		return Network{}, nil
	}
	var cmds strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&cmds, "show %s\n", k)
	}
	out, err := scutil(cmds.String())
	if err != nil {
		return Network{}, err
	}
	return networkFromDicts(parseScutil(out)).WithoutLoopback(), nil
}

func scutil(commands string) (string, error) {
	cmd := exec.Command("/usr/sbin/scutil")
	cmd.Stdin = strings.NewReader(commands)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("scutil: %w", err)
	}
	return string(out), nil
}

// FlushCaches empties macOS's own DNS cache (mDNSResponder's), so that a change shows at once.
func (*MacOS) FlushCaches() error { return flush() }

func flush() error {
	if err := exec.Command("/usr/bin/dscacheutil", "-flushcache").Run(); err != nil {
		return fmt.Errorf("dscacheutil: %w", err)
	}
	if err := exec.Command("/usr/bin/killall", "-HUP", "mDNSResponder").Run(); err != nil {
		return fmt.Errorf("mDNSResponder: %w", err)
	}
	return nil
}

func macBackend(stateDir string) *MacOS {
	return &MacOS{StatePath: filepath.Join(stateDir, "macos-dns.json")}
}

// Detect: macOS has one way.
func Detect(stateDir string) Backend { return macBackend(stateDir) }

// RestoreAll gives every service its saved setting back: what an uninstall runs.
func RestoreAll(stateDir string) error { return macBackend(stateDir).Restore() }

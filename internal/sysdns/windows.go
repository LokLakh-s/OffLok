//go:build windows

package sysdns

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows sets every interface's own DNS servers (SetInterfaceDnsSettings, Windows 10 2004 and later), IPv4 and
// IPv6 apart, and keeps each interface's original: empty when it came from DHCP, which an empty setting gives
// back.
type Windows struct {
	StatePath string
}

// The original static servers of one interface; "" means automatic (DHCP, router advertisements).
type ifaceDNS struct {
	V4 string `json:"v4"`
	V6 string `json:"v6"`
}

func (w *Windows) Name() string { return "Windows" }

var (
	iphlpapi                     = windows.NewLazySystemDLL("iphlpapi.dll")
	procSetInterfaceDnsSettings  = iphlpapi.NewProc("SetInterfaceDnsSettings")
	procGetInterfaceDnsSettings  = iphlpapi.NewProc("GetInterfaceDnsSettings")
	procFreeInterfaceDnsSettings = iphlpapi.NewProc("FreeInterfaceDnsSettings")
)

const (
	dnsSettingIPv6       = 0x0001
	dnsSettingNameServer = 0x0002
	dnsSettingsVersion1  = 1
)

// dnsInterfaceSettings is DNS_INTERFACE_SETTINGS; the padding puts Flags (ULONG64) on 8 bytes, as C does.
type dnsInterfaceSettings struct {
	Version             uint32
	_                   uint32
	Flags               uint64
	Domain              *uint16
	NameServer          *uint16
	SearchList          *uint16
	RegistrationEnabled uint32
	RegisterAdapterName uint32
	EnableLLMNR         uint32
	QueryAdapterName    uint32
	ProfileNameServer   *uint16
}

// guidArgs passes a GUID by value: on amd64 a 16-byte struct goes as a pointer to a copy, on arm64 in two registers.
func guidArgs(g *windows.GUID) []uintptr {
	if runtime.GOARCH == "arm64" {
		words := (*[2]uintptr)(unsafe.Pointer(g))
		return []uintptr{words[0], words[1]}
	}
	return []uintptr{uintptr(unsafe.Pointer(g))}
}

func callStatus(r uintptr) error {
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func getNameServer(guid windows.GUID, ipv6 bool) (string, error) {
	s := dnsInterfaceSettings{Version: dnsSettingsVersion1}
	if ipv6 {
		s.Flags = dnsSettingIPv6
	}
	r, _, _ := procGetInterfaceDnsSettings.Call(append(guidArgs(&guid), uintptr(unsafe.Pointer(&s)))...)
	if err := callStatus(r); err != nil {
		return "", err
	}
	defer procFreeInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&s)))
	if s.NameServer == nil {
		return "", nil
	}
	return windows.UTF16PtrToString(s.NameServer), nil
}

func setNameServer(guid windows.GUID, ipv6 bool, servers string) error {
	p, err := windows.UTF16PtrFromString(servers)
	if err != nil {
		return err
	}
	s := dnsInterfaceSettings{Version: dnsSettingsVersion1, Flags: dnsSettingNameServer, NameServer: p}
	if ipv6 {
		s.Flags |= dnsSettingIPv6
	}
	r, _, _ := procSetInterfaceDnsSettings.Call(append(guidArgs(&guid), uintptr(unsafe.Pointer(&s)))...)
	return callStatus(r)
}

type adapter struct {
	guid   windows.GUID
	name   string // "{...}", the key under Tcpip\Parameters\Interfaces
	suffix string
}

// adapters lists the interfaces that are up, the loopback left out.
func adapters() ([]adapter, error) {
	size := uint32(16 << 10)
	for {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var out []adapter
		for a := first; a != nil; a = a.Next {
			if a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
				continue
			}
			name := windows.BytePtrToString(a.AdapterName)
			guid, err := windows.GUIDFromString(name)
			if err != nil {
				continue
			}
			out = append(out, adapter{guid: guid, name: name, suffix: windows.UTF16PtrToString(a.DnsSuffix)})
		}
		return out, nil
	}
}

func (w *Windows) load() (map[string]ifaceDNS, error) {
	data, err := os.ReadFile(w.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]ifaceDNS{}, nil
	}
	if err != nil {
		return nil, err
	}
	saved := map[string]ifaceDNS{}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("%s: %w", w.StatePath, err)
	}
	return saved, nil
}

func (w *Windows) save(saved map[string]ifaceDNS) error {
	data, _ := json.MarshalIndent(saved, "", "  ")
	if err := os.MkdirAll(filepath.Dir(w.StatePath), 0o755); err != nil {
		return err
	}
	tmp := w.StatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, w.StatePath)
}

func split(servers []netip.Addr) (v4, v6 string) {
	var a, b []string
	for _, s := range servers {
		if s.Is4() {
			a = append(a, s.String())
		} else {
			b = append(b, s.String())
		}
	}
	return strings.Join(a, ","), strings.Join(b, ",")
}

func (w *Windows) Apply(servers []netip.Addr) error {
	saved, err := w.load()
	if err != nil {
		return err
	}
	list, err := adapters()
	if err != nil {
		return err
	}
	v4, v6 := split(servers)
	var errs []error
	for _, a := range list {
		if _, ok := saved[a.name]; !ok {
			cur4, err4 := getNameServer(a.guid, false)
			cur6, err6 := getNameServer(a.guid, true)
			if err4 != nil || err6 != nil {
				errs = append(errs, fmt.Errorf("%s: read its DNS: %v", a.name, errors.Join(err4, err6)))
				continue
			}
			saved[a.name] = ifaceDNS{V4: cur4, V6: cur6}
			if err := w.save(saved); err != nil {
				return err
			}
		}
		if err := setNameServer(a.guid, false, v4); err != nil {
			errs = append(errs, fmt.Errorf("%s: set IPv4 DNS: %w", a.name, err))
		}
		if v6 != "" {
			if err := setNameServer(a.guid, true, v6); err != nil {
				errs = append(errs, fmt.Errorf("%s: set IPv6 DNS: %w", a.name, err))
			}
		}
	}
	return errors.Join(errs...)
}

func (w *Windows) Applied(servers []netip.Addr) (bool, error) {
	list, err := adapters()
	if err != nil {
		return false, err
	}
	v4, v6 := split(servers)
	for _, a := range list {
		cur4, err := getNameServer(a.guid, false)
		if err != nil || normalize(cur4) != v4 {
			return false, err
		}
		if v6 != "" {
			if cur6, err := getNameServer(a.guid, true); err != nil || normalize(cur6) != v6 {
				return false, err
			}
		}
	}
	return true, nil
}

func normalize(servers string) string {
	return strings.Join(strings.FieldsFunc(servers, func(r rune) bool { return r == ',' || r == ' ' }), ",")
}

// Restore puts back every saved interface, connected or not: settings live with the interface, and a Wi-Fi card
// off at uninstall time must not come back pointing at a resolver that is gone.
func (w *Windows) Restore() error {
	saved, err := w.load()
	if err != nil || len(saved) == 0 {
		return err
	}
	var errs []error
	for name, orig := range saved {
		guid, err := windows.GUIDFromString(name)
		if err != nil {
			continue
		}
		if err := setNameServer(guid, false, orig.V4); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		if err := setNameServer(guid, true, orig.V6); err != nil {
			errs = append(errs, fmt.Errorf("%s (IPv6): %w", name, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return os.Remove(w.StatePath)
}

// Network gives the servers the network provides (DHCP's, kept by Windows in the registry, and the static ones each
// interface had before OffLok) and the interfaces' DNS suffixes.
func (w *Windows) Network() (Network, error) {
	saved, _ := w.load()
	list, err := adapters()
	if err != nil {
		return Network{}, err
	}
	var n Network
	add := func(s string) {
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
			if a, err := netip.ParseAddr(f); err == nil && !slices.Contains(n.Servers, a) {
				n.Servers = append(n.Servers, a)
			}
		}
	}
	for _, a := range list {
		if orig, ok := saved[a.name]; ok {
			add(orig.V4)
			add(orig.V6)
		}
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\`+a.name, registry.QUERY_VALUE); err == nil {
			if v, _, err := k.GetStringValue("DhcpNameServer"); err == nil {
				add(v)
			}
			if v, _, err := k.GetStringValue("DhcpDomain"); err == nil && v != "" && !slices.Contains(n.Domains, v) {
				n.Domains = append(n.Domains, v)
			}
			k.Close()
		}
		if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\`+a.name, registry.QUERY_VALUE); err == nil {
			if v, _, err := k.GetBinaryValue("Dhcpv6DNSServers"); err == nil {
				for i := 0; i+16 <= len(v); i += 16 {
					if addr, ok := netip.AddrFromSlice(v[i : i+16]); ok && !slices.Contains(n.Servers, addr) {
						n.Servers = append(n.Servers, addr)
					}
				}
			}
			k.Close()
		}
		if a.suffix != "" && !slices.Contains(n.Domains, a.suffix) {
			n.Domains = append(n.Domains, a.suffix)
		}
	}
	return n.WithoutLoopback(), nil
}

// Detect returns the Windows backend.
func Detect(stateDir string) Backend {
	return &Windows{StatePath: filepath.Join(stateDir, "interfaces.json")}
}

// RestoreAll restores the saved interfaces: what an uninstall runs.
func RestoreAll(stateDir string) error {
	return Detect(stateDir).Restore()
}

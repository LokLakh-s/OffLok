package sysdns

import (
	"net/netip"
	"slices"
	"testing"
)

// The outputs below are copied from GitHub's macOS 26 machine (2026-10-30), a Wi-Fi and a VPN service added.

func TestParseServices(t *testing.T) {
	out := "An asterisk (*) denotes that a network service is disabled.\nEthernet\nWi-Fi\n*Thunderbolt Bridge\n"
	if got, want := parseServices(out), []string{"Ethernet", "Wi-Fi", "Thunderbolt Bridge"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseDNSServers(t *testing.T) {
	if got := parseDNSServers("There aren't any DNS Servers set on Ethernet.\n"); len(got) != 0 {
		t.Fatalf("none set: got %v", got)
	}
	got := parseDNSServers("127.0.0.1\n::1\n")
	want := []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

const scutilListing = `  subKey [0] = State:/Network/Service/AAAA-VPN/DNS
  subKey [1] = State:/Network/Service/910946D2-927E-454C-A788-F597AAD7AF10/DNS
<dictionary> {
  PrimaryInterface : en0
  PrimaryService : 910946D2-927E-454C-A788-F597AAD7AF10
  Router : 192.168.64.1
}
`

const scutilShows = `<dictionary> {
  ServerAddresses : <array> {
    0 : 10.8.0.1
  }
  SupplementalMatchDomains : <array> {
    0 : corp.example
  }
}
<dictionary> {
  DomainName : home
  SearchDomains : <array> {
    0 : home
    1 : lan
  }
  ServerAddresses : <array> {
    0 : 192.168.64.1
    1 : fe80::1%en0
    2 : 127.0.0.1
  }
}
  No such key
`

func TestParseScutil(t *testing.T) {
	keys := parseScutilList(scutilListing)
	if len(keys) != 2 || keys[1] != "State:/Network/Service/910946D2-927E-454C-A788-F597AAD7AF10/DNS" {
		t.Fatalf("keys: %q", keys)
	}
	dicts := parseScutil(scutilListing)
	if len(dicts) != 1 || dicts[0].Values["PrimaryService"] != "910946D2-927E-454C-A788-F597AAD7AF10" {
		t.Fatalf("primary: %+v", dicts)
	}
	dicts = parseScutil(scutilShows)
	if len(dicts) != 3 || dicts[2] != nil {
		t.Fatalf("got %d dictionaries, the last %+v", len(dicts), dicts[len(dicts)-1])
	}
	n := networkFromDicts(dicts).WithoutLoopback()
	wantServers := []netip.Addr{netip.MustParseAddr("192.168.64.1"), netip.MustParseAddr("fe80::1%en0")}
	if !slices.Equal(n.Servers, wantServers) {
		t.Fatalf("servers: got %v, want %v (the VPN's split DNS and the loopback left out)", n.Servers, wantServers)
	}
	if !slices.Equal(n.Domains, []string{"home", "lan"}) {
		t.Fatalf("domains: got %q", n.Domains)
	}
}

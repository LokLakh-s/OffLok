// Package root holds the facts about the DNS root that OffLok ships with: the root servers' addresses and the
// root zone's DNSSEC trust anchors.
package root

import "net/netip"

// Servers are the 13 root servers, IPv4 then IPv6, from InterNIC's named.root of 2026-10-28 (root zone
// 2026102801). Unbound has its own compiled-in copy; offlokd uses this one only to probe reachability.
var Servers = []netip.Addr{
	netip.MustParseAddr("198.41.0.4"),     // a.root-servers.net
	netip.MustParseAddr("170.247.170.2"),  // b
	netip.MustParseAddr("192.33.4.12"),    // c
	netip.MustParseAddr("199.7.91.13"),    // d
	netip.MustParseAddr("192.203.230.10"), // e
	netip.MustParseAddr("192.5.5.241"),    // f
	netip.MustParseAddr("192.112.36.4"),   // g
	netip.MustParseAddr("198.97.190.53"),  // h
	netip.MustParseAddr("192.36.148.17"),  // i
	netip.MustParseAddr("192.58.128.30"),  // j
	netip.MustParseAddr("193.0.14.129"),   // k
	netip.MustParseAddr("199.7.83.42"),    // l
	netip.MustParseAddr("202.12.27.33"),   // m
	netip.MustParseAddr("2001:503:ba3e::2:30"),
	netip.MustParseAddr("2801:1b8:10::b"),
	netip.MustParseAddr("2001:500:2::c"),
	netip.MustParseAddr("2001:500:2d::d"),
	netip.MustParseAddr("2001:500:a8::e"),
	netip.MustParseAddr("2001:500:2f::f"),
	netip.MustParseAddr("2001:500:12::d0d"),
	netip.MustParseAddr("2001:500:1::53"),
	netip.MustParseAddr("2001:7fe::53"),
	netip.MustParseAddr("2001:503:c27::2:30"),
	netip.MustParseAddr("2001:7fd::1"),
	netip.MustParseAddr("2001:500:9f::42"),
	netip.MustParseAddr("2001:dc3::35"),
}

// TrustAnchors are the DS records of the root's key-signing keys still valid on 2026-10-29 (KSK-2017, tag 20326,
// and KSK-2024, tag 38696), from IANA's root-anchors.xml. They seed Unbound's auto-trust-anchor-file once; from
// then on Unbound follows key rollovers by itself (RFC 5011).
const TrustAnchors = `. IN DS 20326 8 2 E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D
. IN DS 38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16
`

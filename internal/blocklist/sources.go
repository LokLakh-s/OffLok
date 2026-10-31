package blocklist

import (
	"fmt"
	"hash/fnv"
)

// Category is a kind of blocking, switched on and off by itself.
type Category string

const (
	Ads       Category = "ads"
	Trackers  Category = "trackers"
	Malicious Category = "malicious"
	Custom    Category = "custom" // the lists the user adds by their address
)

// Categories in the order the apps show them. The ads come first, in builds that offer them (ads.go).
var Categories = []Category{Trackers, Malicious, Custom}

// Source is one list.
type Source struct {
	ID      string
	Name    string
	URL     string
	Licence string
	Home    string
}

func registry(n int) string {
	return fmt.Sprintf("https://adguardteam.github.io/HostlistsRegistry/assets/filter_%d.txt", n)
}

// Sources are the lists OffLok offers (chosen on 29/10/2026, licences checked that day), downloaded by the
// computer itself once a day. They are never shipped with OffLok.
var Sources = map[Category][]Source{
	Trackers: {
		{ID: "shadowwhisperer-tracking", Name: "ShadowWhisperer Tracking List", URL: registry(69), Licence: "Unlicense", Home: "https://github.com/ShadowWhisperer/BlockLists"},
		{ID: "hagezi-windows", Name: "HaGeZi's Windows/Office Tracker Blocklist", URL: registry(63), Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
		{ID: "hagezi-apple", Name: "HaGeZi's Apple Tracker Blocklist", URL: registry(67), Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
		{ID: "hagezi-samsung", Name: "HaGeZi's Samsung Tracker Blocklist", URL: registry(61), Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
		{ID: "hagezi-xiaomi", Name: "HaGeZi's Xiaomi Tracker Blocklist", URL: registry(60), Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
		{ID: "hagezi-huawei", Name: "HaGeZi's Huawei Tracker Blocklist", URL: registry(73), Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
	},
	Malicious: {
		{ID: "urlhaus", Name: "Malicious URL Blocklist (URLhaus)", URL: registry(11), Licence: "CC0-1.0 / MIT", Home: "https://gitlab.com/malware-filter/urlhaus-filter"},
		{ID: "phishing", Name: "Phishing URL Blocklist (PhishTank, OpenPhish)", URL: registry(30), Licence: "CC-BY-SA-4.0", Home: "https://gitlab.com/malware-filter/phishing-filter"},
		{ID: "scam", Name: "Scam Blocklist by DurableNapkin", URL: registry(10), Licence: "MIT", Home: "https://github.com/durablenapkin/scamblocklist"},
		{ID: "hagezi-tif-mini", Name: "HaGeZi's Threat Intelligence Feeds (mini)", URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/tif.mini.txt", Licence: "GPL-3.0", Home: "https://github.com/hagezi/dns-blocklists"},
	},
}

// CustomSource is the source for a list the user added by its address.
func CustomSource(url string) Source {
	h := fnv.New64a()
	h.Write([]byte(url))
	return Source{ID: fmt.Sprintf("custom-%016x", h.Sum64()), Name: url, URL: url}
}

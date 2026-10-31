//go:build !noads

package blocklist

// The ads, in every build but Google Play's (the noads tag): Google Play refuses an app that blocks ads through
// Android's VPN service.
func init() {
	Categories = append([]Category{Ads}, Categories...)
	Sources[Ads] = []Source{
		{ID: "adguard-dns", Name: "AdGuard DNS filter", URL: registry(1), Licence: "GPL-3.0", Home: "https://github.com/AdguardTeam/AdGuardSDNSFilter"},
	}
}

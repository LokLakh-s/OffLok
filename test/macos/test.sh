#!/bin/bash
# OffLok's macOS test, on a disposable Mac (GitHub's macOS runner, where sudo asks no password): installs
# dist/offlok-<version>.pkg as a user would, checks the launchd service, the menu bar app, every network service's
# DNS, DNSSEC and the blocking, kills offlokd, cuts the root servers off (pf) for the fallback and strict mode, then
# uninstalls with the package's script and checks that every network service has its original DNS back.
set -uo pipefail
cd "$(dirname "$0")/../.."
. test/macos/lib.sh

conf="/Library/Application Support/OffLok/unbound/unbound.conf"

# Every service's DNS is loopback addresses only, 127.0.0.1 among them.
all_offlok() {
	local s out
	while IFS= read -r s; do
		out=$(networksetup -getdnsservers "$s")
		grep -qx 127.0.0.1 <<<"$out" || return 1
		grep -Evx '127\.0\.0\.1|::1' <<<"$out" | grep -q . && return 1
	done < <(services)
	return 0
}

unbounds() { ps -axo user=,command= | grep -F -- "-d -c $conf" | grep -v grep; }

# Each check is a function: a `bash -c` would not see this script's functions, and would pass for nothing.
launchd_runs() { sudo launchctl print system/org.loklakh.offlokd 2>/dev/null | grep -q "state = running"; }
system_resolver_is_offlok() {
	scutil --dns | awk '/^resolver #1/ {f = 1} f && /nameserver\[0\]/ {print $3; exit}' | grep -qx 127.0.0.1
}
system_resolves() { dscacheutil -q host -a name nlnetlabs.nl | grep -q ip_address; }
dnssec_checked() { dig @127.0.0.1 +dnssec nlnetlabs.nl A | grep -Eq 'flags:[a-z ]* ad'; }
dnssec_refused() { dig @127.0.0.1 +tries=1 +time=10 dnssec-failed.org A | grep -q 'status: SERVFAIL'; }
loopback_only() {
	local s
	s=$(sudo lsof -nP -iUDP:53 -iTCP:53 -sTCP:LISTEN | awk 'NR > 1 && $1 == "offlokd" {print $9}')
	[ -n "$s" ] && ! grep -Evq '^(127\.0\.0\.1|\[::1\]):53$' <<<"$s"
}
unbound_user_is() { [ "$(unbounds | awk '{print $1}' | sort -u)" = "$1" ]; }
unbound_count_is() { [ "$(unbounds | wc -l | tr -d ' ')" -eq "$1" ]; }
ads_downloaded() { "$offlok" status | grep -Eq 'ads \([0-9]{5,} names\)'; }
blocked() { dig @127.0.0.1 +short "$1" A | grep -qx 0.0.0.0; }
check_names_list() { "$offlok" check doubleclick.net | grep -q 'AdGuard DNS filter'; }
offlokd_runs() { pgrep -x offlokd >/dev/null; }
resolves_through_network() { dig @127.0.0.1 +short example.org A | grep -Eq '^[0-9.]+$'; }
no_account() { ! id _offlok >/dev/null 2>&1; }
pkg_registered() { pkgutil --pkgs | grep -qx org.loklakh.offlok; }
unbound_is_packaged() { unbounds | grep -qF "$libexec/unbound/unbound -d -c"; }
no_programs() { [ ! -e "$libexec" ] && [ ! -e "$offlok" ] && [ ! -e /Applications/OffLok.app ] && ! pkg_registered; }
no_files() { [ ! -e "/Library/Application Support/OffLok" ] && [ ! -e /Library/LaunchDaemons/org.loklakh.offlokd.plist ]; }

before=$(dns_snapshot)
echo "# DNS before:"
echo "$before"

pkg=$(ls dist/offlok-*.pkg | head -1)
sudo installer -pkg "$pkg" -target / || exit 1

check 'launchd runs offlokd' launchd_runs
check 'the package is registered' pkg_registered
check "Unbound is the package's own" unbound_is_packaged
check "Unbound's hidden account exists" id _offlok
check 'resolves by itself, or falls back on this network' wait_until 60 status_is 'On|Fallback'
echo "# $("$offlok" status | tr '\n' '|')"
echo "# DNS while on:"
dns_snapshot
check 'every network service points at OffLok' all_offlok
check "the system's resolver is OffLok" system_resolver_is_offlok
check 'programs resolve through the system' system_resolves
check 'a signed answer is checked (DNSSEC)' dnssec_checked
check 'a broken DNSSEC signature is refused' dnssec_refused
echo "# DNS sockets:"
sudo lsof -nP -iUDP:53 -iTCP:53 -sTCP:LISTEN | tail -n +2
check 'OffLok answers DNS on the loopback only' loopback_only
check 'Unbound runs as _offlok' unbound_user_is _offlok

# The menu bar app, started for the logged-in user by the package (or by hand when no one is).
if ! app_runs; then
	open -a /Applications/OffLok.app --args --hidden
fi
sleep 8
check 'the menu bar app starts and stays up' app_runs

sudo "$offlok" block ads on >/dev/null
check 'ads: the list is downloaded' wait_until 90 ads_downloaded
check 'ads: doubleclick.net is blocked' blocked doubleclick.net
check 'check names the list' check_names_list
sudo "$offlok" block ads off >/dev/null

# A crash: launchd starts offlokd again, which stops the Unbound its predecessor left.
sudo pkill -9 -x offlokd
check 'after a crash, launchd starts offlokd again' wait_until 30 offlokd_runs
check 'and it resolves again' wait_until 60 status_is 'On|Fallback'
check 'one Unbound only' unbound_count_is 1

# The root servers out of reach (a hotel's Wi-Fi): pf lets port 53 through to the network's DNS only.
id=$(printf 'show State:/Network/Global/IPv4\n' | scutil | awk '/PrimaryService/ {print $3}')
netdns=$(printf 'show State:/Network/Service/%s/DNS\n' "$id" | scutil | awk '/^ +0 : / {print $3; exit}')
echo "# the network's DNS: $netdns"
rules=$(mktemp)
printf 'pass quick on lo0 all\npass out quick proto { udp tcp } from any to %s port 53\nblock drop out quick proto { udp tcp } from any to any port 53\n' "$netdns" >"$rules"
token=$(sudo pfctl -E -f "$rules" 2>&1 | awk '/Token/ {print $3}')
check 'root out of reach: falls back on the network' wait_until 90 status_is 'Fallback'
check 'names still resolve, through the network' resolves_through_network
sudo "$offlok" strict on >/dev/null
check 'strict mode: public names are blocked' wait_until 60 status_is 'Strict mode'
sudo "$offlok" strict off >/dev/null
sudo pfctl -q -f /etc/pf.conf
[ -n "$token" ] && sudo pfctl -q -X "$token"
check 'the root back: resolves by itself again' wait_until 120 status_is 'On'

sudo "$libexec/uninstall.sh"
check 'uninstall takes offlokd out of launchd' launchd_has_none
check 'unbound is gone' unbound_count_is 0
check "Unbound's account is gone" no_account
check "OffLok's files are gone" no_files
check "OffLok's programs are gone" no_programs
check 'the menu bar app is gone' bash -c '! pgrep -x offlok-desktop >/dev/null'
after=$(dns_snapshot)
echo "# DNS after:"
echo "$after"
check 'every network service has its original DNS back' [ "$before" = "$after" ]

summary

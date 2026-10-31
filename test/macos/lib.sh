# Helpers shared by OffLok's macOS tests (sourced, never run).
offlok=/usr/local/bin/offlok
libexec=/usr/local/libexec/offlok
plist=/Library/LaunchDaemons/org.loklakh.offlokd.plist
passed=0
failed=0

check() {
	local name=$1
	shift
	if "$@"; then
		echo "ok    $name"
		passed=$((passed + 1))
	else
		echo "FAIL  $name"
		failed=$((failed + 1))
	fi
}

wait_until() {
	local secs=$1
	shift
	for ((i = 0; i < secs; i++)); do
		"$@" && return 0
		sleep 1
	done
	return 1
}

services() { networksetup -listallnetworkservices | tail -n +2 | sed 's/^\*//'; }

# Every network service's DNS servers: what an uninstall must give back.
dns_snapshot() {
	services | while IFS= read -r s; do
		printf '%s: %s\n' "$s" "$(networksetup -getdnsservers "$s" | tr '\n' ' ')"
	done
}

status_is() { "$offlok" status | head -1 | grep -Eq "^($1)"; }
launchd_has_none() { ! sudo launchctl print system/org.loklakh.offlokd >/dev/null 2>&1; }
app_runs() { pgrep -x offlok-desktop >/dev/null; }

summary() {
	echo "# $passed passed, $failed failed"
	[ "$failed" -eq 0 ]
}

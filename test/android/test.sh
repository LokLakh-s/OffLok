#!/bin/bash
# OffLok on an Android emulator (GitHub's Linux machines, .github/workflows/android.yml): the full app's debug build
# installed, let start a VPN (appops, as the user's "OK" in Android's dialog), turned on; then the phone's own
# resolver (ping's lookups) goes through OffLok's tunnel: names resolve, a broken DNSSEC signature is refused, the
# ad list blocks; turned off, the tunnel is gone. The debug build's TestCommand receiver runs the core's commands.
#   test/android/test.sh <apk>
set -uo pipefail
apk=$1
pkg=org.loklakh.offlok
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

# wait_until <seconds> <command...>: a deadline, however long each try takes.
wait_until() {
	local deadline=$((SECONDS + $1))
	shift
	while :; do
		"$@" && return 0
		[ "$SECONDS" -lt "$deadline" ] || return 1
		sleep 1
	done
}

# One of the core's commands; its result is the OffLokTest line with its id (the log is kept whole, for the end).
# adb shell joins its arguments into one command line, where an empty one would vanish: a value goes only when there
# is one.
calls=0
core() {
	local value=() id=$((++calls))
	[ -z "${2:-}" ] || value=(--es value "$2")
	adb shell am broadcast -n "$pkg/.TestCommand" --es id "$id" --es cmd "$1" "${value[@]}" >/dev/null ||
		{ echo "# the broadcast for $1 failed" >&2; return 1; }
	wait_until 10 sh -c "adb logcat -d -s OffLokTest:I | grep -q '#$id .*: {'" ||
		{ echo "# no answer to $1" >&2; return 1; }
	adb logcat -d -s OffLokTest:I | grep "#$id .*: {" | tail -1 | sed 's/^[^{]*//'
}
state_is() { core status | grep -Eq "\"state\":\"($1)\""; }
# ping looks the name up with the phone's resolver and prints the address; the ping itself may go unanswered.
lookup() { adb shell ping -c 1 -W 1 "$1" 2>&1 | sed -n 's/^PING [^ ]* (\([^)]*\)).*/\1/p'; }
resolves() { [ -n "$(lookup "$1")" ]; }
refused() { [ -z "$(lookup "$1")" ]; }
blocked() { [ "$(lookup "$1")" = 0.0.0.0 ]; }
tunnel() { adb shell ip -brief addr show tun0 2>/dev/null | grep -q '10\.111\.222\.2/32'; }
no_tunnel() { ! tunnel; }

adb install -r "$apk" >/dev/null || exit 1
adb shell appops set "$pkg" ACTIVATE_VPN allow
adb shell am start -W -n "$pkg/.MainActivity" >/dev/null
check 'off at first' wait_until 30 state_is off
check 'the phone resolves before' wait_until 60 resolves example.org

core on >/dev/null
check 'on: resolves by itself, or falls back on this network' wait_until 60 state_is 'direct|fallback-network'
echo "# $(core status | grep -o '"state":"[a-z-]*"'), tunnel: $(adb shell ip -brief addr show tun0 2>&1)"
check 'the tunnel is up' tunnel
check 'the phone resolves through OffLok' wait_until 20 resolves nlnetlabs.nl
check 'a broken DNSSEC signature is refused' refused dnssec-failed.org

core block ads=on >/dev/null
check 'ads: the list is downloaded' wait_until 90 sh -c "adb logcat -d | grep -q 'AdGuard DNS filter: [0-9]* names'"
check 'ads: doubleclick.net is blocked' wait_until 15 blocked doubleclick.net
echo "# doubleclick.net: ping says $(adb shell ping -c 1 -W 1 doubleclick.net 2>&1 | head -2 | tr '\n' '|')"
echo "# OffLok's check: $(core check doubleclick.net | grep -o '"data":{[^}]*}')"
core block ads=off >/dev/null

core off >/dev/null
check 'off' wait_until 30 state_is off
check 'off: the tunnel is gone' wait_until 15 no_tunnel
check 'off: the phone resolves without OffLok' resolves example.org

if [ "$failed" -gt 0 ]; then
	echo "# OffLok's log:"
	adb logcat -d -s OffLok:I OffLokTest:I AndroidRuntime:E | cut -c 1-300 | tail -n 80
fi
echo "# $passed passed, $failed failed"
[ "$failed" -eq 0 ]

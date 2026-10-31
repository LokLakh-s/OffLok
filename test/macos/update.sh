#!/bin/bash
# OffLok's update on macOS, from end to end, on a disposable Mac (GitHub's runner, where sudo asks no password).
# Test release A (a test build, which trusts test/release/'s key and reads OFFLOK_UPDATE_BASE) is installed; it
# finds release B on a local server, checks its signature and hash and runs its package with macOS's installer;
# offlokd, the command and the menu bar app come back as B. Then B is uninstalled. The packages come from
# test/update-builds.sh macos, in dist/update-test.
set -uo pipefail
cd "$(dirname "$0")/../.."
. test/macos/lib.sh

u=dist/update-test
A=$(cat "$u/a.version")
B=$(cat "$u/b.version")

version_is() { [ "$("$offlok" version)" = "$1" ]; }
offlokd_is() { "$offlok" status | grep -q "OffLok $1, Unbound"; }
# The menu bar app runs once, and not as the process it was before the update.
app_restarted() {
	local pids
	pids=$(pgrep -x offlok-desktop)
	[ "$(wc -l <<<"$pids" | tr -d ' ')" -eq 1 ] && [ -n "$pids" ] && [ "$pids" != "$1" ]
}

before=$(dns_snapshot)
sudo installer -pkg "$u/offlok-$A.pkg" -target / || exit 1
check 'A resolves' wait_until 60 status_is 'On|Fallback'
check 'the command is A' version_is "$A"

# The release server: B's package and its SHA256SUMS, signed with the test key.
web=$(mktemp -d)
latest=$web/releases/latest/download
release=$web/releases/download/v$B
mkdir -p "$latest" "$release"
cp "$u/offlok-$B.pkg" "$release/"
(cd "$release" && shasum -a 256 "offlok-$B.pkg") >"$latest/SHA256SUMS"
go run ./test/release/sign test/release/test-release.key.pem "$latest/SHA256SUMS"
port=$(python3 -c 'import socket; s = socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
python3 -m http.server "$port" --bind 127.0.0.1 --directory "$web" >/dev/null 2>&1 &
server=$!
base=http://127.0.0.1:$port/releases
check "the release server answers ($base)" wait_until 20 curl -fsS -o /dev/null "$base/latest/download/SHA256SUMS"

# offlokd takes its releases from there: test builds alone read OFFLOK_UPDATE_BASE. B's package writes launchd's
# file again, without it.
sudo /usr/libexec/PlistBuddy -c 'Add :EnvironmentVariables dict' -c "Add :EnvironmentVariables:OFFLOK_UPDATE_BASE string $base" "$plist"
sudo launchctl bootout system/org.loklakh.offlokd
wait_until 30 launchd_has_none
sudo launchctl bootstrap system "$plist"
wait_until 60 status_is 'On|Fallback' >/dev/null

if ! app_runs; then
	open -a /Applications/OffLok.app --args --hidden
fi
check 'the menu bar app runs' wait_until 20 app_runs
app=$(pgrep -x offlok-desktop | head -1)

answer=$(sudo "$offlok" update 2>&1)
echo "# offlok update: $answer"
check 'B is offered' grep -q "OffLok $B" <<<"$answer"
# The status's System line names offlokd's version; other lines name the update's.
check 'offlokd comes back as B' wait_until 240 offlokd_is "$B"
check 'B resolves' wait_until 60 status_is 'On|Fallback'
check 'the command is B' version_is "$B"
check 'the menu bar app started again, from B' wait_until 60 app_restarted "$app"
check 'programs resolve through the system' bash -c 'dscacheutil -q host -a name nlnetlabs.nl | grep -q ip_address'
echo "# $("$offlok" status | tr '\n' '|')"

# What tells why, before the uninstall takes it away.
if [ "$failed" -gt 0 ]; then
	state="/Library/Application Support/OffLok"
	echo "# offlokd's log:" && sudo tail -n 40 "$state/offlokd.log" "$state/offlokd.err" 2>/dev/null
	echo "# the installer, during the update:" && sudo cat "$state/updates/installer.log" "$state/updates/installer.status" 2>/dev/null
	echo "# launchd:" && sudo launchctl print system/org.loklakh.offlokd 2>&1 | grep -E 'state =|last exit|pid =' | head -5
	echo "# the package's scripts:" && grep -E 'postinstall|offlok' /var/log/install.log | tail -n 25
fi

kill "$server"
sudo "$libexec/uninstall.sh"
check 'uninstall takes offlokd out of launchd' launchd_has_none
after=$(dns_snapshot)
check 'every network service has its original DNS back' [ "$before" = "$after" ]

summary

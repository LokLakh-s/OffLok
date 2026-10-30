#!/bin/bash
# offlok-desktop on a virtual screen. run.sh desktop runs it as root inside a disposable container: the offlok and
# offlok-desktop packages installed, a user who is not root starts the app, which talks to the real offlokd;
# screenshots land in /shots for a look.
set -u
. /lib.sh
setup_network
mkdir -p /shots /tmp/alice-run
chown alice /tmp/alice-run
chmod 700 /tmp/alice-run
dpkg -i /tmp/offlok_*_amd64.deb /tmp/offlok-desktop_*_amd64.deb >/dev/null
check "the packages install" sh -c 'dpkg -s offlok offlok-desktop | grep -c "^Status: install ok installed" | grep -qx 2'
check "the session starts the tray icon" grep -qx "Exec=offlok-desktop --hidden" /etc/xdg/autostart/offlok-desktop.desktop
check "the menu has OffLok, with its icon" test -e /usr/share/applications/offlok.desktop -a -e /usr/share/icons/hicolor/48x48/apps/offlok.png
check "polkit knows OffLok's action" test -e /usr/share/polkit-1/actions/org.loklakh.offlok.policy
check "the service resolves from the root" wait_state "On:" 30

Xvfb :99 -screen 0 1000x820x24 -nolisten tcp >/tmp/xvfb.log 2>&1 &
sleep 2
shot() { DISPLAY=:99 import -window root "/shots/$1.png"; }
start_app() { # start_app <locale>
	runuser -u alice -- env DISPLAY=:99 LANG="$1" LC_ALL="$1" HOME=/home/alice XDG_RUNTIME_DIR=/tmp/alice-run \
		dbus-run-session -- /usr/bin/offlok-desktop >"/tmp/desktop-$1.log" 2>&1 &
}
stop_app() { pkill -u alice -x offlok-desktop; sleep 1; }
app_runs() { pgrep -u alice -x offlok-desktop >/dev/null; }

start_app fr_FR.UTF-8
sleep 10
check "the app starts and stays up" app_runs
shot fr-direct
stop_app

start_app en_GB.UTF-8
sleep 10
check "the app starts in English" app_runs
shot en-direct

# Blocking on: the window shows the real list and its count.
offlok block ads on >/dev/null
check "ads: the list is downloaded" sh -c 'for i in $(seq 90); do offlok | grep -qE "ads \([0-9]{5,} names\)" && exit 0; sleep 1; done; exit 1'
sleep 4
shot en-blocking
offlok block ads off >/dev/null

# Strict mode with the root out of reach: the window offers to sign in to the network.
offlok strict on >/dev/null
for p in udp tcp; do
	iptables -A OUTPUT -o lo -p $p --dport 53 -j ACCEPT
	iptables -A OUTPUT -p $p --dport 53 -j REJECT
	ip6tables -A OUTPUT -o lo -p $p --dport 53 -j ACCEPT
	ip6tables -A OUTPUT -p $p --dport 53 -j REJECT
done
resolvectl domain eth0 lan home
check "strict mode, root out of reach" wait_state "Strict mode:" 30
sleep 5
shot en-strict
stop_app

# Errors only: the container has no taskbar to take the tray icon (StatusNotifierWatcher) and no accessibility bus.
for f in /tmp/desktop-*.log; do grep -E "ERR|panic|fatal" "$f" | grep -v StatusNotifierWatcher | sed "s|^|# $f: |"; done
dpkg -P offlok-desktop offlok >/dev/null
check "both packages purge" sh -c '! dpkg -s offlok-desktop offlok >/dev/null 2>&1'
summary

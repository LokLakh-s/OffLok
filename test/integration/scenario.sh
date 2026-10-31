#!/bin/bash
# OffLok's main integration scenario. run.sh runs it as root inside a disposable container: systemd,
# systemd-resolved, Unbound from Ubuntu, offlokd installed as a service.
set -u
. /lib.sh
setup_network

systemctl daemon-reload
systemctl enable --now offlokd >/dev/null 2>&1
check "starts and resolves from the root" wait_state "On:" 30
check "systemd-resolved points at OffLok only" sh -c 'resolvectl dns | grep -qx "Global: 127.0.0.1 ::1"'
check "the drop-in is OffLok's" grep -q "^# Written by OffLok" "$dropin"
check "answers are DNSSEC-validated" sh -c 'dig @127.0.0.1 +dnssec iana.org SOA | grep -q "flags:.* ad"'
check "a broken DNSSEC signature is refused" sh -c 'dig @127.0.0.1 dnssec-failed.org A | grep -q "status: SERVFAIL"'
check "programs resolve through the system" getent ahosts nlnetlabs.nl
# By address, in TCP: ss cannot always tell Unbound's process (it runs as unbound, in offlokd's sandbox), and in UDP
# it also lists the sockets Unbound sends its own queries from.
beyond_loopback() { ss -lnt | awk 'NR > 1 {print $4}' | grep -E ":(53|53530)$" | grep -vE "^(127\.[0-9.]+(%[a-z0-9]+)?|\[::1\]):"; }
check "unbound listens on its internal port" sh -c 'ss -lnt | grep -q "127.0.0.1:53530 "'
check "the front listens on the loopback" sh -c 'ss -lntp | grep -q "127.0.0.1:53 .*offlokd"'
if beyond_loopback >/dev/null; then nok "nothing answers DNS beyond the loopback: $(beyond_loopback | tr '\n' ' ')"; else ok "nothing answers DNS beyond the loopback"; fi

capture sh -c 'for n in wikipedia.org debian.org kernel.org gnu.org nas.lan; do getent ahosts $n; done'
check "public names never reach the network's DNS" test "$(leaked)" -eq 0
check "local names go to the network's DNS" grep -q 'nas\.lan\.' /tmp/capture.txt

su alice -s /bin/sh -c 'offlok off' >/dev/null 2>&1
check "a user cannot turn OffLok off" test $? -eq 4
check "a user sees the status" su alice -s /bin/sh -c offlok
check "OffLok stays on" wait_state "On:" 1

# polkit, for a user who is not root, from a desktop (the tray app; offlok --polkit).
su alice -s /bin/sh -c 'offlok --polkit strict on' >/dev/null 2>&1
check "polkit: no rule and no password prompt here, a user is refused" test $? -eq 4
rule=/etc/polkit-1/rules.d/50-offlok-test.rules
printf 'polkit.addRule(function(action, subject) {\n  if (action.id == "org.loklakh.offlok.manage" && subject.user == "alice") return polkit.Result.YES;\n});\n' >"$rule"
sleep 2 # polkit rereads its rules when they change
check "polkit: a rule lets the user change a setting" su alice -s /bin/sh -c 'offlok --polkit strict on'
check "polkit: without --polkit, sudo is still needed" sh -c "su alice -s /bin/sh -c 'offlok strict off' >/dev/null 2>&1; test \$? -eq 4"
rm -f "$rule"
offlok strict off >/dev/null

# The root becomes unreachable: only the network's DNS answers port 53, over IPv4 and IPv6.
for p in udp tcp; do
	iptables -A OUTPUT -o lo -p $p --dport 53 -j ACCEPT
	iptables -A OUTPUT -d "$NET_DNS" -p $p --dport 53 -j ACCEPT
	iptables -A OUTPUT -p $p --dport 53 -j REJECT
	ip6tables -A OUTPUT -o lo -p $p --dport 53 -j ACCEPT
	ip6tables -A OUTPUT -p $p --dport 53 -j REJECT
done
resolvectl domain eth0 lan home # a network change: OffLok probes at once
check "falls back to the network's DNS" wait_state "Fallback:" 30
check "names resolve in fallback" getent ahosts example.org

offlok strict on >/dev/null
check "strict mode never uses the network's DNS" wait_state "Strict mode:" 20
capture dig @127.0.0.1 +time=2 +tries=1 www.debian.org
check "strict mode leaks nothing" test "$(leaked)" -eq 0
check "signin: a user lets the network's DNS through" su alice -s /bin/sh -c 'offlok signin'
check "signin: the network's DNS answers" wait_state "Fallback:" 20
check "signin: the status says until when" sh -c 'offlok | grep -q "let through until"'
offlok strict off >/dev/null
check "strict mode off: back to the fallback" wait_state "Fallback:" 20

iptables -F OUTPUT
ip6tables -F OUTPUT
check "the root is back: resolves by itself again" wait_state "On:" 45

# Blocking: the real AdGuard DNS filter, downloaded by offlokd, and a custom list served here.
mkdir -p /srv/lists
printf '||blocked-by-me.example^\n0.0.0.0 also-me.example\n' >/srv/lists/mine.txt
busybox httpd -p 127.0.0.1:8099 -h /srv
not_blocked() { dig @127.0.0.1 +short +time=3 "$1" A | grep -q '^[0-9]' && ! dig @127.0.0.1 +short "$1" A | grep -qx 0.0.0.0; }
is_blocked() { dig @127.0.0.1 +short "$1" A | grep -qx 0.0.0.0; }
check "blocking is off at first" not_blocked doubleclick.net
offlok block ads on >/dev/null
check "ads: the list is downloaded" sh -c 'for i in $(seq 90); do offlok | grep -qE "(ads|pubs) \([0-9]{5,} names\)" && exit 0; sleep 1; done; exit 1'
# The list counts once downloaded; it blocks once compiled, a moment later.
check "ads: doubleclick.net is blocked" sh -c 'for i in $(seq 15); do dig @127.0.0.1 +short doubleclick.net A | grep -qx 0.0.0.0 && exit 0; sleep 1; done; exit 1'
check "ads: and what is under it" sh -c 'dig @127.0.0.1 +short stats.g.doubleclick.net AAAA | grep -qx ::'
check "ads: through the system too" sh -c 'getent ahostsv4 doubleclick.net | grep -q "^0\.0\.0\.0"'
check "ads: other sites still answer" not_blocked nlnetlabs.nl
check "check names the list" sh -c 'offlok check https://ad.doubleclick.net/x | grep -q "AdGuard DNS filter"'
check "a user may pause" su alice -s /bin/sh -c 'offlok pause'
check "paused: let through" not_blocked doubleclick.net
offlok pause off >/dev/null
check "end of the pause: blocked again" is_blocked doubleclick.net
su alice -s /bin/sh -c 'offlok trust doubleclick.net' >/dev/null 2>&1
check "trusting a site needs an administrator" test $? -eq 4
offlok trust https://www.doubleclick.net/ >/dev/null
check "a trusted site passes" not_blocked www.doubleclick.net
check "its parent stays blocked" is_blocked doubleclick.net
offlok untrust www.doubleclick.net >/dev/null
offlok block custom on >/dev/null
offlok list add http://127.0.0.1:8099/lists/mine.txt >/dev/null
check "custom list: blocked" sh -c 'for i in $(seq 30); do dig @127.0.0.1 +short blocked-by-me.example A | grep -qx 0.0.0.0 && exit 0; sleep 1; done; exit 1'
check "custom list: the hosts line too" is_blocked also-me.example
check "offlokd stays small with the lists" sh -c 'test $(awk "/VmRSS/ {print \$2}" /proc/$(pgrep -x offlokd)/status) -lt 120000'
echo "#   offlokd: $(awk '/VmRSS/ {print $2/1024 " MB"}' /proc/$(pgrep -x offlokd)/status), unbound: $(awk '/VmRSS/ {print $2/1024 " MB"}' /proc/$(pgrep -x unbound)/status)"
offlok block ads off >/dev/null
offlok block custom off >/dev/null
check "blocking off again" sh -c 'for i in $(seq 15); do dig @127.0.0.1 +short doubleclick.net A | grep -qx 0.0.0.0 || exit 0; sleep 1; done; exit 1'

offlok off >/dev/null
check "off" wait_state "Off:" 15
check "off removes the drop-in" test ! -e "$dropin"
check "off stops unbound" sh -c '! pgrep -x unbound'
check "off gives systemd-resolved its own DNS back" sh -c '! resolvectl dns | grep -q "^Global: 127.0.0.1"'
offlok on >/dev/null
check "on again" wait_state "On:" 30

systemctl kill -s KILL offlokd
sleep 1
check "a crash gives the DNS back before the restart" sh -c 'journalctl -u offlokd -o cat | grep -q "restore: the system has its original DNS"'
check "after a crash, it comes back by itself" wait_state "On:" 40
check "after a crash, the drop-in is back" test -e "$dropin"

systemctl stop offlokd
check "stopping the service removes the drop-in" test ! -e "$dropin"
check "stopping the service stops unbound" sh -c '! pgrep -x unbound'

# A system without systemd-resolved: OffLok writes /etc/resolv.conf itself, and puts it back.
systemctl disable --now systemd-resolved >/dev/null 2>&1
systemctl mask systemd-resolved >/dev/null 2>&1
printf 'nameserver %s\nsearch lan\n' "$NET_DNS" >/etc/resolv.conf
cp /etc/resolv.conf /tmp/resolv.conf.orig
systemctl start offlokd
check "without systemd-resolved: resolves from the root" wait_state "On:" 30
check "resolv.conf points at OffLok" sh -c 'grep -q "^# Written by OffLok" /etc/resolv.conf && grep -qx "nameserver 127.0.0.1" /etc/resolv.conf'
check "resolv.conf keeps the network's search domain" grep -qx "search lan" /etc/resolv.conf
check "programs resolve through resolv.conf" getent ahosts nlnetlabs.nl
systemctl stop offlokd
check "stopping puts the original resolv.conf back" cmp -s /etc/resolv.conf /tmp/resolv.conf.orig

summary

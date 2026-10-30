# Helpers shared by OffLok's integration scenarios (sourced, never run).
pass=0
fail=0
ok() { echo "ok   - $*"; pass=$((pass + 1)); }
nok() { echo "FAIL - $*"; fail=$((fail + 1)); }
check() { # check <description> <command...>
	local what=$1
	shift
	if "$@" >/dev/null 2>&1; then ok "$what"; else nok "$what"; fi
}
state() { offlok 2>/dev/null | head -1; }
wait_state() { # wait_state <first words of the status> <seconds>
	local i
	for ((i = 0; i < $2; i++)); do
		case "$(state)" in "$1"*) return 0 ;; esac
		sleep 1
	done
	echo "     (status after $2 s: $(state))"
	return 1
}
dropin=/etc/systemd/resolved.conf.d/90-offlok.conf
# capture <command...>: runs the command while recording the DNS packets sent to the network's DNS.
capture() {
	tcpdump -l -n -i eth0 "dst host $NET_DNS and port 53" >/tmp/capture.txt 2>/dev/null &
	local pid=$!
	sleep 1
	"$@" >/dev/null 2>&1
	sleep 1
	kill "$pid"
	wait "$pid" 2>/dev/null
}
leaked() { grep -v '\.lan\.' /tmp/capture.txt | grep -c ' > '; }

# setup_network: Docker's resolver becomes "the network's DNS", handed to systemd-resolved as if DHCP gave it,
# with a local domain.
setup_network() {
	NET_DNS=$(awk '/^nameserver/ {print $2; exit}' /etc/resolv.conf)
	echo "# network DNS: $NET_DNS"
	printf 'nameserver 127.0.0.53\noptions edns0 trust-ad\n' >/etc/resolv.conf
	for ((i = 0; i < 30; i++)); do resolvectl status >/dev/null 2>&1 && break; sleep 1; done
	resolvectl dns eth0 "$NET_DNS"
	resolvectl domain eth0 lan
	id alice >/dev/null 2>&1 || useradd -m alice
}

summary() {
	echo "# $pass passed, $fail failed"
	[ "$fail" -eq 0 ]
}

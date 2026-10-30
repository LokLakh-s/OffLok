#!/bin/sh
# After install or upgrade: start offlokd.
#
# Debian and Ubuntu start the distribution's unbound.service as soon as the unbound package is installed, and it
# takes 127.0.0.1:53. When its configuration is the package's own, untouched, it is stopped and disabled, and a
# marker lets the removal scripts turn it back on. A configured unbound.service is someone's resolver: it is left
# alone, and offlokd reports that the port is taken.
set -e
marker=/var/lib/offlok/disabled-unbound-service

unbound_untouched() {
	command -v dpkg >/dev/null 2>&1 || return 1
	# dpkg --verify prints a line for every file changed or missing since install. Only /etc matters: minimal
	# systems (containers) drop the documentation.
	[ -z "$(dpkg --verify unbound 2>/dev/null | grep ' /etc/')" ] || return 1
	shipped=$(dpkg -L unbound | grep '^/etc/unbound/unbound.conf.d/.' | sort)
	present=$(find /etc/unbound/unbound.conf.d -mindepth 1 2>/dev/null | sort)
	[ "$shipped" = "$present" ]
}

[ -d /run/systemd/system ] || exit 0
mkdir -p /var/lib/offlok
if systemctl is-enabled --quiet unbound.service 2>/dev/null && unbound_untouched; then
	systemctl disable --now unbound.service
	touch "$marker"
fi
systemctl daemon-reload
systemctl enable offlokd.service
systemctl restart offlokd.service

#!/bin/sh
# After removal: turn back on the unbound.service that postinstall turned off, and on purge (Debian) or erase (RPM)
# delete OffLok's files. Upgrades change nothing.
set -e
case "$1" in upgrade | 1) exit 0 ;; esac
marker=/var/lib/offlok/disabled-unbound-service
if [ -d /run/systemd/system ]; then
	systemctl daemon-reload || true
	if [ -e "$marker" ] && [ -e /usr/lib/systemd/system/unbound.service -o -e /lib/systemd/system/unbound.service ]; then
		systemctl enable --now unbound.service || true
	fi
fi
case "$1" in purge | 0)
	rm -rf /var/lib/offlok /var/lib/unbound/offlok
	;;
esac

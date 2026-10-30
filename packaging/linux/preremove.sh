#!/bin/sh
# Before removal: stop offlokd, which gives the system its original DNS back. Upgrades keep it running (the new
# package's postinstall restarts it). Debian passes "upgrade"; RPM passes how many versions remain (1 = upgrade).
set -e
case "$1" in upgrade | 1) exit 0 ;; esac
if [ -d /run/systemd/system ]; then
	systemctl disable --now offlokd.service || true
fi
/usr/bin/offlokd restore || true

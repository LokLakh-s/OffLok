#!/bin/sh
# Removes OffLok from this Mac: offlokd gives every network service its original DNS back and leaves launchd, then
# OffLok's programs, files and hidden account go. Run it as an administrator:
#   sudo /usr/local/libexec/offlok/uninstall.sh
set -e
if [ "$(id -u)" != 0 ]; then
	echo "Run it with sudo: sudo $0" >&2
	exit 1
fi
for uid in $(ps -axo uid=,comm= | awk '/offlok-desktop$/ {print $1}' | sort -u); do
	launchctl bootout "gui/$uid/org.loklakh.offlok.desktop" 2>/dev/null || true
done
pkill -x offlok-desktop 2>/dev/null || true
/usr/local/libexec/offlok/offlokd uninstall
rm -rf /Applications/OffLok.app /Library/LaunchAgents/org.loklakh.offlok.desktop.plist /usr/local/bin/offlok \
	/usr/local/libexec/offlok
pkgutil --forget org.loklakh.offlok >/dev/null 2>&1 || true
echo "OffLok is removed: every network service has its original DNS back."

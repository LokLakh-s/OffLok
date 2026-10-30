#!/bin/bash
# The .deb's install and removal scripts. run.sh package runs it as root inside a disposable container.
set -u
. /lib.sh
setup_network
deb=$(ls /tmp/offlok_*_amd64.deb)
dropin=/etc/systemd/resolved.conf.d/90-offlok.conf

# The distribution's unbound.service, configured as shipped, holds 127.0.0.1:53.
systemctl enable --now unbound.service >/dev/null 2>&1
check "the distribution's unbound.service runs" systemctl is-active unbound.service
dpkg -i "$deb" >/dev/null
check "install: offlokd runs" systemctl is-active offlokd
check "install: the untouched unbound.service is turned off" sh -c '! systemctl is-enabled unbound.service'
check "install: resolves from the root" wait_state "On:" 30
dpkg -r offlok >/dev/null
check "remove: offlokd is gone" sh -c '! systemctl is-active offlokd'
check "remove: the drop-in is gone" test ! -e "$dropin"
check "remove: unbound.service is back" systemctl is-active unbound.service
dpkg -P offlok >/dev/null
check "purge: OffLok's files are gone" test ! -e /var/lib/offlok -a ! -e /var/lib/unbound/offlok

# A configured unbound.service is someone's resolver: left alone, and OffLok says the port is taken.
echo "server:" >/etc/unbound/unbound.conf.d/mine.conf
systemctl restart unbound.service
dpkg -i "$deb" >/dev/null
check "a configured unbound.service is left alone" systemctl is-active unbound.service
check "offlokd reports the port as taken" sh -c 'for i in $(seq 20); do offlok | grep -q "already used" && exit 0; sleep 1; done; exit 1'
check "and leaves the system DNS alone" test ! -e "$dropin"
dpkg -P offlok >/dev/null
check "purge with a configured unbound: it keeps running" systemctl is-active unbound.service

summary

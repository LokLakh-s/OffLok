#!/bin/bash
# The update, from end to end: OffLok A (a test build, -tags offlok_testkey) installed from its .deb finds release
# B on a local server, checks its signature and each file's hash, and has apt install it in a unit of its own,
# which restarts offlokd. A release signed by another key, a package that is not the signed one and the same
# version served again are refused. run.sh update runs it as root inside a disposable container, with /update/
# holding both test builds' .deb and the test key.
set -u
. /lib.sh
setup_network
A=$(cat /update/a.version) B=$(cat /update/b.version)
web=/srv/releases
latest=$web/latest/download
mkdir -p "$latest" "$web/download/v$B"
cp "/update/offlok_${B}_amd64.deb" "$web/download/v$B/"
busybox httpd -p 127.0.0.1:8080 -h /srv
# publish <key>: B's SHA256SUMS, signed with key.
publish() {
	(cd "$web/download/v$B" && sha256sum "offlok_${B}_amd64.deb") >"$latest/SHA256SUMS"
	openssl pkeyutl -sign -inkey "$1" -rawin -in "$latest/SHA256SUMS" -out "$latest/SHA256SUMS.sig"
}
installed() { dpkg-query -W -f='${Version}' offlok; }
# offlokd takes its releases from the local server: only test builds read OFFLOK_UPDATE_BASE.
mkdir -p /etc/systemd/system/offlokd.service.d
printf '[Service]\nEnvironment=OFFLOK_UPDATE_BASE=http://127.0.0.1:8080/releases\n' \
	>/etc/systemd/system/offlokd.service.d/update-test.conf

apt-get install -y "/update/offlok_${A}_amd64.deb" >/dev/null 2>&1
check "A is installed and resolves from the root" wait_state "On:" 30
# The status's System line names offlokd's version; other lines name the update's.
check "the status names A and the update check" sh -c "offlok | grep -q 'OffLok $A, Unbound' && offlok | grep -q 'Updates: *on'"
check "update is an administrator's" sh -c '! su alice -c "offlok update"'

openssl genpkey -algorithm ed25519 -out /tmp/other.key.pem
publish /tmp/other.key.pem
check "a release signed by another key is refused" sh -c 'offlok update 2>&1 | grep -q "signature"'

publish /update/test-release.key.pem
echo "not the package" >>"$web/download/v$B/offlok_${B}_amd64.deb"
check "a package that is not the signed one is refused" sh -c '
	offlok update >/dev/null || exit 1
	for i in $(seq 30); do offlok | grep -q "SHA-256" && exit 0; sleep 1; done; exit 1'
check "and nothing was installed" test "$(installed)" = "$A"

cp "/update/offlok_${B}_amd64.deb" "$web/download/v$B/"
check "B is offered" sh -c "offlok update | grep -q 'OffLok $B'"
check "apt installs B, which restarts offlokd" sh -c "for i in \$(seq 90); do offlok 2>/dev/null | grep -q 'OffLok $B, Unbound' && exit 0; sleep 1; done; exit 1"
check "the package is B" test "$(installed)" = "$B"
check "B resolves from the root" wait_state "On:" 30
check "names resolve through the system again" sh -c 'dig +short +time=5 nlnetlabs.nl A @127.0.0.53 | grep -q .'
check "the update's unit is gone" sh -c '! systemctl list-units --all "offlok-update-*" --no-legend | grep -q .'

check "the same version served again is no update" sh -c 'offlok update 2>&1 | grep -q "latest version"'
offlok updates off >/dev/null
check "updates off shows in the status" sh -c 'offlok | grep -q "Updates: *off"'

summary

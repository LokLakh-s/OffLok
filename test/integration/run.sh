#!/bin/bash
# OffLok's integration tests: builds the test image, starts a disposable container with systemd, then
#   run.sh          installs the binaries from bin/amd64/ and runs scenario.sh,
#   run.sh package  installs dist/offlok_*_amd64.deb with dpkg and runs package.sh,
#   run.sh desktop  installs dist/'s amd64 offlok and offlok-desktop packages on a virtual screen and runs
#                   desktop.sh, its screenshots in bin/shots/,
#   run.sh update   installs test release A from dist/update-test/ (test/update-builds.sh linux) and runs
#                   update.sh, which updates it to test release B,
# and removes the container. It changes nothing on the machine running it: the container has its own network
# namespace, resolver and firewall.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
name=offlok-it-$$

target=base image=offlok-it
if [ "${1:-}" = desktop ]; then target=desktop image=offlok-it-desktop; fi
docker build -q --target "$target" -t "$image" "$here" >/dev/null
# systemd as PID 1 without --privileged: SYS_ADMIN remounts the container's own cgroup tree (private namespace)
# writable, NET_ADMIN allows the scenario's firewall rules, and Docker's default AppArmor profile forbids mount.
# /proc/sys and the host's devices stay out of reach.
docker run -d --name "$name" --cgroupns=private --cap-add SYS_ADMIN --cap-add NET_ADMIN \
  --security-opt apparmor=unconfined --tmpfs /run --tmpfs /run/lock --cpus 1 --memory 1g "$image" >/dev/null
# OFFLOK_IT_KEEP=1 keeps the container for a look afterwards (docker exec -it <name> bash, then docker rm -f).
if [ "${OFFLOK_IT_KEEP:-}" = 1 ]; then
	echo "# container: $name"
else
	trap 'docker rm -f "$name" >/dev/null' EXIT
fi
# Wait for systemd and D-Bus: docker exec can come before PID 1 is up.
for _ in $(seq 60); do
	case "$(docker exec "$name" systemctl is-system-running 2>/dev/null)" in running | degraded) break ;; esac
	sleep 0.5
done

docker cp "$here/lib.sh" "$name:/lib.sh"
install_binaries() {
	docker cp "$repo/bin/amd64/offlokd" "$name:/usr/bin/offlokd"
	docker cp "$repo/bin/amd64/offlok" "$name:/usr/bin/offlok"
	docker cp "$repo/packaging/linux/offlokd.service" "$name:/etc/systemd/system/offlokd.service"
	docker cp "$repo/packaging/linux/org.loklakh.offlok.policy" "$name:/usr/share/polkit-1/actions/"
}
if [ "${1:-}" = desktop ]; then
	docker cp "$(ls "$repo"/dist/offlok_*_amd64.deb)" "$name:/tmp/"
	docker cp "$(ls "$repo"/dist/offlok-desktop_*_amd64.deb)" "$name:/tmp/"
	docker cp "$here/desktop.sh" "$name:/desktop.sh"
	status=0
	docker exec "$name" bash /desktop.sh || status=$?
	mkdir -p "$repo/bin/shots"
	docker cp "$name:/shots/." "$repo/bin/shots/"
	exit $status
elif [ "${1:-}" = update ]; then
	u=$repo/dist/update-test
	A=$(cat "$u/a.version") B=$(cat "$u/b.version")
	docker exec "$name" mkdir /update
	for f in a.version b.version "offlok_${A}_amd64.deb" "offlok_${B}_amd64.deb"; do docker cp "$u/$f" "$name:/update/"; done
	docker cp "$repo/test/release/test-release.key.pem" "$name:/update/"
	docker cp "$here/update.sh" "$name:/update.sh"
	docker exec "$name" bash /update.sh
elif [ "${1:-}" = package ]; then
	docker cp "$(ls "$repo"/dist/offlok_*_amd64.deb)" "$name:/tmp/"
	docker cp "$here/package.sh" "$name:/package.sh"
	docker exec "$name" bash /package.sh
else
	install_binaries
	docker cp "$here/scenario.sh" "$name:/scenario.sh"
	docker exec "$name" bash /scenario.sh
fi

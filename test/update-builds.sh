#!/bin/bash
# Builds the update tests' two releases, test builds (-tags offlok_testkey) that trust test/release/'s key: A, this
# version, and B, the next patch, into dist/update-test/ with a.version and b.version.
#   test/update-builds.sh linux    the amd64 .deb (OffLok's Go image), for test/integration/run.sh update
#   test/update-builds.sh windows  the installers (packaging/linux/desktop-build's image), for test/windows/update.ps1
set -euo pipefail
cd "$(dirname "$0")/.."
A=$(cat VERSION)
B=$(awk -F. '{ print $1 "." $2 "." $3 + 1 }' VERSION)
mkdir -p dist/update-test
echo "$A" >dist/update-test/a.version
echo "$B" >dist/update-test/b.version
export GOFLAGS="${GOFLAGS:-} -tags=offlok_testkey" DIST=dist/update-test
for v in "$A" "$B"; do
	case "$1" in
	linux) VERSION=$v ARCHS=amd64 BIN=bin/update-test ./packaging/linux/build.sh ;;
	windows) VERSION=$v ./packaging/windows/build.sh ;;
	*) echo "usage: $0 linux|windows" >&2 && exit 2 ;;
	esac
done

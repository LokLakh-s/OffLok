#!/bin/bash
# Builds offlok-desktop for Linux amd64 into bin/amd64/, then its .deb and .rpm into dist/. Runs in the image of
# packaging/linux/desktop-build/ (Go, GTK 3 and WebKitGTK 4.1's development files): cgo links them.
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION=${VERSION:-$(cat VERSION)}
nfpm="go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.48.0"
(cd desktop && CGO_ENABLED=1 go build -tags gtk3 -trimpath -ldflags "-s -w -X main.version=$VERSION" -o ../bin/amd64/offlok-desktop .)
mkdir -p dist
for packager in deb rpm; do
	VERSION=$VERSION $nfpm package --config packaging/linux/nfpm-desktop.yaml --packager $packager --target dist/
done
ls -l dist/offlok-desktop*

#!/bin/bash
# Builds offlokd and offlok for Linux amd64 and arm64 into bin/<arch>/, then the .deb and .rpm packages into dist/.
# nfpm takes no variables in source paths: each architecture's binaries go through bin/pkg/ in turn.
# Needs Go; nfpm runs through "go run" at a pinned version. DIST, BIN and ARCHS change where and what it builds,
# and GOFLAGS passes through: the update tests build packages with -tags=offlok_testkey apart.
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION=${VERSION:-$(cat VERSION)}
DIST=${DIST:-dist} BIN=${BIN:-bin} ARCHS=${ARCHS:-amd64 arm64}
nfpm="go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.48.0"
mkdir -p "$DIST"
for arch in $ARCHS; do
	CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
		-o "$BIN/$arch/" ./cmd/offlokd ./cmd/offlok
	mkdir -p bin/pkg
	cp "$BIN/$arch/offlokd" "$BIN/$arch/offlok" bin/pkg/
	for packager in deb rpm; do
		ARCH=$arch VERSION=$VERSION $nfpm package --config packaging/linux/nfpm.yaml --packager $packager --target "$DIST/"
	done
done
ls -l "$DIST"

#!/bin/bash
# Builds OffLok for Windows amd64 on Linux: offlokd.exe, offlok.exe, offlok-desktop.exe (no cgo: Wails on Windows
# needs none), NLnet Labs' unbound.exe (pinned in unbound.env, its checksum checked), and the installer
# $DIST/offlok-<version>-setup.exe (NSIS; DIST is dist/ by default). Runs in packaging/linux/desktop-build's image
# (Go, NSIS, unzip). GOFLAGS passes through: the update tests build with -tags=offlok_testkey.
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION=${VERSION:-$(cat VERSION)}
DIST=${DIST:-dist}
. packaging/windows/unbound.env
stage=$DIST/windows/stage
rm -rf "$stage"
mkdir -p "$stage/unbound" dist/windows
ld="-s -w -X main.version=$VERSION"

# Each program carries the product's name and version (SignPath asks for them) and the mark as its icon; the
# window's program also a manifest for sharp text on high-DPI screens. The resource files are for this build only.
winres="go run github.com/tc-hib/go-winres@v0.3.3"
trap 'rm -f cmd/offlokd/rsrc_windows_amd64.syso cmd/offlok/rsrc_windows_amd64.syso desktop/rsrc_windows_amd64.syso' EXIT
resources() { # <package directory> <file name> <description> <manifest>
	$winres simply --arch amd64 --out "$1/rsrc" --manifest "$4" --icon packaging/windows/offlok.ico \
		--product-name OffLok --product-version "$VERSION" --file-version "$VERSION" --file-description "$3" \
		--copyright "MIT licence, LokLakh" --original-filename "$2"
}
resources cmd/offlokd offlokd.exe "OffLok service" cli
resources cmd/offlok offlok.exe "OffLok command" cli
resources desktop offlok-desktop.exe "OffLok" gui
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ld" -o "$stage/" ./cmd/offlokd ./cmd/offlok
(cd desktop && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ld -H windowsgui" \
	-o "../$stage/offlok-desktop.exe" .)
zip=dist/windows/unbound-$UNBOUND_VERSION.zip # shared by every DIST
[ -f "$zip" ] || curl -fsSL -o "$zip" "https://nlnetlabs.nl/downloads/unbound/unbound-$UNBOUND_VERSION.zip"
echo "$UNBOUND_SHA256  $zip" | sha256sum -c --quiet -
unzip -q -o -j "$zip" unbound.exe LICENSE -d "$stage/unbound"
sed 's/$/\r/' LICENSE >"$stage/LICENSE.txt" # the license page shows CRLF text
cp packaging/windows/offlok.ico "$stage/"
makensis -V2 -WX -DVERSION="$VERSION" -DSTAGE="$PWD/$stage" -DOUTFILE="$PWD/$DIST/offlok-$VERSION-setup.exe" \
	packaging/windows/offlok.nsi
ls -l "$DIST/offlok-$VERSION-setup.exe"

#!/bin/bash
# OffLok's macOS package, built on a Mac after build-unbound.sh: dist/offlok-<version>.pkg, for Apple silicon and
# Intel, macOS 13 or later. Unsigned (no Apple Developer account yet): the site says how to open it. It installs
#   /usr/local/libexec/offlok/   offlokd, Unbound, uninstall.sh
#   /usr/local/bin/offlok        the command
#   /Applications/OffLok.app     the menu bar icon and its window, started at every login by
#   /Library/LaunchAgents/org.loklakh.offlok.desktop.plist
# and its script has launchd start offlokd (offlokd install). VERSION and DIST (dist/ by default) may be given: the
# update test builds two versions (test/update-builds.sh macos).
set -euo pipefail
cd "$(dirname "$0")/../.."
VERSION=${VERSION:-$(cat VERSION)}
DIST=${DIST:-dist}
export MACOSX_DEPLOYMENT_TARGET=13.0
mac=packaging/macos
build=bin/darwin/pkg
root=$build/root
app=$root/Applications/OffLok.app
rm -rf "$build"
mkdir -p "$root/usr/local/libexec/offlok/unbound" "$root/usr/local/bin" "$app/Contents/MacOS" \
	"$app/Contents/Resources" "$root/Library/LaunchAgents" "$DIST"

# The programs, for both processors, then one universal file each.
ldflags="-s -w -X main.version=$VERSION"
for arch in arm64 amd64; do
	cc_arch=$([ "$arch" = amd64 ] && echo x86_64 || echo arm64)
	GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$build/$arch/offlokd" ./cmd/offlokd
	GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$build/$arch/offlok" ./cmd/offlok
	(cd desktop && GOOS=darwin GOARCH=$arch CGO_ENABLED=1 CC="clang -arch $cc_arch" \
		go build -trimpath -ldflags "$ldflags" -o "../$build/$arch/offlok-desktop" .)
done
lipo -create "$build/arm64/offlokd" "$build/amd64/offlokd" -output "$root/usr/local/libexec/offlok/offlokd"
lipo -create "$build/arm64/offlok" "$build/amd64/offlok" -output "$root/usr/local/bin/offlok"
lipo -create "$build/arm64/offlok-desktop" "$build/amd64/offlok-desktop" -output "$app/Contents/MacOS/offlok-desktop"
cp bin/darwin/unbound/unbound bin/darwin/unbound/LICENSE bin/darwin/unbound/OPENSSL-LICENSE.txt \
	"$root/usr/local/libexec/offlok/unbound/"
install -m 755 "$mac/uninstall.sh" "$root/usr/local/libexec/offlok/uninstall.sh"
cp "$mac/org.loklakh.offlok.desktop.plist" "$root/Library/LaunchAgents/"

# The app: its Info.plist (a menu bar app, no Dock icon) and its icon, the mark on a tile (scripts/brand-icons.py).
sed "s/@VERSION@/$VERSION/g" "$mac/Info.plist" >"$app/Contents/Info.plist"
iconutil -c icns "$mac/offlok.iconset" -o "$app/Contents/Resources/offlok.icns"

# The component package, then the installer around it: its welcome page (what OffLok changes and sends) and the
# licence, in English and French, following the Mac's language.
pkgbuild --root "$root" --scripts "$mac/scripts" --identifier org.loklakh.offlok --version "$VERSION" \
	--install-location / --ownership recommended "$build/offlok-component.pkg"
resources=$build/resources
cp -R "$mac/resources" "$resources"
for lang in en fr; do cp LICENSE "$resources/$lang.lproj/LICENSE.txt"; done
sed "s/@VERSION@/$VERSION/g" "$mac/Distribution.xml" >"$build/Distribution.xml"
productbuild --distribution "$build/Distribution.xml" --resources "$resources" --package-path "$build" \
	"$DIST/offlok-$VERSION.pkg"
ls -l "$DIST/offlok-$VERSION.pkg"

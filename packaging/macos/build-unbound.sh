#!/bin/bash
# Unbound for the macOS package, built on a Mac from NLnet Labs' source with OpenSSL 3.5 linked in: one universal
# program (Apple silicon and Intel) that needs only macOS's own libraries, its libexpat among them. The sources are
# pinned in sources.env with their SHA-256. Into bin/darwin/unbound/: unbound, its licence and OpenSSL's.
set -euo pipefail
cd "$(dirname "$0")/../.."
. packaging/macos/sources.env
work=$PWD/bin/darwin/unbound-build
out=$PWD/bin/darwin/unbound
# Go 1.27's own minimum: the package asks for no older macOS.
export MACOSX_DEPLOYMENT_TARGET=13.0
jobs=$(sysctl -n hw.ncpu)
mkdir -p "$work" "$out"

fetch() { # url sha256 file
	[ -f "$work/$3" ] || curl -fsSL --retry 3 -o "$work/$3" "$1"
	echo "$2  $work/$3" | shasum -a 256 -c - >/dev/null
}
fetch "https://github.com/openssl/openssl/releases/download/openssl-$OPENSSL_VERSION/openssl-$OPENSSL_VERSION.tar.gz" \
	"$OPENSSL_SHA256" "openssl-$OPENSSL_VERSION.tar.gz"
fetch "https://nlnetlabs.nl/downloads/unbound/unbound-$UNBOUND_VERSION.tar.gz" "$UNBOUND_SHA256" \
	"unbound-$UNBOUND_VERSION.tar.gz"

sdk=$(xcrun --sdk macosx --show-sdk-path)
for arch in arm64 x86_64; do
	# OpenSSL's libraries only (no command, no tests), static: kept between builds of the same version.
	ssl=$work/openssl-$OPENSSL_VERSION-$arch
	if [ ! -f "$ssl/lib/libssl.a" ]; then
		src=$work/openssl-src-$arch
		rm -rf "$src" && mkdir -p "$src"
		tar -xzf "$work/openssl-$OPENSSL_VERSION.tar.gz" -C "$src" --strip-components 1
		(cd "$src" && ./Configure "darwin64-$arch-cc" no-shared no-module no-tests no-docs \
			--prefix="$ssl" --libdir=lib "-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" >/dev/null &&
			make -j"$jobs" build_libs >/dev/null && make install_dev >/dev/null)
	fi
	src=$work/unbound-src-$arch
	rm -rf "$src" && mkdir -p "$src"
	tar -xzf "$work/unbound-$UNBOUND_VERSION.tar.gz" -C "$src" --strip-components 1
	host=$([ "$arch" = arm64 ] && echo aarch64-apple-darwin || echo x86_64-apple-darwin)
	(cd "$src" && CC="clang -arch $arch" CFLAGS="-O2 -mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" \
		./configure --host="$host" --with-ssl="$ssl" --with-libexpat="$sdk/usr" --disable-flto \
		--with-username=_offlok --with-chroot-dir="" --with-pidfile="" >/dev/null &&
		make -j"$jobs" unbound >/dev/null)
	cp "$src/unbound" "$work/unbound-$arch"
done
lipo -create "$work/unbound-arm64" "$work/unbound-x86_64" -output "$out/unbound"
cp "$work/unbound-src-arm64/LICENSE" "$out/LICENSE"
cp "$work/openssl-src-arm64/LICENSE.txt" "$out/OPENSSL-LICENSE.txt" 2>/dev/null ||
	tar -xzf "$work/openssl-$OPENSSL_VERSION.tar.gz" -O "openssl-$OPENSSL_VERSION/LICENSE.txt" >"$out/OPENSSL-LICENSE.txt"
lipo -info "$out/unbound"
otool -L "$out/unbound"
"$out/unbound" -V | head -2

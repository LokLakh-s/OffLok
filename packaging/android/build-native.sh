#!/bin/bash
# libunbound and OpenSSL for the Android app, built with the NDK from the sources the macOS package pins
# (packaging/macos/sources.env: Unbound's and OpenSSL's, each checked by its SHA-256): static libraries per ABI, into
# internal/libunbound/_native/<abi>/, which cgo links into the core (engine.go's #cgo lines). Needs
# ANDROID_NDK_HOME. Kept between builds of the same versions.
#
# Unbound's configure insists on expat.h, which only unbound-anchor uses: libunbound alone (--with-libunbound-only)
# is built, against an empty expat.h.
set -euo pipefail
cd "$(dirname "$0")/../.."
. packaging/macos/sources.env
. packaging/android/android.env
ndk=${ANDROID_NDK_HOME:?the NDK, version $NDK_VERSION}
grep -q "^Pkg.Revision = $NDK_VERSION\$" "$ndk/source.properties" ||
	{ echo "$ndk is not NDK $NDK_VERSION" >&2; exit 1; }
tc=$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin
work=$PWD/bin/android/native-build
out=$PWD/internal/libunbound/_native
jobs=$(nproc)
mkdir -p "$work" "$out"

fetch() { # url sha256 file
	[ -f "$work/$3" ] || curl -fsSL --retry 3 -o "$work/$3" "$1"
	echo "$2  $work/$3" | sha256sum -c - >/dev/null
}
fetch "https://github.com/openssl/openssl/releases/download/openssl-$OPENSSL_VERSION/openssl-$OPENSSL_VERSION.tar.gz" \
	"$OPENSSL_SHA256" "openssl-$OPENSSL_VERSION.tar.gz"
fetch "https://nlnetlabs.nl/downloads/unbound/unbound-$UNBOUND_VERSION.tar.gz" "$UNBOUND_SHA256" \
	"unbound-$UNBOUND_VERSION.tar.gz"
mkdir -p "$work/expat-stub/include"
echo "/* Empty: only unbound-anchor, which OffLok does not build, uses expat. */" >"$work/expat-stub/include/expat.h"

# The same paths in every build: the libraries carry no trace of where they were built.
flags="-O2 -ffile-prefix-map=$work=. -ffile-prefix-map=$ndk=ndk"
for abi in $NDK_ABIS; do
	case $abi in
	arm64-v8a) target=aarch64-linux-android ossl=android-arm64 ;;
	armeabi-v7a) target=armv7a-linux-androideabi ossl=android-arm ;;
	x86_64) target=x86_64-linux-android ossl=android-x86_64 ;;
	*) echo "unknown ABI $abi" >&2; exit 1 ;;
	esac
	stamp=$out/$abi/.built
	want="NDK $NDK_VERSION, API $ANDROID_MIN_API, OpenSSL $OPENSSL_VERSION, Unbound $UNBOUND_VERSION"
	if [ "$(cat "$stamp" 2>/dev/null)" = "$want" ]; then
		echo "$abi: $want, built already"
		continue
	fi
	prefix=$work/prefix-$abi
	rm -rf "$prefix" "$out/$abi"

	src=$work/openssl-src-$abi
	rm -rf "$src" && mkdir -p "$src"
	tar -xzf "$work/openssl-$OPENSSL_VERSION.tar.gz" -C "$src" --strip-components 1
	(cd "$src" && PATH=$tc:$PATH ANDROID_NDK_ROOT=$ndk ./Configure "$ossl" "-D__ANDROID_API__=$ANDROID_MIN_API" \
		no-shared no-module no-tests no-docs no-apps --prefix="$prefix" --libdir=lib $flags >/dev/null &&
		PATH=$tc:$PATH ANDROID_NDK_ROOT=$ndk make -j"$jobs" build_libs >/dev/null &&
		PATH=$tc:$PATH make install_dev >/dev/null)

	src=$work/unbound-src-$abi
	rm -rf "$src" && mkdir -p "$src"
	tar -xzf "$work/unbound-$UNBOUND_VERSION.tar.gz" -C "$src" --strip-components 1
	(cd "$src" && CC="$tc/$target$ANDROID_MIN_API-clang" AR="$tc/llvm-ar" RANLIB="$tc/llvm-ranlib" \
		STRIP="$tc/llvm-strip" CFLAGS="$flags" ./configure --host="$target" --prefix="$prefix" \
		--with-ssl="$prefix" --with-libexpat="$work/expat-stub" --with-libunbound-only --disable-shared \
		--enable-static --disable-flto --with-pthreads >/dev/null &&
		make -j"$jobs" >/dev/null && make install >/dev/null)

	mkdir -p "$out/$abi/lib" "$out/$abi/include"
	cp "$prefix/lib/libunbound.a" "$prefix/lib/libssl.a" "$prefix/lib/libcrypto.a" "$out/$abi/lib/"
	cp "$prefix/include/unbound.h" "$out/$abi/include/"
	echo "$want" >"$stamp"
	echo "$abi: $want"
done
# The licences the app shows.
mkdir -p "$out/licences"
tar -xzf "$work/unbound-$UNBOUND_VERSION.tar.gz" -O "unbound-$UNBOUND_VERSION/LICENSE" >"$out/licences/UNBOUND-LICENSE.txt"
tar -xzf "$work/openssl-$OPENSSL_VERSION.tar.gz" -O "openssl-$OPENSSL_VERSION/LICENSE.txt" >"$out/licences/OPENSSL-LICENSE.txt"

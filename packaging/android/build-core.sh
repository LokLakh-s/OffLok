#!/bin/bash
# OffLok's Go core for the Android app, as two Android libraries built with gomobile bind (go.mod pins it):
# android/app/libs/offlok-core-full.aar, and offlok-core-play.aar with the noads tag (Google Play's app blocks no
# ads). Needs build-native.sh's libraries, ANDROID_HOME and ANDROID_NDK_HOME.
set -euo pipefail
cd "$(dirname "$0")/../.."
. packaging/android/android.env
tools=$PWD/bin/android/tools
mkdir -p "$tools" android/app/libs
go build -o "$tools/gomobile" golang.org/x/mobile/cmd/gomobile
go build -o "$tools/gobind" golang.org/x/mobile/cmd/gobind
export PATH=$tools:$PATH
targets=$(for abi in $NDK_ABIS; do
	case $abi in
	arm64-v8a) echo android/arm64 ;;
	armeabi-v7a) echo android/arm ;;
	x86_64) echo android/amd64 ;;
	esac
done | paste -sd, -)
for flavor in full play; do
	tags=()
	[ "$flavor" = play ] && tags=(-tags noads)
	gomobile bind -target "$targets" -androidapi "$ANDROID_MIN_API" -javapkg org.loklakh.offlok -trimpath \
		-ldflags "-s -w" "${tags[@]}" -o "android/app/libs/offlok-core-$flavor.aar" ./mobile
	echo "android/app/libs/offlok-core-$flavor.aar: $(du -h "android/app/libs/offlok-core-$flavor.aar" | cut -f1)"
done

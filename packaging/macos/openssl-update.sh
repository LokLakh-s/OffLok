#!/bin/bash
# Looks for an OpenSSL newer than the one the macOS package links into Unbound (sources.env), in the same series
# (3.5, OpenSSL's long-term support release, until 2030-04-08). When OpenSSL released one, checks its OpenPGP
# signature by OpenSSL's key (openssl-release.asc, fingerprint below) and the SHA-256 OpenSSL published, pins it in
# sources.env and adds it to CHANGELOG.md, as unbound-update.sh does for Unbound. Prints the new OpenSSL's version,
# or nothing when the pinned one is the series' latest, and writes the pull request's text to
# dist/openssl-update.md. Needs gh (GitHub's API: OpenSSL publishes its releases there). Run from the repository's
# root; .github/workflows/unbound.yml does the rest.
#   --from <version>: a test, as if that OpenSSL were pinned.
set -euo pipefail
. packaging/release/watch.sh

key=packaging/macos/openssl-release.asc
fpr=B146647E45A7B33947AB226B2A2C87D161692D40 # "OpenSSL <openssl@openssl.org>"

. packaging/macos/sources.env
pinned=$OPENSSL_VERSION
[ "${1:-}" != --from ] || pinned=$2
series=${pinned%.*}

latest=$(gh api 'repos/openssl/openssl/releases?per_page=100' --jq '.[] | select(.prerelease | not) | .tag_name' |
	sed -n "s/^openssl-\(${series//./\\.}\.[0-9][0-9]*\)$/\1/p" | sort -V | tail -n1)
[ -n "$latest" ] || { echo "no OpenSSL $series release found on GitHub" >&2; exit 1; }
if [ "$(printf '%s\n' "$pinned" "$latest" | sort -V | tail -n1)" = "$pinned" ]; then
	echo "OpenSSL $pinned is the latest of $series" >&2
	exit 0
fi
echo "OpenSSL released $latest (pinned: $pinned)" >&2

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tarball=openssl-$latest.tar.gz
for f in "$tarball" "$tarball.asc" "$tarball.sha256"; do
	curl -fsSL -o "$tmp/$f" "https://github.com/openssl/openssl/releases/download/openssl-$latest/$f"
done
check_signature "$key" "$fpr" "$tmp/$tarball" "$tmp/$tarball.asc"
sum=$(sha256sum "$tmp/$tarball" | cut -d' ' -f1)
[ "$sum" = "$(awk '{ print $1 }' "$tmp/$tarball.sha256")" ] ||
	{ echo "$tarball: not the SHA-256 OpenSSL published" >&2; exit 1; }

sed -i -e "s/^OPENSSL_VERSION=.*/OPENSSL_VERSION=$latest/" -e "s/^OPENSSL_SHA256=.*/OPENSSL_SHA256=$sum/" \
	packaging/macos/sources.env

# OpenSSL's release notes name every CVE the release fixes.
cves=$(gh api "repos/openssl/openssl/releases/tags/openssl-$latest" --jq .body | grep -oE 'CVE-[0-9]+-[0-9]+' |
	sort -u | paste -sd, - | sed 's/,/, /g') || cves=
notes=https://github.com/openssl/openssl/releases/tag/openssl-$latest
add_to_changelog "- **macOS:** OpenSSL $latest, linked into Unbound${cves:+, with the fixes for $cves} ([its notes]($notes))."

mkdir -p dist
{
	echo "OpenSSL released $latest; OffLok's macOS package links OpenSSL $pinned into its Unbound."
	echo
	echo "- **Signature:** good, by OpenSSL's key (\`$fpr\`)"
	echo "- **SHA-256:** \`$sum\`, as OpenSSL published it"
	echo "- **CVEs fixed:** ${cves:-none named in its notes}"
	echo "- **OpenSSL's notes:** $notes"
	echo
	release_text
} >dist/openssl-update.md
echo "$latest"

#!/bin/bash
# Looks for an Unbound newer than the one pinned in unbound.env (Windows) and packaging/macos/sources.env (the
# macOS package's source). When NLnet Labs released one, checks the OpenPGP signatures of its Windows build and of
# its source by NLnet Labs' release key (nlnetlabs-releases-g2.asc, fingerprint below) and the SHA-256 they
# published, pins both, and adds it to CHANGELOG.md: in a new patch release when VERSION is released already, in
# VERSION's own section otherwise. Prints the new Unbound's version, or nothing when the pinned one is the latest,
# and writes the pull request's text to dist/unbound-update.md. Run from the repository's root;
# .github/workflows/unbound.yml does the rest.
#   --from <version>: a test, as if that Unbound were pinned.
set -euo pipefail
. packaging/release/watch.sh

base=https://nlnetlabs.nl/downloads/unbound
key=packaging/windows/nlnetlabs-releases-g2.asc
fpr=231018690C4D903EF419146AA144323DEAACDF45 # "NLnet Labs releases signing key G2 <releases@nlnetlabs.nl>"

. packaging/windows/unbound.env
pinned=$UNBOUND_VERSION
[ "${1:-}" != --from ] || pinned=$2

# The 64-bit build is unbound-<version>.zip; -w32 and release candidates are left out.
latest=$(curl -fsSL "$base/" | grep -oE 'href="unbound-[0-9]+\.[0-9]+\.[0-9]+\.zip"' |
	grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | sort -uV | tail -n1)
[ -n "$latest" ] || { echo "no Unbound for Windows found at $base/" >&2; exit 1; }
if [ "$(printf '%s\n' "$pinned" "$latest" | sort -V | tail -n1)" = "$pinned" ]; then
	echo "Unbound $pinned is NLnet Labs' latest" >&2
	exit 0
fi
echo "NLnet Labs released Unbound $latest (pinned: $pinned)" >&2

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
zip=unbound-$latest.zip
tarball=unbound-$latest.tar.gz
for f in "$zip" "$tarball"; do
	for g in "$f" "$f.asc" "$f.sha256"; do curl -fsSL -o "$tmp/$g" "$base/$g"; done
	check_signature "$key" "$fpr" "$tmp/$f" "$tmp/$f.asc"
	[ "$(sha256sum "$tmp/$f" | cut -d' ' -f1)" = "$(cut -d' ' -f1 <"$tmp/$f.sha256")" ] ||
		{ echo "$f: not the SHA-256 NLnet Labs published" >&2; exit 1; }
done
files=$(unzip -Z1 "$tmp/$zip")
for f in unbound.exe LICENSE; do # what build.sh takes from it
	grep -qx "$f" <<<"$files" || { echo "$zip has no $f" >&2; exit 1; }
done
sum=$(sha256sum "$tmp/$zip" | cut -d' ' -f1)
src_sum=$(sha256sum "$tmp/$tarball" | cut -d' ' -f1)

sed -i -e "s/^UNBOUND_VERSION=.*/UNBOUND_VERSION=$latest/" -e "s/^UNBOUND_SHA256=.*/UNBOUND_SHA256=$sum/" \
	packaging/windows/unbound.env
sed -i -e "s/^UNBOUND_VERSION=.*/UNBOUND_VERSION=$latest/" -e "s/^UNBOUND_SHA256=.*/UNBOUND_SHA256=$src_sum/" \
	packaging/macos/sources.env

# NLnet Labs' advisories say "Unbound <version> includes a fix" under each CVE the release fixes.
cves=$(curl -fsSL https://nlnetlabs.nl/projects/unbound/security-advisories/ | sed -E 's/<[^>]+>//g' |
	awk -v fix="Unbound $latest includes a fix" '
		match($0, /CVE-[0-9]+-[0-9]+/) { cve = substr($0, RSTART, RLENGTH) }
		index($0, fix) && cve != "" { print cve; cve = "" }' | sort -u | paste -sd, - | sed 's/,/, /g') || cves=
notes=https://github.com/NLnetLabs/unbound/releases/tag/release-$latest
add_to_changelog "- **Windows, macOS and Android:** Unbound $latest from NLnet Labs${cves:+, with the fixes for $cves} ([their notes]($notes))."

mkdir -p dist
{
	echo "NLnet Labs released Unbound $latest; OffLok's Windows installer, macOS package and Android app ship Unbound $pinned."
	echo
	echo "- **Signatures:** good, by NLnet Labs' release key (\`$fpr\`), on the Windows build and on the source"
	echo "- **SHA-256:** \`$sum\` (Windows), \`$src_sum\` (source), as NLnet Labs published them"
	echo "- **Security advisories:** ${cves:-none names Unbound $latest}"
	echo "- **NLnet Labs' notes:** $notes"
	echo
	release_text
} >dist/unbound-update.md
echo "$latest"

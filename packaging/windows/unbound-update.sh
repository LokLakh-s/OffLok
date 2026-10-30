#!/bin/bash
# Looks for an Unbound for Windows newer than the one pinned in unbound.env. When NLnet Labs released one, checks
# its OpenPGP signature by NLnet Labs' release key (nlnetlabs-releases-g2.asc, fingerprint below), pins it in
# unbound.env and adds it to CHANGELOG.md: in a new patch release when VERSION is released already, in VERSION's
# own section otherwise. Prints the new Unbound's version, or nothing when the pinned one is the latest, and writes
# the pull request's text to dist/unbound-update.md. Run from the repository's root; .github/workflows/unbound.yml
# does the rest.
#   --from <version>: a test, as if that Unbound were pinned.
set -euo pipefail

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
for f in "$zip" "$zip.asc" "$zip.sha256"; do curl -fsSL -o "$tmp/$f" "$base/$f"; done
[ "$(gpg --show-keys --with-colons "$key" 2>/dev/null | awk -F: '/^fpr/ && !n++ { print $10 }')" = "$fpr" ] ||
	{ echo "$key is not NLnet Labs' release key" >&2; exit 1; }
gpg --dearmor <"$key" >"$tmp/key.gpg"
status=$(gpgv --status-fd 1 --keyring "$tmp/key.gpg" "$tmp/$zip.asc" "$tmp/$zip" 2>/dev/null) &&
	grep -q "^\[GNUPG:\] VALIDSIG .*$fpr" <<<"$status" ||
	{ echo "$zip: no good signature by NLnet Labs' release key" >&2; exit 1; }
sum=$(sha256sum "$tmp/$zip" | cut -d' ' -f1)
[ "$sum" = "$(cut -d' ' -f1 <"$tmp/$zip.sha256")" ] ||
	{ echo "$zip: not the SHA-256 NLnet Labs published" >&2; exit 1; }
files=$(unzip -Z1 "$tmp/$zip")
for f in unbound.exe LICENSE; do # what build.sh takes from it
	grep -qx "$f" <<<"$files" || { echo "$zip has no $f" >&2; exit 1; }
done

sed -i -e "s/^UNBOUND_VERSION=.*/UNBOUND_VERSION=$latest/" -e "s/^UNBOUND_SHA256=.*/UNBOUND_SHA256=$sum/" \
	packaging/windows/unbound.env

# NLnet Labs' advisories say "Unbound <version> includes a fix" under each CVE the release fixes.
cves=$(curl -fsSL https://nlnetlabs.nl/projects/unbound/security-advisories/ | sed -E 's/<[^>]+>//g' |
	awk -v fix="Unbound $latest includes a fix" '
		match($0, /CVE-[0-9]+-[0-9]+/) { cve = substr($0, RSTART, RLENGTH) }
		index($0, fix) && cve != "" { print cve; cve = "" }' | sort -u | paste -sd, - | sed 's/,/, /g') || cves=
notes=https://github.com/NLnetLabs/unbound/releases/tag/release-$latest
line="- **Windows:** Unbound $latest from NLnet Labs${cves:+, with the fixes for $cves} ([their notes]($notes))."
line=$(fold -s -w 116 <<<"$line" | sed -e 's/ *$//' -e '2,$s/^/  /') # wrapped as the changelog's other lines

version=$(cat VERSION)
if git ls-remote --exit-code --tags origin "refs/tags/v$version" >/dev/null; then
	IFS=. read -r major minor patch <<<"$version"
	version=$major.$minor.$((patch + 1))
	echo "$version" >VERSION
	released=yes
else
	released=
fi
# A new section before the first one, or the line at the end of VERSION's own section.
LINE=$line awk -v version="$version" -v new="$released" '
	BEGIN { line = ENVIRON["LINE"] }
	/^## / && !seen++ {
		if (new || $0 != "## " version) { print "## " version "\n\n" line "\n"; done = 1 }
		print; mine = !done; next
	}
	/^## / && mine && !done { print line; done = 1 }
	mine && !done && /^$/ { blank++; next }
	{ while (blank) { print ""; blank-- } print }
	END { if (!done) print (seen ? "" : "## " version "\n\n") line }' CHANGELOG.md >"$tmp/changelog"
cp "$tmp/changelog" CHANGELOG.md

mkdir -p dist
{
	echo "NLnet Labs released Unbound $latest; OffLok's Windows installer ships Unbound $pinned."
	echo
	echo "- **Signature:** good, by NLnet Labs' release key (\`$fpr\`)"
	echo "- **SHA-256:** \`$sum\`, as NLnet Labs published it"
	echo "- **Security advisories:** ${cves:-none names Unbound $latest}"
	echo "- **NLnet Labs' notes:** $notes"
	echo
	if [ -n "$released" ]; then
		echo "This pins it in OffLok $version. **Merging releases OffLok $version**: once the CI passes on main, the tag"
		echo "\`v$version\` is made and the release workflow runs."
	else
		echo "This pins it in OffLok $version, which is not released yet: it ships with that release, and merging"
		echo "releases nothing by itself."
	fi
} >dist/unbound-update.md
echo "$latest"

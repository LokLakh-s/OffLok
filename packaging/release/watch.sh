# What the dependency watches share (sourced by packaging/windows/unbound-update.sh and
# packaging/macos/openssl-update.sh, from the repository's root).

# check_signature <key.asc> <fingerprint> <file> <file.asc>: a good OpenPGP signature of file by that key.
check_signature() {
	local key=$1 fpr=$2 file=$3 sig=$4 ring status
	[ "$(gpg --show-keys --with-colons "$key" 2>/dev/null | awk -F: '/^fpr/ && !n++ { print $10 }')" = "$fpr" ] ||
		{ echo "$key is not the key $fpr" >&2; return 1; }
	ring=$(mktemp)
	gpg --dearmor <"$key" >"$ring"
	status=$(gpgv --status-fd 1 --keyring "$ring" "$sig" "$file" 2>/dev/null) || true
	rm -f "$ring"
	grep -q "^\[GNUPG:\] VALIDSIG .*$fpr" <<<"$status" ||
		{ echo "$(basename "$file"): no good signature by $fpr" >&2; return 1; }
}

# add_to_changelog <markdown list item>: adds it to CHANGELOG.md, in a new patch release when VERSION is released
# already (VERSION is raised), at the end of VERSION's own section otherwise. Sets version and released (yes or
# empty).
add_to_changelog() {
	local line tmp major minor patch
	line=$(fold -s -w 116 <<<"$1" | sed -e 's/ *$//' -e '2,$s/^/  /') # wrapped as the changelog's other lines
	version=$(cat VERSION)
	if git ls-remote --exit-code --tags origin "refs/tags/v$version" >/dev/null; then
		IFS=. read -r major minor patch <<<"$version"
		version=$major.$minor.$((patch + 1))
		echo "$version" >VERSION
		released=yes
	else
		released=
	fi
	tmp=$(mktemp)
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
		END { if (!done) print (seen ? "" : "## " version "\n\n") line }' CHANGELOG.md >"$tmp"
	cp "$tmp" CHANGELOG.md
	rm -f "$tmp"
}

# release_text: the end of a pull request's text, after add_to_changelog.
release_text() {
	if [ -n "$released" ]; then
		echo "This pins it in OffLok $version. **Merging releases OffLok $version**: once the CI passes on main, the tag"
		echo "\`v$version\` is made and the release workflow runs."
	else
		echo "This pins it in OffLok $version, which is not released yet: it ships with that release, and merging"
		echo "releases nothing by itself."
	fi
}

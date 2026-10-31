#!/bin/bash
# Prints a release's notes for GitHub: its section of CHANGELOG.md, then how to download and check it, and the code
# signing policy. A list item's wrapped lines are joined: GitHub may show every line break.
# Usage: packaging/release/notes.sh <version>
set -euo pipefail
cd "$(dirname "$0")/../.."
v=$1
section=$(awk -v v="$v" '/^## / { on = ($2 == v); next } on' CHANGELOG.md | awk '
	/^  [^ ]/ && prev != "" { prev = prev " " substr($0, 3); next }
	{ if (NR > 1) print prev; prev = $0 }
	END { print prev }')
[ -n "$section" ] || { echo "CHANGELOG.md has no section for $v" >&2; exit 1; }
key=$(basename packaging/release/offlok-release-*.pub.pem)
cat <<NOTES
$section

### Download

- **Windows 10 (version 2004 or later) and 11, 64-bit:** \`offlok-$v-setup.exe\`
- **macOS 13 or later, Apple silicon and Intel:** \`offlok-$v.pkg\` (not signed yet: open it once from System Settings › Privacy & Security, "Open Anyway")
- **Debian, Ubuntu:** \`offlok_${v}_amd64.deb\` or \`_arm64.deb\`, and \`offlok-desktop_${v}_amd64.deb\` for the tray icon and window
- **Fedora, openSUSE:** \`offlok-$v-1.x86_64.rpm\` or \`.aarch64.rpm\`, and \`offlok-desktop-$v-1.x86_64.rpm\`

### Check a download

\`SHA256SUMS\` gives every file's SHA-256 and is signed with OffLok's release key (Ed25519, \`$key\`). With OpenSSL 3:

\`\`\`
openssl pkeyutl -verify -pubin -inkey $key -rawin -in SHA256SUMS -sigfile SHA256SUMS.sig
sha256sum --ignore-missing -c SHA256SUMS
\`\`\`

OffLok checks the same signature and hashes before it installs an update.

### Code signing policy

Windows releases are to be signed through the SignPath Foundation: free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org). See the [code signing policy](https://github.com/LokLakh-s/OffLok#code-signing-policy) and [what OffLok sends over the network](https://github.com/LokLakh-s/OffLok#privacy).
NOTES

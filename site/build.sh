#!/bin/sh
# The website, offlok.org, ready to serve in bin/site/: every page with {{VERSION}} replaced by the release its
# download links point at (the argument, or VERSION by default; a deployment passes the latest published release).
set -eu
cd "$(dirname "$0")/.."
version=${1:-$(cat VERSION)}
version=${version#v}
case $version in
	*[!0-9.]* | "") echo "build.sh: not a version: $version" >&2; exit 1 ;;
esac
out=bin/site
rm -rf "$out"
mkdir -p "$out"
cp -R site/assets site/img site/favicon.ico "$out/"
(cd site && find . -name '*.html') | while read -r page; do
	mkdir -p "$(dirname "$out/$page")"
	sed "s/{{VERSION}}/$version/g" "site/$page" > "$out/$page"
done
if grep -rq "{{" "$out" --include=*.html; then
	echo "build.sh: a placeholder is left in $out" >&2
	exit 1
fi
echo "$out (OffLok $version)"

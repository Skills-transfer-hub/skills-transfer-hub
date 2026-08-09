#!/bin/sh
set -eu

version=${1:-}
destination=${2:-}

case "$version" in
	v[0-9]*.[0-9]*.[0-9]*) version=${version#v} ;;
	[0-9]*.[0-9]*.[0-9]*) ;;
	*) printf 'version STH invalide: %s\n' "$version" >&2; exit 2 ;;
esac

if [ -z "$destination" ]; then
	printf 'usage: write-sth-code-engine-manifest.sh <version> <destination>\n' >&2
	exit 2
fi

temporary="${destination}.tmp.$$"
trap 'rm -f "$temporary"' EXIT HUP INT TERM
mkdir -p "$(dirname "$destination")"
printf '%s\n' \
	'{' \
	'  "schema_version": 1,' \
	"  \"engine_version\": \"$version\"," \
	'  "bridge_protocol": 1,' \
	'  "contract": "sth-code-bridge-v1",' \
	'  "artifacts": {' \
	'    "darwin-amd64": { "name": "sth-engine-darwin-amd64" },' \
	'    "darwin-arm64": { "name": "sth-engine-darwin-arm64" },' \
	'    "linux-amd64": { "name": "sth-engine-linux-amd64" },' \
	'    "windows-amd64": { "name": "sth-engine-windows-amd64.exe" }' \
	'  }' \
	'}' > "$temporary"
mv "$temporary" "$destination"
trap - EXIT HUP INT TERM

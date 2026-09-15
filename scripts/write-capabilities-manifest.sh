#!/bin/sh

set -eu

release_version=${1:-}
output_path=${2:-}

if [ -z "$release_version" ] || [ -z "$output_path" ]; then
	printf 'usage: write-capabilities-manifest.sh <version> <output>\n' >&2
	exit 1
fi

output_directory=$(dirname "$output_path")
temporary_path="$output_path.tmp"
mkdir -p "$output_directory"
go run -ldflags="-X sth/cmd.version=$release_version" . capabilities --json > "$temporary_path"
jq -e --arg version "$release_version" '.schema_version == 1 and .cli_version == $version and (.targets | length) == 6 and (.artifact_kinds | length) == 6 and (.compatibility | length) == 36' "$temporary_path" >/dev/null
mv "$temporary_path" "$output_path"

if command -v sha256sum >/dev/null 2>&1; then
	checksum=$(sha256sum "$output_path" | awk '{ print $1 }')
else
	checksum=$(shasum -a 256 "$output_path" | awk '{ print $1 }')
fi
printf '%s  %s\n' "$checksum" "$(basename "$output_path")" > "$output_path.sha256"

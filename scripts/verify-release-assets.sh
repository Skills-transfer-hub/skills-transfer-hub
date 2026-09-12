#!/bin/sh
set -eu

asset_directory=${1:-}
release_version=${2:-}

if [ -z "$asset_directory" ] || [ -z "$release_version" ]; then
	printf 'usage: verify-release-assets.sh <asset-directory> <version>\n' >&2
	exit 2
fi

if [ ! -d "$asset_directory" ]; then
	printf 'release asset directory not found: %s\n' "$asset_directory" >&2
	exit 1
fi

asset_directory=$(CDPATH= cd -- "$asset_directory" && pwd -P)
normalized_version=${release_version#v}
script_directory=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repository_directory=$(dirname "$script_directory")

if ! command -v jq >/dev/null 2>&1; then
	printf 'jq is required\n' >&2
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	checksum_command=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	checksum_command=shasum
else
	printf 'sha256sum or shasum is required\n' >&2
	exit 1
fi

expected_assets='sth-linux-amd64.tar.gz
sth-darwin-amd64.tar.gz
sth-darwin-arm64.tar.gz
sth-windows-amd64.zip
sth-engine-linux-amd64
sth-engine-darwin-amd64
sth-engine-darwin-arm64
sth-engine-windows-amd64.exe
sth-code-engine.json
sth-capabilities-v1.json
sth-capabilities-v1.json.sha256
sth-capabilities-v1.md'

resolve_asset_path() {
	asset_name=$1
	if resolved_asset_path=$(sh "$script_directory/resolve-release-asset.sh" "$asset_directory" "$asset_name" 2>/dev/null); then
		printf '%s\n' "$resolved_asset_path"
		return 0
	fi
	if [ "$asset_name" = sth-code-engine.json ] && [ -f "$asset_directory/artifacts.json" ] && [ -f "$repository_directory/.release/sth-code-engine.json" ] && [ ! -L "$repository_directory/.release/sth-code-engine.json" ]; then
		printf '%s\n' "$repository_directory/.release/sth-code-engine.json"
		return 0
	fi
	return 1
}

for asset_name in $expected_assets; do
	if ! resolve_asset_path "$asset_name" >/dev/null; then
		printf 'missing release asset: %s\n' "$asset_name" >&2
		exit 1
	fi
done

for control_asset in SHA256SUMS SHA256SUMS.sig; do
	if [ ! -s "$asset_directory/$control_asset" ]; then
		printf 'missing or empty release control asset: %s\n' "$control_asset" >&2
		exit 1
	fi
done

for asset_name in $expected_assets; do
	if ! awk -v expected="$asset_name" '$2 == expected { count++ } END { exit count == 1 ? 0 : 1 }' "$asset_directory/SHA256SUMS"; then
		printf 'checksum entry missing or duplicated: %s\n' "$asset_name" >&2
		exit 1
	fi
done

checksum_entries=$(awk 'NF >= 2 { count++ } END { print count + 0 }' "$asset_directory/SHA256SUMS")
if [ "$checksum_entries" -ne 12 ]; then
	printf 'unexpected checksum entry count: %s\n' "$checksum_entries" >&2
	exit 1
fi

for asset_name in $expected_assets; do
	asset_path=$(resolve_asset_path "$asset_name")
	expected_checksum=$(awk -v expected="$asset_name" '$2 == expected { print $1 }' "$asset_directory/SHA256SUMS")
	if [ "$checksum_command" = sha256sum ]; then
		actual_checksum=$(sha256sum "$asset_path" | awk '{ print $1 }')
	else
		actual_checksum=$(shasum -a 256 "$asset_path" | awk '{ print $1 }')
	fi
	if [ "$actual_checksum" != "$expected_checksum" ]; then
		printf 'checksum mismatch: %s\n' "$asset_name" >&2
		exit 1
	fi
	printf '%s: OK\n' "$asset_name"
done

manifest_path=$(resolve_asset_path sth-code-engine.json)
jq -e --arg version "$normalized_version" '
  .schema_version == 1 and
  .engine_version == $version and
  .bridge_protocol == 1 and
  .contract == "sth-code-bridge-v1" and
  .artifacts["linux-amd64"].name == "sth-engine-linux-amd64" and
  .artifacts["darwin-amd64"].name == "sth-engine-darwin-amd64" and
  .artifacts["darwin-arm64"].name == "sth-engine-darwin-arm64" and
  .artifacts["windows-amd64"].name == "sth-engine-windows-amd64.exe"
' "$manifest_path" >/dev/null

capabilities_path=$(resolve_asset_path sth-capabilities-v1.json)
jq -e --arg version "$normalized_version" '
  .schema_version == 1 and
  (.cli_version == $version or .cli_version == ("v" + $version)) and
  (.targets | length) == 6 and
  (.artifact_kinds | length) == 6 and
  (.compatibility | length) == 36
' "$capabilities_path" >/dev/null
capabilities_checksum_path=$(resolve_asset_path sth-capabilities-v1.json.sha256)
expected_capabilities_checksum=$(awk -v expected="sth-capabilities-v1.json" '$2 == expected { print $1 }' "$capabilities_checksum_path")
if [ "$checksum_command" = sha256sum ]; then
	actual_capabilities_checksum=$(sha256sum "$capabilities_path" | awk '{ print $1 }')
else
	actual_capabilities_checksum=$(shasum -a 256 "$capabilities_path" | awk '{ print $1 }')
fi
if [ "$expected_capabilities_checksum" != "$actual_capabilities_checksum" ]; then
	printf 'capabilities sidecar checksum mismatch\n' >&2
	exit 1
fi

capabilities_documentation_path=$(resolve_asset_path sth-capabilities-v1.md)
if { ! grep -Fq "Generated from schema version 1 for CLI $normalized_version." "$capabilities_documentation_path" && ! grep -Fq "Generated from schema version 1 for CLI v$normalized_version." "$capabilities_documentation_path"; } || ! grep -Fq '| cursor ' "$capabilities_documentation_path"; then
	printf 'capabilities documentation does not match manifest\n' >&2
	exit 1
fi

(cd "$repository_directory" && go run ./tools/release-sign --verify --in "$asset_directory/SHA256SUMS" --signature "$asset_directory/SHA256SUMS.sig")

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-release-assets.XXXXXX")
cleanup() {
	rm -rf "$temporary_directory"
}
trap cleanup EXIT HUP INT TERM

verify_tar_pair() {
	archive_name=$1
	binary_name=$2
	extracted_binary=$3
	archive_path=$(resolve_asset_path "$archive_name")
	binary_path=$(resolve_asset_path "$binary_name")
	tar -xOf "$archive_path" sth > "$temporary_directory/$extracted_binary"
	if ! cmp -s "$temporary_directory/$extracted_binary" "$binary_path"; then
		printf 'engine binary differs from archive binary: %s\n' "$binary_name" >&2
		exit 1
	fi
}

verify_tar_pair sth-linux-amd64.tar.gz sth-engine-linux-amd64 linux-amd64
verify_tar_pair sth-darwin-amd64.tar.gz sth-engine-darwin-amd64 darwin-amd64
verify_tar_pair sth-darwin-arm64.tar.gz sth-engine-darwin-arm64 darwin-arm64
windows_archive_path=$(resolve_asset_path sth-windows-amd64.zip)
windows_binary_path=$(resolve_asset_path sth-engine-windows-amd64.exe)
unzip -p "$windows_archive_path" sth.exe > "$temporary_directory/windows-amd64.exe"
if ! cmp -s "$temporary_directory/windows-amd64.exe" "$windows_binary_path"; then
	printf 'engine binary differs from archive binary: sth-engine-windows-amd64.exe\n' >&2
	exit 1
fi

printf 'release assets verified: %s\n' "$normalized_version"

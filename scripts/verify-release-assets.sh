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

asset_directory=$(CDPATH= cd -- "$asset_directory" && pwd)
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
sth-code-engine.json'

resolve_asset_path() {
	asset_name=$1
	if [ -f "$asset_directory/$asset_name" ]; then
		printf '%s\n' "$asset_directory/$asset_name"
		return 0
	fi
	if [ "$asset_name" = sth-code-engine.json ] && [ -f "$asset_directory/artifacts.json" ] && [ -f "$repository_directory/.release/sth-code-engine.json" ]; then
		printf '%s\n' "$repository_directory/.release/sth-code-engine.json"
		return 0
	fi
	if [ ! -f "$asset_directory/artifacts.json" ]; then
		return 1
	fi
	artifact_path=$(jq -r --arg name "$asset_name" '[.[] | select(.name == $name) | .path] | if length == 1 then .[0] else empty end' "$asset_directory/artifacts.json")
	case "$artifact_path" in
		"$asset_directory"/*)
			if [ -f "$artifact_path" ]; then
				printf '%s\n' "$artifact_path"
				return 0
			fi
			;;
	esac
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
if [ "$checksum_entries" -ne 9 ]; then
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

#!/bin/sh
set -eu

asset_directory=${1:-}
asset_name=${2:-}

if [ -z "$asset_directory" ] || [ -z "$asset_name" ]; then
	printf 'usage: resolve-release-asset.sh <asset-directory> <asset-name>\n' >&2
	exit 2
fi
case "$asset_name" in
	.|..|*/*)
		printf 'invalid release asset name: %s\n' "$asset_name" >&2
		exit 2
		;;
esac
if [ ! -d "$asset_directory" ]; then
	printf 'release asset directory not found: %s\n' "$asset_directory" >&2
	exit 1
fi

asset_directory=$(CDPATH= cd -- "$asset_directory" && pwd -P)
asset_parent_directory=$(dirname "$asset_directory")
direct_path="$asset_directory/$asset_name"
if [ -f "$direct_path" ] && [ ! -L "$direct_path" ]; then
	printf '%s\n' "$direct_path"
	exit 0
fi
if [ ! -f "$asset_directory/artifacts.json" ]; then
	printf 'release asset not found: %s\n' "$asset_name" >&2
	exit 1
fi
case "$asset_name" in
 sth-code-engine.json|sth-capabilities-v1.json|sth-capabilities-v1.json.sha256|sth-capabilities-v1.md)
  metadata_directory="$asset_parent_directory/.release"
  metadata_path="$metadata_directory/$asset_name"
  if [ -d "$metadata_directory" ] && [ ! -L "$metadata_directory" ] && [ -f "$metadata_path" ] && [ ! -L "$metadata_path" ]; then
   printf '%s\n' "$metadata_path"
   exit 0
  fi
  ;;
esac

if ! command -v jq >/dev/null 2>&1; then
	printf 'jq is required\n' >&2
	exit 1
fi
if ! artifact_path=$(jq -er --arg name "$asset_name" '[.[] | select(.name == $name) | .path] | if length == 1 and (.[0] | type == "string") and (.[0] | length > 0) then .[0] else empty end' "$asset_directory/artifacts.json"); then
	printf 'release asset mapping not found or ambiguous: %s\n' "$asset_name" >&2
	exit 1
fi

case "$artifact_path" in
	/*) artifact_candidate=$artifact_path ;;
	*) artifact_candidate="$asset_parent_directory/$artifact_path" ;;
esac
if [ ! -f "$artifact_candidate" ] && [ "${artifact_path#/}" = "$artifact_path" ]; then
	artifact_candidate="$asset_directory/$artifact_path"
fi
if [ ! -f "$artifact_candidate" ] || [ -L "$artifact_candidate" ]; then
	printf 'release asset path is missing or unsafe: %s\n' "$asset_name" >&2
	exit 1
fi

artifact_directory=$(CDPATH= cd -- "$(dirname "$artifact_candidate")" && pwd -P)
artifact_candidate="$artifact_directory/$(basename "$artifact_candidate")"
case "$artifact_candidate" in
	"$asset_directory"/*)
		printf '%s\n' "$artifact_candidate"
		;;
	*)
		printf 'release asset path escapes the asset directory: %s\n' "$asset_name" >&2
		exit 1
		;;
esac

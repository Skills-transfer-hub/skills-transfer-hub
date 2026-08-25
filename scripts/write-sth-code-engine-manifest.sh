#!/bin/sh
set -eu
LC_ALL=C
export LC_ALL

version=${1:-}
destination=${2:-}

invalid_version() {
	printf 'invalid STH version: %s\n' "$version" >&2
	exit 2
}

validate_identifier_list() (
	identifier_list=$1
	reject_numeric_leading_zero=$2
	case "$identifier_list" in
		''|.*|*.|*..*|*[!0-9A-Za-z.-]*) exit 1 ;;
	esac
	previous_ifs=$IFS
	IFS=.
	set -- $identifier_list
	IFS=$previous_ifs
	for identifier in "$@"; do
		case "$identifier" in
			''|*[!0-9A-Za-z-]*) exit 1 ;;
		esac
		if [ "$reject_numeric_leading_zero" -eq 1 ]; then
			case "$identifier" in
				*[!0-9]*) ;;
				0) ;;
				0*) exit 1 ;;
			esac
		fi
	done
)

normalized_version=$version
case "$normalized_version" in
	v*) normalized_version=${normalized_version#v} ;;
esac
case "$normalized_version" in
	''|*[!0-9A-Za-z.+-]*) invalid_version ;;
esac

has_build_metadata=0
build_metadata=
case "$normalized_version" in
	*+*)
		has_build_metadata=1
		core_and_prerelease=${normalized_version%%+*}
		build_metadata=${normalized_version#*+}
		case "$build_metadata" in
			''|*+*) invalid_version ;;
		esac
		;;
	*) core_and_prerelease=$normalized_version ;;
esac

has_prerelease=0
prerelease=
case "$core_and_prerelease" in
	*-*)
		has_prerelease=1
		core_version=${core_and_prerelease%%-*}
		prerelease=${core_and_prerelease#*-}
		;;
	*) core_version=$core_and_prerelease ;;
esac

if ! printf '%s\n' "$core_version" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
	invalid_version
fi
if [ "$has_prerelease" -eq 1 ] && ! validate_identifier_list "$prerelease" 1; then
	invalid_version
fi
if [ "$has_build_metadata" -eq 1 ] && ! validate_identifier_list "$build_metadata" 0; then
	invalid_version
fi

if [ -z "$destination" ]; then
	printf 'usage: write-sth-code-engine-manifest.sh <version> <destination>\n' >&2
	exit 2
fi

temporary="${destination}.tmp.$$"
trap 'rm -f "$temporary"' EXIT HUP INT TERM
mkdir -p "$(dirname "$destination")"
{
	printf '%s\n' \
		'{' \
		'  "schema_version": 1,'
	printf '  "engine_version": "%s",\n' "$normalized_version"
	printf '%s\n' \
		'  "bridge_protocol": 1,' \
		'  "contract": "sth-code-bridge-v1",' \
		'  "artifacts": {' \
		'    "darwin-amd64": { "name": "sth-engine-darwin-amd64" },' \
		'    "darwin-arm64": { "name": "sth-engine-darwin-arm64" },' \
		'    "linux-amd64": { "name": "sth-engine-linux-amd64" },' \
		'    "windows-amd64": { "name": "sth-engine-windows-amd64.exe" }' \
		'  }' \
		'}'
} > "$temporary"
mv "$temporary" "$destination"
trap - EXIT HUP INT TERM

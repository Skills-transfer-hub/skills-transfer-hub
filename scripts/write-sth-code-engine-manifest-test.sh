#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
manifest_writer="$repository_root/scripts/write-sth-code-engine-manifest.sh"
test_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-engine-manifest-test.XXXXXX")
case_number=0

cleanup_test_directory() {
	case "$test_directory" in
		/tmp/sth-engine-manifest-test.*|/private/tmp/sth-engine-manifest-test.*|/var/folders/*/sth-engine-manifest-test.*)
			if [ -d "$test_directory" ]; then
				rm -r -- "$test_directory"
			fi
			;;
	esac
}

trap cleanup_test_directory EXIT HUP INT TERM

fail_test() {
	printf 'KO: %s\n' "$1" >&2
	exit 1
}

expect_valid_version() {
	input_version=$1
	expected_version=$2
	case_number=$((case_number + 1))
	destination="$test_directory/valid-$case_number/manifest.json"
	sh "$manifest_writer" "$input_version" "$destination"
	if ! grep -Fqx "  \"engine_version\": \"$expected_version\"," "$destination"; then
		fail_test "manifest did not contain normalized version $expected_version"
	fi
}

expect_invalid_version() {
	input_version=$1
	case_number=$((case_number + 1))
	destination="$test_directory/invalid-$case_number.json"
	set +e
	sh "$manifest_writer" "$input_version" "$destination" >"$test_directory/invalid-$case_number.out" 2>"$test_directory/invalid-$case_number.err"
	status=$?
	set -e
	if [ "$status" -ne 2 ]; then
		fail_test "version unexpectedly returned status $status: $input_version"
	fi
	if [ -e "$destination" ]; then
		fail_test "invalid version created a manifest: $input_version"
	fi
}

expect_valid_version 0.0.0 0.0.0
expect_valid_version 1.2.3 1.2.3
expect_valid_version v1.2.3 1.2.3
expect_valid_version 1.2.3-rc.1+build.5 1.2.3-rc.1+build.5
expect_valid_version 1.2.3-alpha-1 1.2.3-alpha-1

expect_invalid_version ''
expect_invalid_version 1.2
expect_invalid_version 1.2.3.4
expect_invalid_version 01.2.3
expect_invalid_version 1.02.3
expect_invalid_version 1.2.03
expect_invalid_version 1.2.3-01
expect_invalid_version 1.2.3-
expect_invalid_version 1.2.3+
expect_invalid_version 1.2.3-alpha..1
expect_invalid_version 1.2.3+build..1
expect_invalid_version 1.2.3+build+again
expect_invalid_version '1.2.3"'
expect_invalid_version '1.2.3\injected'
expect_invalid_version "$(printf '1.2.3\n\"injected\":true')"
expect_invalid_version 1.2.3-é

printf 'write-sth-code-engine-manifest: OK\n'

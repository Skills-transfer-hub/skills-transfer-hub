#!/bin/sh

set -u

application_path=${1:-}
expected_version=${2:-}
hermetic_mode=${3:-0}

case "$hermetic_mode" in
	0|1)
		;;
	*)
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
		;;
esac

if [ -z "$application_path" ] || [ ! -d "$application_path" ] || [ -z "$expected_version" ]; then
	printf 'ANTIGRAVITY_IDE_APP_KO\n'
	exit 1
fi

if ! python3 - "$application_path" "$expected_version" <<'PY'
import os
import plistlib
import sys

application_path = os.path.realpath(sys.argv[1])
expected_version = sys.argv[2]
info_path = os.path.join(application_path, "Contents", "Info.plist")

if not os.path.isfile(info_path):
    raise SystemExit("Info.plist Antigravity absent")
with open(info_path, "rb") as info_stream:
    info = plistlib.load(info_stream)
if info.get("CFBundleIdentifier") != "com.google.antigravity-ide":
    raise SystemExit("bundle Antigravity inattendu")
if info.get("CFBundleShortVersionString") != expected_version:
    raise SystemExit("version Antigravity inattendue")
executable_name = info.get("CFBundleExecutable")
if not isinstance(executable_name, str) or not executable_name:
    raise SystemExit("exécutable Antigravity invalide")
executable_path = os.path.join(application_path, "Contents", "MacOS", executable_name)
if not os.path.isfile(executable_path) or not os.access(executable_path, os.X_OK):
    raise SystemExit("exécutable Antigravity absent")
PY
then
	printf 'ANTIGRAVITY_IDE_APP_KO\n'
	exit 1
fi

if [ "$hermetic_mode" = "0" ]; then
	if [ "$(uname -s)" != "Darwin" ]; then
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
	fi
	if ! codesign --verify --deep --strict --verbose=0 "$application_path" >/dev/null 2>&1; then
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
	fi
	signature_details=$(codesign -dv --verbose=4 "$application_path" 2>&1) || {
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
	}
	if ! printf '%s\n' "$signature_details" | grep -Fxq 'TeamIdentifier=EQHXZ8M8AV'; then
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
	fi
	if ! spctl --assess --type execute "$application_path" >/dev/null 2>&1; then
		printf 'ANTIGRAVITY_IDE_APP_KO\n'
		exit 1
	fi
fi

printf 'ANTIGRAVITY_IDE_APP_OK\n'

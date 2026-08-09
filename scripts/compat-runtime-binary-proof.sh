#!/bin/sh

set -u

binary_path=${1:-}
expected_version=${2:-}
expected_sha256=${3:-}

if [ -z "$binary_path" ] || [ -z "$expected_version" ] || [ -z "$expected_sha256" ]; then
	printf 'RUNTIME_BINARY_IDENTITY_KO\n'
	exit 1
fi

if ! python3 - "$binary_path" "$expected_version" "$expected_sha256" <<'PY'
import hashlib
import os
import re
import subprocess
import sys

binary_path = os.path.realpath(sys.argv[1])
expected_version = sys.argv[2]
expected_sha256 = sys.argv[3].lower()

if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", expected_version):
    raise SystemExit("version attendue invalide")
if not re.fullmatch(r"[0-9a-f]{64}", expected_sha256):
    raise SystemExit("SHA-256 attendu invalide")
if not os.path.isfile(binary_path) or not os.access(binary_path, os.X_OK):
    raise SystemExit("executable runtime absent")

def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as input_stream:
        while chunk := input_stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()

binary_sha256 = sha256_file(binary_path)
if binary_sha256 != expected_sha256:
    raise SystemExit("SHA-256 runtime inattendu")

try:
    version_process = subprocess.run(
        [binary_path, "--version"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        timeout=30,
        check=False,
    )
except (OSError, subprocess.TimeoutExpired) as runtime_error:
    raise SystemExit(f"lecture de version impossible: {runtime_error}")

if version_process.returncode != 0:
    raise SystemExit("commande --version non concluante")
version_output = f"{version_process.stdout}\n{version_process.stderr}"
version_pattern = (
    rf"(?<![A-Za-z0-9._+-]){re.escape(expected_version)}"
    r"(?![A-Za-z0-9_+-])(?!\.[A-Za-z0-9._+-])"
)
if re.search(version_pattern, version_output) is None:
    raise SystemExit("version runtime inattendue")
if sha256_file(binary_path) != expected_sha256:
    raise SystemExit("executable runtime modifie pendant la preuve")
PY
then
	printf 'RUNTIME_BINARY_IDENTITY_KO\n'
	exit 1
fi

printf 'RUNTIME_BINARY_IDENTITY_OK\n'

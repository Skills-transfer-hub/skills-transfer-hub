#!/bin/sh

set -u

fixture_root=${1:-}
fixture_manifest=${2:-}
expected_suffix=${3:-}
expected_binary=${4:-}

if [ -z "$fixture_root" ] || [ ! -d "$fixture_root" ] || [ -z "$fixture_manifest" ] || [ ! -f "$fixture_manifest" ]; then
	printf 'FIXTURE_ROOT_KO\n'
	exit 1
fi

python3 - "$fixture_root" "$fixture_manifest" "$expected_suffix" "$expected_binary" <<'PY'
import hashlib
import datetime
import json
import os
import re
import sys

fixture_root = os.path.realpath(sys.argv[1])
manifest_path = sys.argv[2]
expected_suffix = sys.argv[3]
expected_binary = sys.argv[4]
provenance_path = os.path.join(fixture_root, ".sth-compat-provenance.json")

if not os.path.isfile(provenance_path):
    raise SystemExit("provenance absente")

def reject_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"clé JSON dupliquée: {key}")
        result[key] = value
    return result

with open(provenance_path, encoding="utf-8") as provenance_stream:
    provenance = json.load(provenance_stream, object_pairs_hook=reject_duplicate_keys)

if provenance.get("schema_version") != 2:
    raise SystemExit("schema de provenance invalide")
if provenance.get("generator") != "sth":
    raise SystemExit("générateur de provenance invalide")
if provenance.get("sentinel_suffix") != expected_suffix:
    raise SystemExit("suffixe de sentinelle inattendu")
if not re.fullmatch(r"[0-9a-f]{64}", provenance.get("binary_sha256", "")):
    raise SystemExit("empreinte du binaire invalide")
if not re.fullmatch(r"[A-F0-9]{32}", provenance.get("runtime_challenge", "")):
    raise SystemExit("challenge runtime invalide")
generated_at = provenance.get("generated_at")
if not isinstance(generated_at, str):
    raise SystemExit("horodatage de provenance absent")
try:
    normalized_generated_at = generated_at[:-1] + "+00:00" if generated_at.endswith("Z") else generated_at
    generated_datetime = datetime.datetime.fromisoformat(normalized_generated_at)
except ValueError as parse_error:
    raise SystemExit("horodatage de provenance invalide") from parse_error
if generated_datetime.tzinfo is None or generated_datetime.utcoffset() is None:
    raise SystemExit("fuseau de provenance absent")
current_datetime = datetime.datetime.now(datetime.timezone.utc)
generated_datetime = generated_datetime.astimezone(datetime.timezone.utc)
if generated_datetime > current_datetime + datetime.timedelta(minutes=5):
    raise SystemExit("provenance datée dans le futur")
if generated_datetime < current_datetime - datetime.timedelta(hours=6):
    raise SystemExit("provenance expirée")

def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as input_stream:
        while chunk := input_stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()

if expected_binary:
    expected_binary = os.path.realpath(expected_binary)
    if not os.path.isfile(expected_binary):
        raise SystemExit("binaire STH attendu absent")
    if provenance.get("binary_sha256") != sha256_file(expected_binary):
        raise SystemExit("empreinte du binaire STH inattendue")

with open(manifest_path, encoding="utf-8") as manifest_stream:
    expected_paths = [
        line.strip().removeprefix("testdata/runtime/")
        for line in manifest_stream
        if line.strip()
    ]

if len(expected_paths) != len(set(expected_paths)):
    raise SystemExit("manifeste de fixtures dupliqué")

actual_paths = []
for directory_path, _, file_names in os.walk(fixture_root):
    for file_name in file_names:
        absolute_path = os.path.join(directory_path, file_name)
        relative_path = os.path.relpath(absolute_path, fixture_root)
        if relative_path != ".sth-compat-provenance.json":
            actual_paths.append(relative_path)

if sorted(actual_paths) != sorted(expected_paths):
    raise SystemExit("surface exportée différente du manifeste")

files = provenance.get("files")
if not isinstance(files, list):
    raise SystemExit("liste de fichiers de provenance absente")

provenance_by_path = {}
for file_entry in files:
    if not isinstance(file_entry, dict):
        raise SystemExit("entrée de provenance invalide")
    relative_path = file_entry.get("path")
    if not isinstance(relative_path, str) or not relative_path:
        raise SystemExit("chemin de provenance invalide")
    if relative_path in provenance_by_path:
        raise SystemExit("chemin de provenance dupliqué")
    if os.path.isabs(relative_path) or ".." in relative_path.split(os.sep):
        raise SystemExit("chemin de provenance hors racine")
    provenance_by_path[relative_path] = file_entry

if sorted(provenance_by_path) != sorted(expected_paths):
    raise SystemExit("provenance différente du manifeste")

managed_count = 0
harness_count = 0
for relative_path in expected_paths:
    file_entry = provenance_by_path[relative_path]
    absolute_path = os.path.realpath(os.path.join(fixture_root, relative_path))
    if os.path.commonpath([fixture_root, absolute_path]) != fixture_root:
        raise SystemExit("fixture hors racine")
    expected_kind = (
        "harness"
        if relative_path.endswith("sth-runtime-instruction-target.txt")
        else "managed"
    )
    if file_entry.get("kind") != expected_kind:
        raise SystemExit("type de fixture invalide")
    if file_entry.get("sha256") != sha256_file(absolute_path):
        raise SystemExit("empreinte de fixture invalide")
    managed_count += expected_kind == "managed"
    harness_count += expected_kind == "harness"

if provenance.get("managed_file_count") != managed_count or managed_count != 23:
    raise SystemExit("nombre de fichiers gérés invalide")
if provenance.get("harness_file_count") != harness_count or harness_count != 1:
    raise SystemExit("nombre de fichiers harness invalide")

print("FIXTURE_ROOT_OK")
PY

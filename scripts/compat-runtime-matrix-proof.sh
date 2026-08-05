#!/bin/sh

set -u

expected_contracts=${1:-}
actual_results=${2:-}

if [ -z "$expected_contracts" ] || [ ! -f "$expected_contracts" ] || [ -z "$actual_results" ] || [ ! -f "$actual_results" ]; then
	printf 'RUNTIME_MATRIX_KO\n'
	exit 1
fi

python3 - "$expected_contracts" "$actual_results" <<'PY'
import sys

expected_path, actual_path = sys.argv[1:]

expected = {}
with open(expected_path, encoding="utf-8") as expected_stream:
    for line_number, line in enumerate(expected_stream, 1):
        fields = line.rstrip("\n").split("\t")
        if len(fields) != 3:
            raise SystemExit(f"manifeste strict invalide ligne {line_number}")
        target, artifact, allowed_statuses = fields
        key = (target, artifact)
        if key in expected:
            raise SystemExit(f"contrat strict dupliqué: {target}/{artifact}")
        statuses = set(allowed_statuses.split(","))
        if not statuses or not statuses <= {"OK", "INFO"}:
            raise SystemExit(f"statuts stricts invalides: {target}/{artifact}")
        expected[key] = statuses

actual = {}
with open(actual_path, encoding="utf-8") as actual_stream:
    for line_number, line in enumerate(actual_stream, 1):
        fields = line.rstrip("\n").split("\t")
        if len(fields) != 3:
            raise SystemExit(f"résultat runtime invalide ligne {line_number}")
        status, target, artifact = fields
        key = (target, artifact)
        if key in actual:
            raise SystemExit(f"preuve runtime dupliquée: {target}/{artifact}")
        actual[key] = status

missing = sorted(set(expected) - set(actual))
unexpected = sorted(set(actual) - set(expected))
invalid = sorted(
    (target, artifact, actual[(target, artifact)])
    for target, artifact in set(expected) & set(actual)
    if actual[(target, artifact)] not in expected[(target, artifact)]
)

if missing:
    raise SystemExit(f"preuves runtime absentes: {missing!r}")
if unexpected:
    raise SystemExit(f"preuves runtime inattendues: {unexpected!r}")
if invalid:
    raise SystemExit(f"statuts runtime non certifiants: {invalid!r}")

print("RUNTIME_MATRIX_OK")
PY

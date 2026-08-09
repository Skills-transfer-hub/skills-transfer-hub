#!/bin/sh

set -u

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_root=${COMPAT_FIXTURE_ROOT:-}
expected_binary=${COMPAT_STH_BIN:-}
application_path=${ANTIGRAVITY_IDE_APP:-}
expected_version=${ANTIGRAVITY_IDE_VERSION:-}
antigravity_data_root=${ANTIGRAVITY_IDE_DATA_ROOT:-}
hermetic_mode=${COMPAT_HERMETIC:-0}
prepare_only=0

if [ "${1:-}" = "--prepare-only" ]; then
	prepare_only=1
	shift
fi
if [ "$#" -ne 0 ]; then
	printf 'Usage: COMPAT_FIXTURE_ROOT=/abs/fixtures COMPAT_STH_BIN=/abs/sth ANTIGRAVITY_IDE_APP=/abs/Antigravity.app ANTIGRAVITY_IDE_VERSION=x.y.z %s [--prepare-only]\n' "$0" >&2
	exit 2
fi

case "$hermetic_mode" in
	0|1)
		;;
	*)
		printf 'COMPAT_HERMETIC doit valoir 0 ou 1\n' >&2
		exit 2
		;;
esac
if [ "$hermetic_mode" = "1" ]; then
	compat_mode=hermetic
else
	compat_mode=real
fi

if [ -z "$fixture_root" ] || [ ! -d "$fixture_root" ]; then
	printf 'COMPAT_FIXTURE_ROOT généré par STH est obligatoire\n' >&2
	exit 2
fi
if [ -z "$expected_binary" ] || [ ! -x "$expected_binary" ]; then
	printf 'COMPAT_STH_BIN exécutable est obligatoire\n' >&2
	exit 2
fi
if [ -z "$application_path" ] || [ ! -d "$application_path" ]; then
	printf 'ANTIGRAVITY_IDE_APP est obligatoire\n' >&2
	exit 2
fi
if [ -z "$expected_version" ] || ! printf '%s\n' "$expected_version" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._+-]*$'; then
	printf 'ANTIGRAVITY_IDE_VERSION est obligatoire et doit être canonique\n' >&2
	exit 2
fi

fixture_root=$(CDPATH= cd -- "$fixture_root" && pwd) || {
	printf 'Résolution de COMPAT_FIXTURE_ROOT impossible : %s\n' "$fixture_root" >&2
	exit 2
}
expected_binary_directory=$(CDPATH= cd -- "$(dirname -- "$expected_binary")" && pwd) || {
	printf 'Résolution de COMPAT_STH_BIN impossible : %s\n' "$expected_binary" >&2
	exit 2
}
expected_binary="$expected_binary_directory/$(basename -- "$expected_binary")"
application_path=$(CDPATH= cd -- "$application_path" && pwd) || {
	printf 'Résolution de ANTIGRAVITY_IDE_APP impossible : %s\n' "$application_path" >&2
	exit 2
}

fixture_manifest="$repository_root/testdata/runtime/fixture-manifest.txt"
provenance_file="$fixture_root/.sth-compat-provenance.json"
project_directory="$fixture_root/antigravity-project"
workflow_file="$project_directory/.agents/workflows/sth-runtime-command.md"
rule_file="$project_directory/.agents/rules/sth-runtime-rule.md"
application_cli="$application_path/Contents/Resources/app/bin/antigravity-ide"

if [ ! -f "$provenance_file" ] || [ ! -d "$project_directory" ] || [ ! -f "$workflow_file" ] || [ ! -f "$rule_file" ]; then
	printf 'Fixture Antigravity IDE générée incomplète : %s\n' "$fixture_root" >&2
	exit 2
fi
fixture_proof_output=$(sh "$repository_root/scripts/compat-fixture-root-proof.sh" \
	"$fixture_root" "$fixture_manifest" UPDATED "$expected_binary" 2>&1) || {
	printf 'Fixture Antigravity IDE générée invalide : %s\n' "$fixture_proof_output" >&2
	exit 2
}
if [ "$fixture_proof_output" != "FIXTURE_ROOT_OK" ]; then
	printf 'Preuve de fixture Antigravity IDE inattendue : %s\n' "$fixture_proof_output" >&2
	exit 2
fi

application_proof_output=$(sh "$repository_root/scripts/compat-antigravity-ide-app-proof.sh" \
	"$application_path" "$expected_version" "$hermetic_mode" 2>&1) || {
	printf 'Application Antigravity IDE invalide : %s\n' "$application_proof_output" >&2
	exit 2
}
if [ "$application_proof_output" != "ANTIGRAVITY_IDE_APP_OK" ]; then
	printf 'Preuve application Antigravity IDE inattendue : %s\n' "$application_proof_output" >&2
	exit 2
fi
if [ ! -x "$application_cli" ]; then
	printf 'CLI application Antigravity IDE introuvable : %s\n' "$application_cli" >&2
	exit 2
fi

runtime_challenge=$(python3 - "$provenance_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as provenance_stream:
    provenance = json.load(provenance_stream)
print(provenance["runtime_challenge"])
PY
) || {
	printf 'Lecture du challenge Antigravity IDE impossible\n' >&2
	exit 2
}

if [ -z "$antigravity_data_root" ]; then
	antigravity_data_root=$(python3 - <<'PY'
import pathlib

print(pathlib.Path.home() / ".gemini" / "antigravity-ide")
PY
	) || {
		printf 'Résolution de la racine de données Antigravity IDE impossible\n' >&2
		exit 2
	}
fi
antigravity_data_root=$(python3 - "$antigravity_data_root" <<'PY'
import pathlib
import sys

print(pathlib.Path(sys.argv[1]).expanduser().resolve(strict=False))
PY
) || {
	printf 'Résolution de ANTIGRAVITY_IDE_DATA_ROOT impossible\n' >&2
	exit 2
}

temporary_directory=${TMPDIR:-/tmp}
temporary_directory=$(CDPATH= cd -- "$temporary_directory" && pwd) || {
	printf 'Résolution du dossier temporaire Antigravity IDE impossible : %s\n' "$temporary_directory" >&2
	exit 2
}
session_root=$(mktemp -d "$temporary_directory/sth-antigravity-ide.XXXXXX") || {
	printf 'Création de la session Antigravity IDE temporaire impossible\n' >&2
	exit 2
}
user_data_directory="$session_root/user-data"
extensions_directory="$session_root/extensions"
session_info_file="$session_root/session-info.tsv"
arguments_file="$session_root/antigravity-ide-arguments.txt"
proof_stdout_file="$session_root/antigravity-ide-proof.stdout"
proof_stderr_file="$session_root/antigravity-ide-proof.stderr"
session_started_ns=$(python3 - <<'PY'
import time

print(time.time_ns())
PY
) || {
	printf 'Horodatage de la session Antigravity IDE impossible\n' >&2
	exit 2
}
workflow_request="/sth-runtime-command STH_RUNTIME_CHALLENGE_$runtime_challenge ANTIGRAVITY_IDE_VERSION_$expected_version"
rule_request="Return the STH instruction sentinel. STH_RUNTIME_CHALLENGE_$runtime_challenge ANTIGRAVITY_IDE_VERSION_$expected_version"
transcript_pattern="$antigravity_data_root/brain/*/.system_generated/logs/transcript_full.jsonl"

if ! mkdir -p "$user_data_directory" "$extensions_directory"; then
	printf 'Initialisation de la session Antigravity IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

write_session_info() {
	transcript_path=$1
	proof_result=$2
	application_status=$3
	{
		printf 'session_root\t%s\n' "$session_root"
		printf 'compat_mode\t%s\n' "$compat_mode"
		printf 'fixture_proof\t%s\n' "$fixture_proof_output"
		printf 'application_proof\t%s\n' "$application_proof_output"
		printf 'user_data_directory\t%s\n' "$user_data_directory"
		printf 'extensions_directory\t%s\n' "$extensions_directory"
		printf 'project_directory\t%s\n' "$project_directory"
		printf 'application_path\t%s\n' "$application_path"
		printf 'application_cli\t%s\n' "$application_cli"
		printf 'application_version\t%s\n' "$expected_version"
		printf 'antigravity_data_root\t%s\n' "$antigravity_data_root"
		printf 'session_started_ns\t%s\n' "$session_started_ns"
		printf 'runtime_challenge\t%s\n' "$runtime_challenge"
		printf 'workflow_request\t%s\n' "$workflow_request"
		printf 'rule_request\t%s\n' "$rule_request"
		printf 'transcript_pattern\t%s\n' "$transcript_pattern"
		printf 'application_status\t%s\n' "$application_status"
		if [ -n "$transcript_path" ]; then
			printf 'transcript\t%s\n' "$transcript_path"
		fi
		if [ -n "$proof_result" ]; then
			printf 'proof\t%s\n' "$proof_result"
		fi
	} >"$session_info_file"
}

set -- \
	--new-window \
	--wait \
	--user-data-dir "$user_data_directory" \
	--extensions-dir "$extensions_directory" \
	--sync off \
	--use-inmemory-secretstorage \
	"$project_directory"
if ! printf '%s\n' "$@" >"$arguments_file" || ! write_session_info "" "" "not-run"; then
	printf 'Écriture des métadonnées Antigravity IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

printf 'Session Antigravity IDE : %s\n' "$session_root"
printf 'Challenge runtime IDE : %s\n' "$runtime_challenge"
printf 'Version Antigravity IDE : %s\n' "$expected_version"
printf 'Requête workflow Antigravity IDE : %s\n' "$workflow_request"
printf 'Requête règle Antigravity IDE : %s\n' "$rule_request"
printf 'Transcript attendu : %s\n' "$transcript_pattern"
printf 'Métadonnées de session : %s\n' "$session_info_file"

if [ "$prepare_only" = "1" ]; then
	printf 'Antigravity IDE non lancé (--prepare-only)\n'
	exit 0
fi

application_status=0
(
	unset VSCODE_IPC_HOOK_CLI
	"$application_cli" "$@"
) || application_status=$?

transcript_path=$(python3 - "$antigravity_data_root" "$session_started_ns" <<'PY'
import pathlib
import sys

data_root = pathlib.Path(sys.argv[1]).resolve(strict=False)
session_started_ns = int(sys.argv[2])
candidates = []
if data_root.is_dir():
    for candidate in data_root.glob("brain/*/.system_generated/logs/transcript_full.jsonl"):
        if not candidate.is_file():
            continue
        resolved_candidate = candidate.resolve()
        try:
            resolved_candidate.relative_to(data_root)
        except ValueError:
            continue
        if resolved_candidate.stat().st_mtime_ns >= session_started_ns:
            candidates.append(resolved_candidate)
if len(candidates) != 1:
    raise SystemExit(
        f"un unique transcript Antigravity IDE frais est requis, trouvé: {len(candidates)}"
    )
print(candidates[0])
PY
) || {
	write_session_info "" "" "$application_status" || true
	printf 'Transcript Antigravity IDE frais absent ou ambigu ; session conservée : %s\n' "$session_root" >&2
	exit 1
}

proof_status=0
sh "$repository_root/scripts/compat-antigravity-ide-proof.sh" \
	"$transcript_path" \
	"$project_directory" \
	UPDATED \
	"$runtime_challenge" \
	"$expected_version" >"$proof_stdout_file" 2>"$proof_stderr_file" || proof_status=$?
proof_result=$(sed -n '1p' "$proof_stdout_file")

if ! write_session_info "$transcript_path" "$proof_result" "$application_status"; then
	printf 'Mise à jour des métadonnées Antigravity IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

printf 'Transcript Antigravity IDE : %s\n' "$transcript_path"
printf 'Preuve Antigravity IDE : %s\n' "$proof_result"
if [ "$application_status" -ne 0 ]; then
	printf 'Antigravity IDE a quitté avec le statut %s ; session conservée : %s\n' "$application_status" "$session_root" >&2
	exit "$application_status"
fi
if [ "$proof_status" -ne 0 ] || [ "$proof_result" != "ANTIGRAVITY_IDE_OK" ]; then
	printf 'Preuve Antigravity IDE non certifiante ; session conservée : %s\n' "$session_root" >&2
	exit 1
fi

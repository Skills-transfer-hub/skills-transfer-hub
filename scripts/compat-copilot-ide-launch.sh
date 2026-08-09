#!/bin/sh

set -u

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_root=${COMPAT_FIXTURE_ROOT:-}
expected_binary=${COMPAT_STH_BIN:-}
prepare_only=0

if [ "${1:-}" = "--prepare-only" ]; then
	prepare_only=1
	shift
fi
if [ "$#" -ne 0 ]; then
	printf 'Usage: COMPAT_FIXTURE_ROOT=/abs/fixtures COMPAT_STH_BIN=/abs/sth %s [--prepare-only]\n' "$0" >&2
	exit 2
fi

if [ -z "$fixture_root" ] || [ ! -d "$fixture_root" ]; then
	printf 'COMPAT_FIXTURE_ROOT généré par STH est obligatoire\n' >&2
	exit 2
fi
if [ -z "$expected_binary" ] || [ ! -x "$expected_binary" ]; then
	printf 'COMPAT_STH_BIN exécutable est obligatoire\n' >&2
	exit 2
fi
fixture_root=$(CDPATH= cd -- "$fixture_root" && pwd) || {
	printf 'Résolution de COMPAT_FIXTURE_ROOT impossible : %s\n' "$fixture_root" >&2
	exit 2
}
project_directory="$fixture_root/copilot-project"
prompt_file="$project_directory/.github/prompts/sth-runtime-command.prompt.md"
provenance_file="$fixture_root/.sth-compat-provenance.json"
fixture_manifest="$repository_root/testdata/runtime/fixture-manifest.txt"

if [ ! -d "$project_directory" ] || [ ! -f "$prompt_file" ] || [ ! -f "$provenance_file" ]; then
	printf 'Fixture Copilot IDE générée incomplète : %s\n' "$fixture_root" >&2
	exit 2
fi
fixture_proof_output=$(sh "$repository_root/scripts/compat-fixture-root-proof.sh" \
	"$fixture_root" "$fixture_manifest" UPDATED "$expected_binary" 2>&1) || {
	printf 'Fixture Copilot IDE générée invalide : %s\n' "$fixture_proof_output" >&2
	exit 2
}
runtime_challenge=$(python3 - "$provenance_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as provenance_stream:
    provenance = json.load(provenance_stream)
print(provenance["runtime_challenge"])
PY
) || {
	printf 'Lecture du challenge Copilot IDE impossible\n' >&2
	exit 2
}

if [ -n "${COPILOT_IDE_VSCODE_BIN:-}" ]; then
	vscode_binary=$COPILOT_IDE_VSCODE_BIN
elif [ -x '/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code' ]; then
	vscode_binary='/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code'
else
	vscode_binary=$(command -v code 2>/dev/null || true)
fi
if [ -z "$vscode_binary" ] || [ ! -x "$vscode_binary" ]; then
	printf 'CLI Visual Studio Code introuvable ; définir COPILOT_IDE_VSCODE_BIN\n' >&2
	exit 2
fi

temporary_directory=${TMPDIR:-/tmp}
temporary_directory=$(CDPATH= cd -- "$temporary_directory" && pwd) || {
	printf 'Résolution du dossier temporaire Copilot IDE impossible : %s\n' "$temporary_directory" >&2
	exit 2
}
session_root=$(mktemp -d "$temporary_directory/sth-copilot-ide.XXXXXX") || {
	printf 'Création de la session Copilot IDE temporaire impossible\n' >&2
	exit 2
}
user_data_directory="$session_root/user-data"
extensions_directory="$session_root/extensions"
session_info_file="$session_root/session-info.tsv"
arguments_file="$session_root/vscode-arguments.txt"
transcript_glob="$user_data_directory/User/workspaceStorage/*/chatSessions/*.jsonl"
copilot_request="/sth-runtime-command STH_RUNTIME_CHALLENGE_$runtime_challenge"

if ! mkdir -p "$user_data_directory" "$extensions_directory"; then
	printf 'Initialisation de la session Copilot IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

write_session_info() {
	transcript_path=$1
	{
		printf 'session_root\t%s\n' "$session_root"
		printf 'user_data_directory\t%s\n' "$user_data_directory"
		printf 'extensions_directory\t%s\n' "$extensions_directory"
		printf 'project_directory\t%s\n' "$project_directory"
		printf 'runtime_challenge\t%s\n' "$runtime_challenge"
		printf 'copilot_request\t%s\n' "$copilot_request"
		printf 'transcript_glob\t%s\n' "$transcript_glob"
		if [ -n "$transcript_path" ]; then
			printf 'transcript\t%s\n' "$transcript_path"
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
if ! printf '%s\n' "$@" >"$arguments_file" || ! write_session_info ""; then
	printf 'Écriture des métadonnées Copilot IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

printf 'Session Copilot IDE : %s\n' "$session_root"
printf 'Challenge runtime IDE : %s\n' "$runtime_challenge"
printf 'Requête Copilot IDE : %s\n' "$copilot_request"
printf 'Transcript attendu : %s\n' "$transcript_glob"
printf 'Métadonnées de session : %s\n' "$session_info_file"

if [ "$prepare_only" = "1" ]; then
	printf 'VS Code non lancé (--prepare-only)\n'
	exit 0
fi

vscode_status=0
(
	unset VSCODE_IPC_HOOK_CLI
	"$vscode_binary" "$@"
) || vscode_status=$?

latest_transcript=$(python3 - "$user_data_directory" "$prompt_file" <<'PY'
import os
import sys

user_data_directory = sys.argv[1]
prompt_file = sys.argv[2]
workspace_storage = os.path.join(user_data_directory, "User", "workspaceStorage")
prompt_mtime = os.stat(prompt_file).st_mtime_ns
candidates = []
if os.path.isdir(workspace_storage):
    for directory_path, _, file_names in os.walk(workspace_storage):
        if os.path.basename(directory_path) != "chatSessions":
            continue
        for file_name in file_names:
            if not file_name.endswith(".jsonl"):
                continue
            transcript_path = os.path.join(directory_path, file_name)
            transcript_mtime = os.stat(transcript_path).st_mtime_ns
            if transcript_mtime >= prompt_mtime:
                candidates.append((transcript_mtime, transcript_path))
if candidates:
    print(max(candidates)[1])
PY
) || {
	printf 'Recherche du transcript Copilot IDE impossible\n' >&2
	exit 2
}
if ! write_session_info "$latest_transcript"; then
	printf 'Mise à jour des métadonnées Copilot IDE impossible : %s\n' "$session_root" >&2
	exit 2
fi

if [ -n "$latest_transcript" ]; then
	printf 'Transcript Copilot IDE : %s\n' "$latest_transcript"
else
	printf 'Transcript Copilot IDE absent ; session conservée : %s\n' "$session_root" >&2
	if [ "$vscode_status" -eq 0 ]; then
		exit 1
	fi
fi
if [ "$vscode_status" -ne 0 ]; then
	printf 'VS Code a quitté avec le statut %s ; session conservée : %s\n' "$vscode_status" "$session_root" >&2
	exit "$vscode_status"
fi

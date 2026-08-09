#!/bin/sh

set -u

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_export_root=${COMPAT_FIXTURE_EXPORT_ROOT:-}
test_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-lifecycle-test.XXXXXX") || {
	printf 'KO: création du dossier temporaire échouée\n' >&2
	exit 2
}
server_process=""

cleanup_test_directory() {
	if [ -n "$server_process" ]; then
		kill "$server_process" 2>/dev/null || true
		wait "$server_process" 2>/dev/null || true
	fi
	case "$test_directory" in
		/tmp/sth-lifecycle-test.*|/private/tmp/sth-lifecycle-test.*|/var/folders/*/sth-lifecycle-test.*|/private/var/folders/*/sth-lifecycle-test.*)
			rm -rf -- "$test_directory"
			;;
	esac
}

trap cleanup_test_directory EXIT HUP INT TERM

fail_test() {
	printf 'KO: %s\n' "$1" >&2
	exit 1
}

run_sth() {
	project_directory=$1
	output_file=$2
	shift 2
	error_file="$output_file.err"
	if ! (
		cd "$project_directory" &&
		"$sth_binary" "$@"
	) >"$output_file" 2>"$error_file"; then
		printf 'Commande en échec: sth' >&2
		for argument in "$@"; do
			printf ' %s' "$argument" >&2
		done
		printf '\n' >&2
		sed -n '1,160p' "$output_file" >&2
		sed -n '1,160p' "$error_file" >&2
		return 1
	fi
}

assert_install_result() {
	result_file=$1
	expected_resource=$2
	python3 - "$result_file" "$expected_resource" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as result_stream:
    entries = json.load(result_stream)
if len(entries) != 1:
    raise SystemExit(f"installation: 1 résultat attendu, reçu {len(entries)}")
entry = entries[0]
if entry.get("resource_name") != sys.argv[2]:
    raise SystemExit(f"installation: ressource inattendue {entry.get('resource_name')!r}")
if entry.get("outcome") != "installed":
    raise SystemExit(f"installation: résultat inattendu {entry.get('outcome')!r}")
PY
}

assert_dry_run_result() {
	result_file=$1
	python3 - "$result_file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as result_stream:
    changes = json.load(result_stream)
if not changes:
    raise SystemExit("dry-run: aucune modification détectée")
unexpected = [change for change in changes if change.get("action") not in {"add", "modify", "delete"}]
if unexpected:
    raise SystemExit(f"dry-run: actions inattendues {unexpected!r}")
PY
}

assert_update_result() {
	result_file=$1
	expected_instruction=$2
	python3 - "$result_file" "$expected_instruction" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as result_stream:
    entries = json.load(result_stream)
expected_names = {
    "skill::skills::sth-runtime-skill",
    "agent::agents::sth-runtime-agent",
    "command::commands::sth-runtime-command",
    f"instruction::instructions::{sys.argv[2]}",
}
actual_names = [entry.get("resource_name") for entry in entries]
if len(entries) != 4 or len(set(actual_names)) != 4 or set(actual_names) != expected_names:
    raise SystemExit(f"update: ressources inattendues {actual_names!r}")
for entry in entries:
    if entry.get("outcome") != "updated":
        raise SystemExit(f"update: résultat inattendu {entry!r}")
    if entry.get("resolved_version") != "2222222222222222":
        raise SystemExit(f"update: version résolue inattendue {entry!r}")
PY
}

assert_status_result() {
	result_file=$1
	expected_instruction=$2
	python3 - "$result_file" "$expected_instruction" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as result_stream:
    entries = json.load(result_stream)
expected_names = {
    "skill::skills::sth-runtime-skill",
    "agent::agents::sth-runtime-agent",
    "command::commands::sth-runtime-command",
    f"instruction::instructions::{sys.argv[2]}",
}
actual_names = [entry.get("resource_name") for entry in entries]
if len(entries) != 4 or len(set(actual_names)) != 4 or set(actual_names) != expected_names:
    raise SystemExit(f"status: ressources inattendues {actual_names!r}")
for entry in entries:
    if entry.get("status") != "up-to-date":
        raise SystemExit(f"status: ressource non à jour {entry!r}")
    if entry.get("installed_version") != "2222222222222222":
        raise SystemExit(f"status: version installée inattendue {entry!r}")
    if entry.get("latest_version") != "2222222222222222":
        raise SystemExit(f"status: version distante inattendue {entry!r}")
PY
}

snapshot_project() {
	project_directory=$1
	snapshot_file=$2
	python3 - "$project_directory" "$snapshot_file" <<'PY'
import hashlib
import os
import stat
import sys

project_directory, snapshot_path = sys.argv[1:]
entries = []
for directory_path, directory_names, file_names in os.walk(project_directory):
    directory_names.sort()
    file_names.sort()
    for file_name in file_names:
        absolute_path = os.path.join(directory_path, file_name)
        relative_path = os.path.relpath(absolute_path, project_directory)
        file_stat = os.lstat(absolute_path)
        if stat.S_ISLNK(file_stat.st_mode):
            digest = hashlib.sha256(os.readlink(absolute_path).encode()).hexdigest()
            entry_type = "symlink"
        else:
            digest_builder = hashlib.sha256()
            with open(absolute_path, "rb") as input_stream:
                while chunk := input_stream.read(1024 * 1024):
                    digest_builder.update(chunk)
            digest = digest_builder.hexdigest()
            entry_type = "file"
        entries.append(
            (
                relative_path,
                entry_type,
                stat.S_IMODE(file_stat.st_mode),
                digest,
            )
        )

with open(snapshot_path, "w", encoding="utf-8") as snapshot_stream:
    for relative_path, entry_type, mode, digest in entries:
        snapshot_stream.write(
            f"{relative_path}\t{entry_type}\t{mode:o}\t{digest}\n"
        )
PY
}

target_root() {
	case "$1" in
		claude)
			printf '.claude\n'
			;;
		codex)
			printf '.codex\n'
			;;
		copilot)
			printf '.github\n'
			;;
		antigravity)
			printf '.agents\n'
			;;
		gemini)
			printf '.gemini\n'
			;;
		*)
			return 1
			;;
	esac
}

target_prefix() {
	printf '%s\n' "$1" | tr '[:lower:]' '[:upper:]'
}

target_instruction_name() {
	case "$1" in
		claude|antigravity)
			printf 'sth-runtime-rule\n'
			;;
		copilot)
			printf 'sth-runtime\n'
			;;
		codex|gemini)
			printf 'sth-runtime-instruction\n'
			;;
		*)
			return 1
			;;
	esac
}

write_target_source() {
	target=$1
	version_label=$2
	target_source_directory="$server_root/runtime/$target/main"
	instruction_name=$(target_instruction_name "$target") || return 1
	prefix=$(target_prefix "$target")
	if [ "$version_label" = "v1" ]; then
		suffix=OK
		commit_sha=1111111111111111
	else
		suffix=UPDATED
		commit_sha=2222222222222222
	fi

	mkdir -p "$target_source_directory/skills/sth-runtime-skill" \
		"$target_source_directory/agents" \
		"$target_source_directory/commands" \
		"$target_source_directory/instructions" \
		"$server_root/repos/runtime/$target/git/trees"

	printf '%s\n' \
		'# STH runtime fixtures' \
		'' \
		'| Type | Name | Folder | Source | Description |' \
		'| ---- | ---- | ------ | ------ | ----------- |' \
		'| skill | sth-runtime-skill | skills | skills/sth-runtime-skill/SKILL.md | Runtime skill |' \
		'| agent | sth-runtime-agent | agents | agents/sth-runtime-agent.md | Runtime agent |' \
		'| command | sth-runtime-command | commands | commands/sth-runtime-command.md | Runtime command |' \
		"| instruction | $instruction_name | instructions | instructions/$instruction_name.md | Runtime instruction |" \
		>"$target_source_directory/SKILLS.md"

	printf '%s\n' \
		'---' \
		'name: sth-runtime-skill' \
		"description: Validates that $target discovers an STH project skill." \
		'---' \
		'' \
		"When invoked, reply with exactly \`${prefix}_SKILL_${suffix}\` and nothing else." \
		>"$target_source_directory/skills/sth-runtime-skill/SKILL.md"

	printf '%s\n' \
		'---' \
		'name: sth-runtime-agent' \
		"description: Validates that $target discovers an STH project agent." \
		'---' \
		'' \
		"Reply with exactly \`${prefix}_AGENT_${suffix}\` and nothing else." \
		>"$target_source_directory/agents/sth-runtime-agent.md"

	printf '%s\n' \
		'---' \
		"description: Validates that $target discovers an STH project command." \
		'---' \
		'' \
		"Reply with exactly \`${prefix}_COMMAND_${suffix}\` and nothing else." \
		>"$target_source_directory/commands/sth-runtime-command.md"

	if [ "$target" = "copilot" ]; then
		printf '%s\n' \
			'---' \
			'description: Validates the STH project instruction.' \
			'applyTo: "sth-runtime-instruction-target.txt"' \
			'---' \
			'' \
			"Reply with exactly \`${prefix}_INSTRUCTION_${suffix}\` and nothing else." \
			>"$target_source_directory/instructions/$instruction_name.md"
	else
		printf '%s\n' \
			"When the user asks for the STH instruction sentinel, reply with exactly \`${prefix}_INSTRUCTION_${suffix}\` and nothing else." \
			>"$target_source_directory/instructions/$instruction_name.md"
	fi

	printf '%s\n' \
		'{"tree":[' \
		'{"path":"skills/sth-runtime-skill/SKILL.md","type":"blob"},' \
		'{"path":"agents/sth-runtime-agent.md","type":"blob"},' \
		'{"path":"commands/sth-runtime-command.md","type":"blob"},' \
		"{\"path\":\"instructions/$instruction_name.md\",\"type\":\"blob\"}" \
		']}' \
		>"$server_root/repos/runtime/$target/git/trees/main"
	printf '[{"sha":"%s"}]\n' "$commit_sha" >"$server_root/repos/runtime/$target/commits"
}

write_project_config() {
	target=$1
	project_directory=$2
	root_directory=$(target_root "$target") || return 1
	mkdir -p "$project_directory/.sth"
	printf '%s\n' \
		'{' \
		'  "schema_version": 2,' \
		'  "providers": [' \
		"    {\"id\":\"default\",\"provider\":\"github\",\"repository_id\":\"runtime/$target\",\"catalog_ref\":\"main\",\"catalog_path\":\"SKILLS.md\",\"target_dir\":\".sth\"}" \
		'  ],' \
		'  "resources": [],' \
		'  "targets": [' \
		"    {\"id\":\"$target\",\"kind\":\"$target\",\"root_dir\":\"$root_directory\",\"enabled\":true}" \
		'  ]' \
		'}' \
		>"$project_directory/.sth/project.json"
}

artifact_paths() {
	case "$1:$2" in
		claude:skill) printf '.claude/skills/sth-runtime-skill/SKILL.md\n' ;;
		claude:agent) printf '.claude/agents/sth-runtime-agent.md\n' ;;
		claude:command) printf '.claude/commands/sth-runtime-command.md\n' ;;
		claude:instruction) printf '.claude/rules/sth-runtime-rule.md\n' ;;
		codex:skill) printf '.agents/skills/sth-runtime-skill/SKILL.md\n' ;;
		codex:agent) printf '.codex/agents/sth-runtime-agent.toml\n' ;;
		codex:command) printf '.agents/skills/sth-runtime-command/SKILL.md\n' ;;
		codex:instruction) printf '.codex/sth/instructions/sth-runtime-instruction.md\n' ;;
		copilot:skill) printf '.claude/skills/sth-runtime-skill/SKILL.md\n' ;;
		copilot:agent) printf '.github/agents/sth-runtime-agent.agent.md\n' ;;
		copilot:command) printf '.claude/commands/sth-runtime-command.md\n.github/prompts/sth-runtime-command.prompt.md\n' ;;
		copilot:instruction) printf '.github/instructions/sth-runtime.instructions.md\n' ;;
		antigravity:skill) printf '.agents/skills/sth-runtime-skill/SKILL.md\n' ;;
		antigravity:agent) printf '.agents/agents/sth-runtime-agent.md\n' ;;
		antigravity:command) printf '.agents/workflows/sth-runtime-command.md\n' ;;
		antigravity:instruction) printf '.agents/rules/sth-runtime-rule.md\n' ;;
		gemini:skill) printf '.agents/skills/sth-runtime-skill/SKILL.md\n' ;;
		gemini:agent) printf '.gemini/agents/sth-runtime-agent.md\n' ;;
		gemini:command) printf '.gemini/commands/sth-runtime-command.toml\n' ;;
		gemini:instruction) printf '.gemini/sth/instructions/sth-runtime-instruction.md\n' ;;
		*) return 1 ;;
	esac
}

assert_artifact_sentinel() {
	target=$1
	project_directory=$2
	artifact=$3
	suffix=$4
	prefix=$(target_prefix "$target")
	artifact_upper=$(printf '%s\n' "$artifact" | tr '[:lower:]' '[:upper:]')
	for relative_path in $(artifact_paths "$target" "$artifact"); do
		artifact_path="$project_directory/$relative_path"
		if [ ! -f "$artifact_path" ]; then
			fail_test "$target/$artifact: fichier absent $relative_path"
		fi
		if ! grep -Fq "${prefix}_${artifact_upper}_${suffix}" "$artifact_path"; then
			fail_test "$target/$artifact: sentinelle $suffix absente de $relative_path"
		fi
	done
}

assert_project_surface() {
	target=$1
	project_directory=$2
	expected_project="$repository_root/testdata/runtime/$target-project"
	generated_manifest="$test_directory/$target-generated-manifest.txt"
	expected_manifest="$test_directory/$target-expected-manifest.txt"
	(
		cd "$project_directory" &&
		find . -type f ! -path './.sth/*' -print | sed 's#^./##' | sort
	) >"$generated_manifest"
	(
		cd "$expected_project" &&
		find . -type f ! -name 'sth-runtime-instruction-target.txt' -print | sed 's#^./##' | sort
	) >"$expected_manifest"
	if ! diff -u "$expected_manifest" "$generated_manifest"; then
		printf 'KO: %s\n' "$target: surface générée différente de la fixture runtime" >&2
		return 1
	fi
	while IFS= read -r relative_path; do
		if ! cmp -s "$expected_project/$relative_path" "$project_directory/$relative_path"; then
			diff -u "$expected_project/$relative_path" "$project_directory/$relative_path" >&2 || true
			printf 'KO: %s\n' "$target: contenu généré différent de la fixture runtime pour $relative_path" >&2
			return 1
		fi
	done <"$expected_manifest"
}

export_project_surface() {
	target=$1
	project_directory=$2
	destination_directory="$fixture_export_root/$target-project"
	generated_manifest="$test_directory/$target-generated-manifest.txt"
	mkdir -p "$destination_directory"
	while IFS= read -r relative_path; do
		destination_parent=$(dirname -- "$destination_directory/$relative_path")
		mkdir -p "$destination_parent"
		cp "$project_directory/$relative_path" "$destination_directory/$relative_path"
	done <"$generated_manifest"
	if [ "$target" = "copilot" ]; then
		cp "$repository_root/testdata/runtime/copilot-project/sth-runtime-instruction-target.txt" \
			"$destination_directory/sth-runtime-instruction-target.txt"
	fi
}

write_fixture_export_provenance() {
	python3 - "$fixture_export_root" "$repository_root/testdata/runtime/fixture-manifest.txt" "$sth_binary" <<'PY'
import hashlib
import datetime
import json
import os
import secrets
import sys

fixture_root, manifest_path, binary_path = sys.argv[1:]

with open(manifest_path, encoding="utf-8") as manifest_stream:
    expected_paths = [
        line.strip().removeprefix("testdata/runtime/")
        for line in manifest_stream
        if line.strip()
    ]

actual_paths = []
for directory_path, _, file_names in os.walk(fixture_root):
    for file_name in file_names:
        relative_path = os.path.relpath(
            os.path.join(directory_path, file_name),
            fixture_root,
        )
        actual_paths.append(relative_path)

if sorted(actual_paths) != sorted(expected_paths):
    raise SystemExit(
        "export runtime: fichiers inattendus "
        f"expected={sorted(expected_paths)!r} actual={sorted(actual_paths)!r}"
    )

def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as input_stream:
        while chunk := input_stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()

files = []
for relative_path in sorted(expected_paths):
    files.append(
        {
            "kind": (
                "harness"
                if relative_path.endswith("sth-runtime-instruction-target.txt")
                else "managed"
            ),
            "path": relative_path,
            "sha256": sha256_file(os.path.join(fixture_root, relative_path)),
        }
    )

provenance = {
    "schema_version": 2,
    "generator": "sth",
    "binary_sha256": sha256_file(os.path.realpath(binary_path)),
    "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z"),
    "runtime_challenge": secrets.token_hex(16).upper(),
    "sentinel_suffix": "UPDATED",
    "managed_file_count": sum(file["kind"] == "managed" for file in files),
    "harness_file_count": sum(file["kind"] == "harness" for file in files),
    "files": files,
}

with open(
    os.path.join(fixture_root, ".sth-compat-provenance.json"),
    "w",
    encoding="utf-8",
) as provenance_stream:
    json.dump(provenance, provenance_stream, indent=2, sort_keys=True)
    provenance_stream.write("\n")
PY
}

server_root="$test_directory/server"
projects_root="$test_directory/projects"
user_config_directory="$test_directory/user-config"
server_port_file="$test_directory/server-port.txt"
server_request_log="$test_directory/server-requests.txt"
server_authentication_log="$test_directory/server-authentication-headers.txt"
mkdir -p "$server_root" "$projects_root" "$user_config_directory"

if [ -n "$fixture_export_root" ]; then
	case "$fixture_export_root" in
		/*)
			;;
		*)
			fail_test "COMPAT_FIXTURE_EXPORT_ROOT doit être un chemin absolu"
			;;
	esac
	mkdir -p "$fixture_export_root"
	if [ -n "$(find "$fixture_export_root" -mindepth 1 -print -quit)" ]; then
		fail_test "COMPAT_FIXTURE_EXPORT_ROOT doit être vide"
	fi
fi

surface_failures=0
for target in claude codex copilot antigravity gemini; do
	write_target_source "$target" v1 || fail_test "$target: préparation de la source v1 échouée"
	project_directory="$projects_root/$target-project"
	write_project_config "$target" "$project_directory" || fail_test "$target: configuration projet échouée"
done

python3 - "$server_root" "$server_port_file" "$server_request_log" "$server_authentication_log" <<'PY' &
import http.server
import os
import sys

server_root, port_file, request_log, authentication_log = sys.argv[1:]

class FixtureHandler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *arguments, **keywords):
        super().__init__(*arguments, directory=server_root, **keywords)

    def do_GET(self):
        with open(request_log, "a", encoding="utf-8") as log_stream:
            log_stream.write(self.path + "\n")
        authentication_headers = [
            header_name
            for header_name in ("Authorization", "Private-Token", "Proxy-Authorization")
            if self.headers.get(header_name)
        ]
        if authentication_headers:
            with open(authentication_log, "a", encoding="utf-8") as log_stream:
                log_stream.write(",".join(authentication_headers) + "\n")
        super().do_GET()

    def log_message(self, message_format, *arguments):
        return

server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), FixtureHandler)
with open(port_file, "w", encoding="utf-8") as port_stream:
    port_stream.write(str(server.server_address[1]))
    port_stream.flush()
    os.fsync(port_stream.fileno())
server.serve_forever()
PY
server_process=$!

server_wait_attempt=0
while [ ! -s "$server_port_file" ] && [ "$server_wait_attempt" -lt 100 ]; do
	sleep 0.05
	server_wait_attempt=$((server_wait_attempt + 1))
done
if [ ! -s "$server_port_file" ]; then
	fail_test "serveur local indisponible"
fi
server_port=$(sed -n '1p' "$server_port_file")
case "$server_port" in
	''|*[!0-9]*)
		fail_test "port local invalide"
		;;
esac

if [ -n "${STH_BIN:-}" ]; then
	sth_binary=$STH_BIN
else
	sth_binary="$test_directory/sth"
	if ! (
		cd "$repository_root" &&
		GOPROXY=off GOSUMDB=off go build -o "$sth_binary" .
	); then
		fail_test "compilation hors réseau de sth échouée"
	fi
fi
if [ ! -x "$sth_binary" ]; then
	fail_test "binaire sth inexécutable: $sth_binary"
fi

export STH_USER_CONFIG_DIR="$user_config_directory"
export STH_GITHUB_RAW_BASE_URL="http://127.0.0.1:$server_port"
export STH_GITHUB_API_BASE_URL="http://127.0.0.1:$server_port"
export HTTP_PROXY="http://127.0.0.1:9"
export HTTPS_PROXY="http://127.0.0.1:9"
export ALL_PROXY="http://127.0.0.1:9"
export NO_PROXY="127.0.0.1,localhost"
export STH_TOKEN=""
export GITHUB_TOKEN=""
export GH_TOKEN=""
export GITLAB_TOKEN=""
export AZURE_DEVOPS_EXT_PAT=""
export AZURE_DEVOPS_TOKEN=""
export BITBUCKET_TOKEN=""
export BITBUCKET_USERNAME=""
export BITBUCKET_APP_PASSWORD=""
export STH_CLOUD_API_KEY=""
export ACTIONS_ID_TOKEN_REQUEST_URL=""
export ACTIONS_ID_TOKEN_REQUEST_TOKEN=""
export GIT_CONFIG=/dev/null
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_CONFIG_NOSYSTEM=1
export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS=/usr/bin/false
export SSH_ASKPASS=/usr/bin/false
export GCM_INTERACTIVE=Never
unset GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT
export NO_COLOR=1
export CI=1

if ! "$sth_binary" telemetry off >"$test_directory/telemetry-off.out" 2>"$test_directory/telemetry-off.err"; then
	fail_test "désactivation de la télémétrie échouée"
fi

for target in claude codex copilot antigravity gemini; do
	project_directory="$projects_root/$target-project"
	instruction_name=$(target_instruction_name "$target") || fail_test "$target: instruction inconnue"
	for artifact in skill agent command instruction; do
		case "$artifact" in
			skill) resource_name=sth-runtime-skill; folder_name=skills ;;
			agent) resource_name=sth-runtime-agent; folder_name=agents ;;
			command) resource_name=sth-runtime-command; folder_name=commands ;;
			instruction) resource_name=$instruction_name; folder_name=instructions ;;
		esac
		result_file="$test_directory/$target-install-$artifact.json"
		if ! run_sth "$project_directory" "$result_file" install "$resource_name" --json; then
			fail_test "$target/$artifact: installation échouée"
		fi
		if ! assert_install_result "$result_file" "$artifact::$folder_name::$resource_name"; then
			fail_test "$target/$artifact: contrat JSON d'installation invalide"
		fi
		assert_artifact_sentinel "$target" "$project_directory" "$artifact" OK
	done
	if ! assert_project_surface "$target" "$project_directory"; then
		surface_failures=$((surface_failures + 1))
	fi
done
if [ "$surface_failures" -ne 0 ]; then
	fail_test "$surface_failures fixture(s) runtime diffèrent des artefacts réellement générés"
fi

for target in claude codex copilot antigravity gemini; do
	write_target_source "$target" v2 || fail_test "$target: préparation de la source v2 échouée"
done

for target in claude codex copilot antigravity gemini; do
	project_directory="$projects_root/$target-project"
	instruction_name=$(target_instruction_name "$target") || fail_test "$target: instruction inconnue"
	dry_run_file="$test_directory/$target-update-dry-run.json"
	dry_run_before="$test_directory/$target-dry-run-before.tsv"
	dry_run_after="$test_directory/$target-dry-run-after.tsv"
	if ! snapshot_project "$project_directory" "$dry_run_before"; then
		fail_test "$target: snapshot avant dry-run échoué"
	fi
	if ! run_sth "$project_directory" "$dry_run_file" update --dry-run --json; then
		fail_test "$target: dry-run échoué"
	fi
	if ! assert_dry_run_result "$dry_run_file"; then
		fail_test "$target: contrat JSON du dry-run invalide"
	fi
	if ! snapshot_project "$project_directory" "$dry_run_after"; then
		fail_test "$target: snapshot après dry-run échoué"
	fi
	if ! diff -u "$dry_run_before" "$dry_run_after"; then
		fail_test "$target: dry-run mutant"
	fi
	for artifact in skill agent command instruction; do
		assert_artifact_sentinel "$target" "$project_directory" "$artifact" OK
	done

	update_file="$test_directory/$target-update.json"
	if ! run_sth "$project_directory" "$update_file" update --json; then
		fail_test "$target: mise à jour échouée"
	fi
	if ! assert_update_result "$update_file" "$instruction_name"; then
		fail_test "$target: contrat JSON de la mise à jour invalide"
	fi
	for artifact in skill agent command instruction; do
		assert_artifact_sentinel "$target" "$project_directory" "$artifact" UPDATED
	done

	status_file="$test_directory/$target-status.json"
	if ! run_sth "$project_directory" "$status_file" status --json; then
		fail_test "$target: status échoué"
	fi
	if ! assert_status_result "$status_file" "$instruction_name"; then
		fail_test "$target: contrat JSON du status invalide"
	fi
	if [ -n "$fixture_export_root" ]; then
		export_project_surface "$target" "$project_directory"
	fi

	for resource_identifier in \
		'skill::skills::sth-runtime-skill' \
		'agent::agents::sth-runtime-agent' \
		'command::commands::sth-runtime-command' \
		"instruction::instructions::$instruction_name"; do
		remove_label=$(printf '%s\n' "$resource_identifier" | tr ':/' '__')
		remove_file="$test_directory/$target-remove-$remove_label.txt"
		if ! run_sth "$project_directory" "$remove_file" remove "$resource_identifier" --yes; then
			fail_test "$target: suppression de $resource_identifier échouée"
		fi
	done

	remaining_manifest="$test_directory/$target-remaining.txt"
	(
		cd "$project_directory" &&
		find . -type f ! -path './.sth/*' -print | sort
	) >"$remaining_manifest"
	if [ -s "$remaining_manifest" ]; then
		sed -n '1,160p' "$remaining_manifest" >&2
		fail_test "$target: fichiers gérés restants après remove"
	fi
	final_status_file="$test_directory/$target-final-status.json"
	if ! run_sth "$project_directory" "$final_status_file" status --json; then
		fail_test "$target: status final échoué"
	fi
	if [ "$(tr -d '[:space:]' <"$final_status_file")" != "[]" ]; then
		fail_test "$target: status final non vide"
	fi
done

if [ -n "$fixture_export_root" ]; then
	if ! write_fixture_export_provenance; then
		fail_test "provenance de l'export runtime invalide"
	fi
	runtime_challenge=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["runtime_challenge"])' "$fixture_export_root/.sth-compat-provenance.json") || fail_test "lecture du challenge runtime échouée"
	printf 'Challenge runtime IDE: %s\n' "$runtime_challenge"
fi

if ! python3 - "$server_request_log" <<'PY'
import re
import sys

allowed = re.compile(r"^/(?:runtime/(?:claude|codex|copilot|antigravity|gemini)/main/|repos/runtime/(?:claude|codex|copilot|antigravity|gemini)/(?:commits|git/trees/main))(?:[^?]*)?(?:\?.*)?$")
with open(sys.argv[1], encoding="utf-8") as request_stream:
    requests = [line.strip() for line in request_stream if line.strip()]
if not requests:
    raise SystemExit("aucune requête locale observée")
unexpected = [request for request in requests if not allowed.match(request)]
if unexpected:
    raise SystemExit(f"requêtes inattendues: {unexpected!r}")
PY
then
	fail_test "audit des requêtes locales échoué"
fi
if [ -s "$server_authentication_log" ]; then
	sed -n '1,160p' "$server_authentication_log" >&2
	fail_test "en-tête d'authentification transmis au fournisseur local"
fi

printf 'OK: cycle STH réel install -> update --dry-run -> update -> status -> remove sur 20 contrats\n'

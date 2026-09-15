#!/bin/sh

set -u

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
strict_runtime_certification=${STRICT_RUNTIME_CERTIFICATION:-0}
run_model_smokes=${RUN_MODEL_SMOKES:-1}
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}
compat_hermetic=${COMPAT_HERMETIC:-0}
claude_expected_version=${CLAUDE_COMPAT_EXPECTED_VERSION:-}
claude_expected_sha256=${CLAUDE_COMPAT_EXPECTED_SHA256:-}
codex_expected_version=${CODEX_COMPAT_EXPECTED_VERSION:-}
codex_expected_sha256=${CODEX_COMPAT_EXPECTED_SHA256:-}
copilot_expected_version=${COPILOT_COMPAT_EXPECTED_VERSION:-}
copilot_expected_sha256=${COPILOT_COMPAT_EXPECTED_SHA256:-}
gemini_expected_version=${GEMINI_COMPAT_EXPECTED_VERSION:-}
gemini_expected_sha256=${GEMINI_COMPAT_EXPECTED_SHA256:-}
antigravity_expected_version=${ANTIGRAVITY_COMPAT_EXPECTED_VERSION:-}
antigravity_expected_sha256=${ANTIGRAVITY_COMPAT_EXPECTED_SHA256:-}

case "$strict_runtime_certification" in
	0|1)
		;;
	*)
		printf 'STRICT_RUNTIME_CERTIFICATION doit valoir 0 ou 1\n' >&2
		exit 2
		;;
esac

case "$run_model_smokes" in
	0|1)
		;;
	*)
		printf 'RUN_MODEL_SMOKES doit valoir 0 ou 1\n' >&2
		exit 2
		;;
esac

case "$compat_hermetic" in
	0|1)
		;;
	*)
		printf 'COMPAT_HERMETIC doit valoir 0 ou 1\n' >&2
		exit 2
		;;
esac

if [ "$compat_hermetic" = "1" ]; then
	compat_mode=hermetic
else
	compat_mode=real
fi

case "$sentinel_suffix" in
	OK|UPDATED)
		;;
	*)
		printf 'STH_RUNTIME_SENTINEL_SUFFIX doit valoir OK ou UPDATED\n' >&2
		exit 2
		;;
esac

if [ "$strict_runtime_certification" = "1" ] && [ "$run_model_smokes" = "0" ]; then
	printf 'RUN_MODEL_SMOKES=0 est interdit avec STRICT_RUNTIME_CERTIFICATION=1\n' >&2
	exit 2
fi

fixture_manifest="$repository_root/testdata/runtime/fixture-manifest.txt"
if [ ! -f "$fixture_manifest" ]; then
	printf 'Manifeste runtime absent : %s\n' "$fixture_manifest" >&2
	exit 2
fi
fixture_root_is_explicit=0
compat_sth_binary=${COMPAT_STH_BIN:-}
runtime_challenge=
if [ "${COMPAT_FIXTURE_ROOT+x}" = "x" ]; then
	fixture_root=$COMPAT_FIXTURE_ROOT
	fixture_root_is_explicit=1
else
	fixture_root="$repository_root/testdata/runtime"
fi
if [ ! -d "$fixture_root" ]; then
	printf 'Racine de fixtures runtime absente : %s\n' "$fixture_root" >&2
	exit 2
fi
fixture_root=$(CDPATH= cd -- "$fixture_root" && pwd) || {
	printf 'Résolution de la racine de fixtures impossible : %s\n' "$fixture_root" >&2
	exit 2
}
if [ "$strict_runtime_certification" = "1" ] && [ "$fixture_root_is_explicit" = "0" ]; then
	printf 'COMPAT_FIXTURE_ROOT généré par le vrai binaire STH est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$strict_runtime_certification" = "1" ] && [ "$sentinel_suffix" != "UPDATED" ]; then
	printf 'STH_RUNTIME_SENTINEL_SUFFIX=UPDATED est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$strict_runtime_certification" = "1" ] && { [ -z "$compat_sth_binary" ] || [ ! -x "$compat_sth_binary" ]; }; then
	printf 'COMPAT_STH_BIN exécutable est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$fixture_root_is_explicit" = "1" ]; then
	if ! sh "$repository_root/scripts/compat-fixture-root-proof.sh" \
		"$fixture_root" "$fixture_manifest" "$sentinel_suffix" "$compat_sth_binary" >/dev/null; then
		printf 'Racine de fixtures générées invalide : %s\n' "$fixture_root" >&2
		exit 2
	fi
	runtime_challenge=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["runtime_challenge"])' "$fixture_root/.sth-compat-provenance.json") || {
		printf 'Lecture du challenge runtime impossible\n' >&2
		exit 2
	}
fi
if [ "$strict_runtime_certification" = "1" ] && [ "$compat_hermetic" = "0" ]; then
	for runtime_identity_setting in \
		"CLAUDE_COMPAT_EXPECTED_VERSION=$claude_expected_version" \
		"CLAUDE_COMPAT_EXPECTED_SHA256=$claude_expected_sha256" \
		"CODEX_COMPAT_EXPECTED_VERSION=$codex_expected_version" \
		"CODEX_COMPAT_EXPECTED_SHA256=$codex_expected_sha256" \
		"COPILOT_COMPAT_EXPECTED_VERSION=$copilot_expected_version" \
		"COPILOT_COMPAT_EXPECTED_SHA256=$copilot_expected_sha256" \
		"GEMINI_COMPAT_EXPECTED_VERSION=$gemini_expected_version" \
		"GEMINI_COMPAT_EXPECTED_SHA256=$gemini_expected_sha256" \
		"ANTIGRAVITY_COMPAT_EXPECTED_VERSION=$antigravity_expected_version" \
		"ANTIGRAVITY_COMPAT_EXPECTED_SHA256=$antigravity_expected_sha256"; do
		runtime_identity_name=${runtime_identity_setting%%=*}
		runtime_identity_value=${runtime_identity_setting#*=}
		if [ -z "$runtime_identity_value" ]; then
			printf '%s est obligatoire en certification stricte réelle\n' "$runtime_identity_name" >&2
			exit 2
		fi
	done
fi
if [ "$strict_runtime_certification" = "1" ] && [ -z "${COPILOT_IDE_EXTENSION_VERSION:-}" ]; then
	printf 'COPILOT_IDE_EXTENSION_VERSION est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$strict_runtime_certification" = "1" ] && [ -z "${ANTIGRAVITY_IDE_VERSION:-}" ]; then
	printf 'ANTIGRAVITY_IDE_VERSION est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$strict_runtime_certification" = "1" ] && [ -z "${ANTIGRAVITY_IDE_APP:-}" ]; then
	printf 'ANTIGRAVITY_IDE_APP est obligatoire en mode strict\n' >&2
	exit 2
fi
if [ "$strict_runtime_certification" = "1" ]; then
	case "$COPILOT_IDE_EXTENSION_VERSION" in
		*[!A-Za-z0-9._-]*)
			printf 'COPILOT_IDE_EXTENSION_VERSION invalide\n' >&2
			exit 2
			;;
	esac
	case "$ANTIGRAVITY_IDE_VERSION" in
		*[!A-Za-z0-9._-]*)
			printf 'ANTIGRAVITY_IDE_VERSION invalide\n' >&2
			exit 2
			;;
	esac
	if ! sh "$repository_root/scripts/compat-antigravity-ide-app-proof.sh" \
		"$ANTIGRAVITY_IDE_APP" "$ANTIGRAVITY_IDE_VERSION" "$compat_hermetic" >/dev/null; then
		printf 'Application Antigravity IDE invalide\n' >&2
		exit 2
	fi
fi
while IFS= read -r fixture_path; do
	fixture_relative_path=${fixture_path#testdata/runtime/}
	if [ -n "$fixture_path" ] && [ ! -f "$fixture_root/$fixture_relative_path" ]; then
		printf 'Fixture runtime absente : %s\n' "$fixture_path" >&2
		exit 2
	fi
done <"$fixture_manifest"

runtime_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-compat.XXXXXX") || {
	printf 'Création du dossier runtime temporaire échouée\n' >&2
	exit 2
}
failures=0
runtime_results_file="$runtime_directory/runtime-results.tsv"
strict_contracts_manifest="$repository_root/testdata/runtime/expected-strict-contracts.tsv"
if ! : >"$runtime_results_file"; then
	printf 'Initialisation des résultats runtime échouée\n' >&2
	exit 2
fi

cleanup_runtime_directory() {
	case "$runtime_directory" in
		/tmp/sth-compat.*|/private/tmp/sth-compat.*|/var/folders/*/sth-compat.*|/private/var/folders/*/sth-compat.*)
			rm -rf -- "$runtime_directory"
			;;
	esac
}

trap cleanup_runtime_directory EXIT HUP INT TERM

validate_runtime_binary() {
	configured_binary=$1
	validated_runtime_binary=$configured_binary
	if [ -z "$configured_binary" ] || [ "$compat_hermetic" = "1" ]; then
		return
	fi
	case "$configured_binary" in
		*/*)
			configured_binary_directory=$(dirname -- "$configured_binary")
			configured_binary_name=$(basename -- "$configured_binary")
			canonical_binary_directory=$(CDPATH= cd -- "$configured_binary_directory" 2>/dev/null && pwd) || {
				printf 'Résolution du binaire runtime impossible : %s\n' "$configured_binary" >&2
				exit 2
			}
			canonical_binary="$canonical_binary_directory/$configured_binary_name"
			;;
		*)
			canonical_binary=$(command -v "$configured_binary" 2>/dev/null || true)
			;;
	esac
	if [ -z "$canonical_binary" ]; then
		printf 'Binaire runtime introuvable : %s\n' "$configured_binary" >&2
		exit 2
	fi
	canonical_binary=$(python3 -c 'import os, sys; print(os.path.realpath(sys.argv[1]))' "$canonical_binary") || {
		printf 'Résolution du binaire runtime impossible : %s\n' "$configured_binary" >&2
		exit 2
	}
	if [ ! -f "$canonical_binary" ] || [ ! -x "$canonical_binary" ]; then
		printf 'Binaire runtime non exécutable : %s\n' "$canonical_binary" >&2
		exit 2
	fi
	case "$canonical_binary" in
		"$repository_root/testdata/runtime/fake-cli/"*)
			printf 'Binaire runtime hermétique interdit sans COMPAT_HERMETIC=1 : %s\n' "$canonical_binary" >&2
			exit 2
			;;
	esac
	fake_binary_match_status=0
	python3 - "$canonical_binary" "$repository_root/testdata/runtime/fake-cli" <<'PY' || fake_binary_match_status=$?
import hashlib
import os
import sys

binary_path = sys.argv[1]
fake_directory = sys.argv[2]

def sha256_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as input_stream:
        while chunk := input_stream.read(1024 * 1024):
            digest.update(chunk)
    return digest.digest()

try:
    binary_digest = sha256_file(binary_path)
    for fake_name in os.listdir(fake_directory):
        fake_path = os.path.join(fake_directory, fake_name)
        if os.path.isfile(fake_path) and sha256_file(fake_path) == binary_digest:
            raise SystemExit(0)
except OSError:
    raise SystemExit(2)

raise SystemExit(1)
PY
	case "$fake_binary_match_status" in
		0)
			printf 'Binaire runtime hermétique interdit sans COMPAT_HERMETIC=1 : %s\n' "$canonical_binary" >&2
			exit 2
			;;
		1)
			;;
		*)
			printf 'Empreinte du binaire runtime impossible : %s\n' "$canonical_binary" >&2
			exit 2
			;;
	esac
	validated_runtime_binary=$canonical_binary
}

validate_strict_runtime_binary_identity() {
	runtime_name=$1
	runtime_binary=$2
	runtime_expected_version=$3
	runtime_expected_sha256=$4
	if [ "$strict_runtime_certification" != "1" ] || [ "$compat_hermetic" = "1" ]; then
		return
	fi
	if [ -z "$runtime_binary" ]; then
		printf 'Binaire %s obligatoire en certification stricte réelle\n' "$runtime_name" >&2
		exit 2
	fi
	if ! sh "$repository_root/scripts/compat-runtime-binary-proof.sh" \
		"$runtime_binary" "$runtime_expected_version" "$runtime_expected_sha256" >/dev/null; then
		printf 'Identité du binaire %s invalide en certification stricte réelle : %s\n' "$runtime_name" "$runtime_binary" >&2
		exit 2
	fi
	runtime_verified_sha256=$(printf '%s' "$runtime_expected_sha256" | tr '[:upper:]' '[:lower:]')
	printf 'Identité runtime vérifiée: %s version=%s sha256=%s\n' \
		"$runtime_name" "$runtime_expected_version" "$runtime_verified_sha256" >&2
}

report_result() {
	status=$1
	target=$2
	artifact=$3
	evidence=$4
	printf '%s\t%s\t%s\t%s\t%s\n' "$status" "$target" "$artifact" "$evidence" "$compat_mode"
	printf '%s\t%s\t%s\n' "$status" "$target" "$artifact" >>"$runtime_results_file"
	case "$status" in
		OK)
			;;
		KO)
			failures=$((failures + 1))
			;;
		NON_EXECUTE)
			if [ "$strict_runtime_certification" = "1" ]; then
				failures=$((failures + 1))
			fi
			;;
		*)
			failures=$((failures + 1))
			;;
	esac
}

claude_authentication_available() {
	claude_auth_file="$runtime_directory/claude-auth.json"
	if "$claude_binary" auth status --json >"$claude_auth_file" 2>&1; then
		return 0
	fi
	if [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -n "${ANTHROPIC_AUTH_TOKEN:-}" ] || [ -n "${CLAUDE_CODE_OAUTH_TOKEN:-}" ]; then
		return 0
	fi
	if [ "${CLAUDE_CODE_USE_BEDROCK:-0}" = "1" ] || [ "${CLAUDE_CODE_USE_VERTEX:-0}" = "1" ] || [ "${CLAUDE_CODE_USE_FOUNDRY:-0}" = "1" ] || [ "${CLAUDE_CODE_USE_MANTLE:-0}" = "1" ] || [ "${CLAUDE_CODE_USE_ANTHROPIC_AWS:-0}" = "1" ]; then
		return 0
	fi
	return 1
}

claude_authentication_failed() {
	grep -Eiq 'not logged in|please run /login|authentication[_ ]error|authentication required|invalid (api )?key|oauth token.*(expired|invalid)|unauthorized' "$1"
}

run_claude_discovery() {
	discovery_output_file="$runtime_directory/claude-discovery.jsonl"
	system_init_file="$runtime_directory/claude-system-init.json"
	instructions_loaded_file="$runtime_directory/claude-instructions-loaded.jsonl"
	discovery_status=0
	(
		cd "$runtime_directory/claude-project" &&
		STH_CLAUDE_INSTRUCTIONS_FILE="$instructions_loaded_file" \
			"$claude_binary" -p "/help" \
			--output-format stream-json \
			--include-hook-events \
			--verbose \
			--permission-mode dontAsk \
			--tools "" \
			--strict-mcp-config \
			--mcp-config '{"mcpServers":{}}' \
			--setting-sources project \
			--no-session-persistence \
			--settings '{"hooks":{"InstructionsLoaded":[{"matcher":"session_start|include","hooks":[{"type":"command","command":"tee -a \"$STH_CLAUDE_INSTRUCTIONS_FILE\""}]}]}}'
	) >"$discovery_output_file" 2>&1 || discovery_status=$?

	awk '/"type"[[:space:]]*:[[:space:]]*"system"/ && /"subtype"[[:space:]]*:[[:space:]]*"init"/ { print; exit }' "$discovery_output_file" >"$system_init_file"
	if [ "$discovery_status" -ne 0 ]; then
		for discovery in skill-discovery agent-discovery command-discovery instruction-discovery; do
			report_result "KO" "claude" "$discovery" "initialisation locale en échec"
		done
		return
	fi
	if ! awk '/"type"[[:space:]]*:[[:space:]]*"result"/ && /"duration_api_ms"[[:space:]]*:[[:space:]]*0/ && /"num_turns"[[:space:]]*:[[:space:]]*0/ && /"total_cost_usd"[[:space:]]*:[[:space:]]*0([,}])/ { found = 1 } END { exit found ? 0 : 1 }' "$discovery_output_file"; then
		for discovery in skill-discovery agent-discovery command-discovery instruction-discovery; do
			report_result "KO" "claude" "$discovery" "absence d'appel modèle non prouvée"
		done
		return
	fi

	if grep -Fq '"sth-runtime-skill"' "$system_init_file"; then
		report_result "OK" "claude" "skill-discovery" "system/init: skill chargé sans modèle"
	else
		report_result "KO" "claude" "skill-discovery" "skill absent de system/init"
	fi
	if grep -Fq '"sth-runtime-agent"' "$system_init_file"; then
		report_result "OK" "claude" "agent-discovery" "system/init: agent chargé sans modèle"
	else
		report_result "KO" "claude" "agent-discovery" "agent absent de system/init"
	fi
	if grep -Fq '"sth-runtime-command"' "$system_init_file"; then
		report_result "OK" "claude" "command-discovery" "system/init: commande chargée sans modèle"
	else
		report_result "KO" "claude" "command-discovery" "commande absente de system/init"
	fi
	if [ -f "$instructions_loaded_file" ] && \
		grep -Fq '"hook_event_name":"InstructionsLoaded"' "$instructions_loaded_file" && \
		grep -Fq '.claude/rules/sth-runtime-rule.md' "$instructions_loaded_file" && \
		grep -Fq '"load_reason":"session_start"' "$instructions_loaded_file" && \
		grep -Fq '"memory_type":"Project"' "$instructions_loaded_file"; then
		report_result "OK" "claude" "instruction-discovery" "InstructionsLoaded: règle projet chargée sans modèle"
	else
		report_result "KO" "claude" "instruction-discovery" "événement InstructionsLoaded absent"
	fi
}

copilot_authentication_failed() {
	grep -Eiq 'not logged in|not authenticated|no authentication information|authentication required|please (run|use).*login|invalid.*token|token.*(expired|invalid)|unauthorized' "$1"
}

copilot_discovery_unavailable() {
	grep -Eiq 'plugins command is not available|command is not available|unknown command' "$1"
}

codex_authentication_failed() {
	grep -Eiq 'not logged in|not authenticated|authentication required|missing bearer authentication|please.*login|invalid.*(api )?key|token.*(expired|invalid)|unauthorized' "$1"
}

codex_environment_unavailable() {
	grep -Eiq 'operation not permitted|read-only file system' "$1"
}

gemini_authentication_failed() {
	grep -Eiq 'no authentication|authentication (is )?required|please set an auth method|please.*(authenticate|sign in|login)|unable to authenticate|invalid.*(api )?key|api key not valid|credentials?.*(missing|not found)|unauthenticated' "$1"
}

antigravity_authentication_failed() {
	grep -Eiq 'not logged in|not logged into|authentication (is )?required|please.*(authenticate|sign in|log in|login)|no saved session|credentials?.*(missing|not found)|failed to get oauth token|error getting token source|state syncing error: key not found|oauth.*(required|expired|invalid)|unauthorized' "$1"
}

antigravity_environment_unavailable() {
	grep -Eiq 'operation not permitted|read-only file system|permission denied|failed to (bind|listen|create log|open log)|listen tcp.*not permitted' "$1"
}

run_claude_smoke() {
	artifact=$1
	prompt=$2
	sentinel=$3
	shift 3
	stdout_file="$runtime_directory/claude-$artifact.stdout.json"
	stderr_file="$runtime_directory/claude-$artifact.stderr.log"
	if (
		cd "$runtime_directory/claude-project" &&
		"$claude_binary" "$@" -p "$prompt" \
			--output-format json \
			--permission-mode dontAsk \
			--tools "" \
			--strict-mcp-config \
			--mcp-config '{"mcpServers":{}}' \
			--setting-sources project \
			--no-session-persistence \
			--max-budget-usd "${CLAUDE_COMPAT_MAX_BUDGET_USD:-0.10}"
	) >"$stdout_file" 2>"$stderr_file"; then
		if sh "$repository_root/scripts/compat-model-output-proof.sh" claude "$stdout_file" "$sentinel" >/dev/null; then
			report_result "OK" "claude" "$artifact" "$sentinel"
		elif claude_authentication_failed "$stdout_file" || claude_authentication_failed "$stderr_file"; then
			claude_runtime_authentication_failed=1
			report_result "NON_EXECUTE" "claude" "$artifact" "authentification absente ou invalide"
		else
			report_result "KO" "claude" "$artifact" "sentinelle absente"
		fi
	elif claude_authentication_failed "$stdout_file" || claude_authentication_failed "$stderr_file"; then
		claude_runtime_authentication_failed=1
		report_result "NON_EXECUTE" "claude" "$artifact" "authentification absente ou invalide"
	else
		report_result "KO" "claude" "$artifact" "commande en échec"
	fi
}

run_codex_smoke() {
	artifact=$1
	prompt=$2
	sentinel=$3
	maximum_attempts=1
	if [ "$artifact" = "agent" ]; then
		maximum_attempts=2
	fi
	attempt=1
	while [ "$attempt" -le "$maximum_attempts" ]; do
		stdout_file="$runtime_directory/codex-$artifact-attempt-$attempt.stdout.jsonl"
		stderr_file="$runtime_directory/codex-$artifact-attempt-$attempt.stderr.log"
		command_status=0
		(
			cd "$runtime_directory/codex-project" &&
			run_codex_cli --ask-for-approval never exec \
				--ephemeral \
				--ignore-user-config \
				--skip-git-repo-check \
				--sandbox read-only \
				--json \
				"$prompt"
		) >"$stdout_file" 2>"$stderr_file" || command_status=$?
		if [ "$command_status" -eq 0 ] && sh "$repository_root/scripts/compat-model-output-proof.sh" codex "$stdout_file" "$sentinel" >/dev/null; then
			report_result "OK" "codex" "$artifact" "$sentinel"
			return
		fi
		if codex_authentication_failed "$stdout_file" || codex_authentication_failed "$stderr_file"; then
			codex_runtime_blocked=1
			report_result "NON_EXECUTE" "codex" "$artifact" "authentification absente ou invalide"
			return
		fi
		if codex_environment_unavailable "$stdout_file" || codex_environment_unavailable "$stderr_file"; then
			codex_runtime_blocked=1
			report_result "NON_EXECUTE" "codex" "$artifact" "environnement d'exécution interdit"
			return
		fi
		if [ "$attempt" -lt "$maximum_attempts" ]; then
			attempt=$((attempt + 1))
			continue
		fi
		if [ "$command_status" -eq 0 ]; then
			report_result "KO" "codex" "$artifact" "sentinelle absente après $maximum_attempts tentative(s)"
		else
			report_result "KO" "codex" "$artifact" "commande en échec après $maximum_attempts tentative(s)"
		fi
		return
	done
}

run_codex_cli() {
	if [ -n "${CODEX_COMPAT_HOME:-}" ]; then
		CODEX_HOME="$CODEX_COMPAT_HOME" "$codex_binary" "$@"
	else
		"$codex_binary" "$@"
	fi
}

run_copilot_smoke() {
	artifact=$1
	prompt=$2
	sentinel=$3
	shift 3
	stdout_file="$runtime_directory/copilot-$artifact.stdout.txt"
	stderr_file="$runtime_directory/copilot-$artifact.stderr.log"
	if run_copilot_cli \
		-C "$runtime_directory/copilot-project" \
		--no-auto-update \
		--no-remote \
		--no-remote-export \
		--disable-builtin-mcps \
		--allow-all-tools \
		--no-ask-user \
		--max-ai-credits "${COPILOT_COMPAT_MAX_AI_CREDITS:-30}" \
		"$@" \
		-s -p "$prompt" >"$stdout_file" 2>"$stderr_file"; then
		if sh "$repository_root/scripts/compat-model-output-proof.sh" copilot "$stdout_file" "$sentinel" >/dev/null; then
			report_result "OK" "copilot-cli" "$artifact" "$sentinel"
		elif copilot_authentication_failed "$stdout_file" || copilot_authentication_failed "$stderr_file"; then
			copilot_runtime_authentication_failed=1
			report_result "NON_EXECUTE" "copilot-cli" "$artifact" "authentification absente ou invalide"
		else
			report_result "KO" "copilot-cli" "$artifact" "sentinelle absente"
		fi
	elif copilot_authentication_failed "$stdout_file" || copilot_authentication_failed "$stderr_file"; then
		copilot_runtime_authentication_failed=1
		report_result "NON_EXECUTE" "copilot-cli" "$artifact" "authentification absente ou invalide"
	else
		report_result "KO" "copilot-cli" "$artifact" "commande en échec"
	fi
}

run_copilot_cli() {
	if [ -n "${COPILOT_COMPAT_HOME:-}" ]; then
		COPILOT_HOME="$COPILOT_COMPAT_HOME" COPILOT_CACHE_HOME="$runtime_directory/copilot-cache" "$copilot_binary" "$@"
	else
		COPILOT_CACHE_HOME="$runtime_directory/copilot-cache" "$copilot_binary" "$@"
	fi
}

run_gemini_cli() {
	(
		CLOUDSDK_CONFIG="$gemini_cloudsdk_config" \
		CODE_ASSIST_API_VERSION=v1internal \
		CODE_ASSIST_ENDPOINT=https://cloudcode-pa.googleapis.com \
		GEMINI_CLI_ACTIVITY_LOG_TARGET= \
		GEMINI_CLI_CUSTOM_HEADERS= \
		GEMINI_CLI_HOME="$gemini_cli_home" \
		GEMINI_CLI_NO_RELAUNCH=true \
		GEMINI_CLI_SYSTEM_DEFAULTS_PATH="$gemini_system_defaults_file" \
		GEMINI_CLI_SYSTEM_SETTINGS_PATH="$gemini_system_settings_file" \
		GEMINI_CLI_TRUST_WORKSPACE=true \
		GEMINI_SANDBOX=false \
		GEMINI_TELEMETRY_ENABLED=false \
		GEMINI_TELEMETRY_LOG_PROMPTS=false \
		GEMINI_TELEMETRY_TRACES_ENABLED=false \
		GOOGLE_APPLICATION_CREDENTIALS="$gemini_application_credentials" \
		GOOGLE_GEMINI_BASE_URL= \
		GOOGLE_VERTEX_BASE_URL= \
		SANDBOX= \
			"$gemini_binary" "$@"
	)
}

run_gemini_smoke() {
	artifact=$1
	prompt=$2
	sentinel=$3
	stdout_file="$runtime_directory/gemini-$artifact.stdout.json"
	stderr_file="$runtime_directory/gemini-$artifact.stderr.log"
	if (
		cd "$runtime_directory/gemini-project" &&
		run_gemini_cli \
			--extensions none \
			--allowed-mcp-server-names "$gemini_disabled_mcp_server_name" \
			--approval-mode plan \
			--admin-policy "$gemini_admin_policy_file" \
			--output-format json \
			-p "$prompt"
	) >"$stdout_file" 2>"$stderr_file"; then
		if sh "$repository_root/scripts/compat-model-output-proof.sh" gemini "$stdout_file" "$sentinel" >/dev/null; then
			report_result "OK" "gemini" "$artifact" "$sentinel"
		elif gemini_authentication_failed "$stdout_file" || gemini_authentication_failed "$stderr_file"; then
			gemini_runtime_authentication_failed=1
			report_result "NON_EXECUTE" "gemini" "$artifact" "authentification absente ou invalide"
		else
			report_result "KO" "gemini" "$artifact" "sentinelle absente"
		fi
	elif gemini_authentication_failed "$stdout_file" || gemini_authentication_failed "$stderr_file"; then
		gemini_runtime_authentication_failed=1
		report_result "NON_EXECUTE" "gemini" "$artifact" "authentification absente ou invalide"
	else
		report_result "KO" "gemini" "$artifact" "commande en échec"
	fi
}

run_antigravity_smoke() {
	artifact=$1
	prompt=$2
	sentinel=$3
	shift 3
	stdout_file="$runtime_directory/antigravity-$artifact.stdout.json"
	stderr_file="$runtime_directory/antigravity-$artifact.stderr.log"
	cli_log_file="$runtime_directory/antigravity-$artifact.cli.log"
	if (
		cd "$runtime_directory/antigravity-project" &&
		"$antigravity_binary" \
			--add-dir "$runtime_directory/antigravity-project" \
			--log-file "$cli_log_file" \
			--output-format json \
			"$@" \
			-p "$prompt"
	) >"$stdout_file" 2>"$stderr_file"; then
		if sh "$repository_root/scripts/compat-model-output-proof.sh" antigravity "$stdout_file" "$sentinel" >/dev/null; then
			report_result "OK" "antigravity-cli" "$artifact" "$sentinel"
		elif antigravity_authentication_failed "$stdout_file" || antigravity_authentication_failed "$stderr_file" || { [ -f "$cli_log_file" ] && antigravity_authentication_failed "$cli_log_file"; }; then
			antigravity_runtime_blocked=1
			report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "authentification absente ou invalide"
		elif antigravity_environment_unavailable "$stdout_file" || antigravity_environment_unavailable "$stderr_file" || { [ -f "$cli_log_file" ] && antigravity_environment_unavailable "$cli_log_file"; }; then
			antigravity_runtime_blocked=1
			report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "environnement d'exécution interdit"
		else
			report_result "KO" "antigravity-cli" "$artifact" "sentinelle absente"
		fi
	elif antigravity_authentication_failed "$stdout_file" || antigravity_authentication_failed "$stderr_file" || { [ -f "$cli_log_file" ] && antigravity_authentication_failed "$cli_log_file"; }; then
		antigravity_runtime_blocked=1
		report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "authentification absente ou invalide"
	elif antigravity_environment_unavailable "$stdout_file" || antigravity_environment_unavailable "$stderr_file" || { [ -f "$cli_log_file" ] && antigravity_environment_unavailable "$cli_log_file"; }; then
		antigravity_runtime_blocked=1
		report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "environnement d'exécution interdit"
	else
		report_result "KO" "antigravity-cli" "$artifact" "commande en échec"
	fi
}

printf 'status\ttarget\tartifact\tevidence\tmode\n'

for fixture_project in claude codex copilot gemini antigravity cursor; do
	if ! cp -R "$fixture_root/$fixture_project-project" "$runtime_directory/$fixture_project-project"; then
		printf 'Copie de la fixture %s échouée\n' "$fixture_project" >&2
		exit 2
	fi
done
if ! mkdir -p "$runtime_directory/codex-home" "$runtime_directory/copilot-home" "$runtime_directory/copilot-cache" "$runtime_directory/gemini-home" "$runtime_directory/gemini-cloudsdk"; then
	printf 'Création des dossiers runtime isolés échouée\n' >&2
	exit 2
fi

gemini_cli_home="$runtime_directory/gemini-home"
gemini_cloudsdk_config="$runtime_directory/gemini-cloudsdk"
gemini_system_defaults_file="$runtime_directory/gemini-system-defaults.json"
gemini_system_settings_file="$runtime_directory/gemini-system-settings.json"
gemini_admin_policy_file="$runtime_directory/gemini-admin-policy.toml"
if ! python3 - "$runtime_directory/gemini-project" "$gemini_cli_home" <<'PY'
import os
import sys

project_directory = os.path.realpath(sys.argv[1])
gemini_home = os.path.realpath(sys.argv[2])
current_directory = project_directory
while True:
    if os.path.exists(os.path.join(current_directory, ".gemini", ".env")):
        raise SystemExit("fichier .gemini/.env trouvé dans la chaîne de parents")
    parent_directory = os.path.dirname(current_directory)
    if parent_directory == current_directory:
        break
    current_directory = parent_directory
for relative_path in ((".gemini", ".env"), (".env",)):
    if os.path.exists(os.path.join(gemini_home, *relative_path)):
        raise SystemExit("fichier dotenv trouvé dans GEMINI_CLI_HOME")
PY
then
	printf 'Fichier .gemini/.env interdit dans l’environnement Gemini contrôlé\n' >&2
	exit 2
fi
if [ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]; then
	gemini_application_credentials=$(python3 - "$GOOGLE_APPLICATION_CREDENTIALS" <<'PY'
import os
import sys

credential_path = os.path.realpath(sys.argv[1])
if not os.path.isfile(credential_path):
    raise SystemExit("fichier GOOGLE_APPLICATION_CREDENTIALS absent")
print(credential_path)
PY
	) || {
		printf 'GOOGLE_APPLICATION_CREDENTIALS explicite invalide\n' >&2
		exit 2
	}
else
	gemini_application_credentials="$runtime_directory/gemini-no-ambient-adc.json"
fi
gemini_disabled_mcp_server_name=$(python3 -c 'import secrets; print("sth-runtime-disabled-" + secrets.token_hex(16))') || {
	printf 'Génération de la garde MCP Gemini échouée\n' >&2
	exit 2
}
if [ "$compat_hermetic" = "1" ] && [ -n "${GEMINI_COMPAT_SYSTEM_POLICIES_DIR:-}" ]; then
	gemini_system_policies_directory=$GEMINI_COMPAT_SYSTEM_POLICIES_DIR
else
	case "$(uname -s)" in
		Darwin)
			gemini_system_policies_directory='/Library/Application Support/GeminiCli/policies'
			;;
		MINGW*|MSYS*)
			gemini_system_policies_directory='C:\ProgramData\gemini-cli\policies'
			;;
		CYGWIN*)
			if command -v cygpath >/dev/null 2>&1; then
				gemini_system_policies_directory=$(cygpath -u 'C:\ProgramData\gemini-cli\policies') || {
					printf 'Résolution du dossier de policies Gemini Windows échouée\n' >&2
					exit 2
				}
			else
				gemini_system_policies_directory='/cygdrive/c/ProgramData/gemini-cli/policies'
			fi
			;;
		*)
			gemini_system_policies_directory=/etc/gemini-cli/policies
			;;
	esac
fi
gemini_system_policy_present=$(python3 - "$gemini_system_policies_directory" <<'PY'
import os
import sys

policy_directory = sys.argv[1]
try:
    policy_entries = os.listdir(policy_directory)
except FileNotFoundError:
    policy_entries = []
except OSError as err:
    raise SystemExit(f"lecture des policies système Gemini impossible: {err}")
print("1" if any(entry.endswith(".toml") for entry in policy_entries) else "0")
PY
) || {
	printf 'Inspection des policies système Gemini échouée\n' >&2
	exit 2
}
if ! mkdir -p "$gemini_cli_home"; then
	printf 'Création du dossier Gemini isolé échouée\n' >&2
	exit 2
fi
if ! printf '%s\n' '{}' >"$gemini_system_defaults_file"; then
	printf 'Initialisation des valeurs par défaut Gemini échouée\n' >&2
	exit 2
fi
if ! printf '%s\n' \
	'{' \
	'  "advanced": {"ignoreLocalEnv": true},' \
	'  "hooksConfig": {"enabled": false},' \
	'  "privacy": {"usageStatisticsEnabled": false},' \
	'  "telemetry": {"enabled": false, "logPrompts": false},' \
	'  "security": {"disableYoloMode": true, "disableAlwaysAllow": true}' \
	'}' >"$gemini_system_settings_file"; then
	printf 'Initialisation des réglages Gemini sécurisés échouée\n' >&2
	exit 2
fi
if ! printf '%s\n' \
	'[[rule]]' \
	'toolName = "*"' \
	'decision = "deny"' \
	'priority = 900' \
	'modes = ["plan"]' \
	'interactive = false' \
	'' \
	'[[rule]]' \
	'toolName = ["activate_skill", "sth-runtime-agent"]' \
	'decision = "allow"' \
	'priority = 999' \
	'modes = ["plan"]' \
	'interactive = false' >"$gemini_admin_policy_file"; then
	printf 'Initialisation de la politique Gemini sécurisée échouée\n' >&2
	exit 2
fi

if [ "${CLAUDE_BIN+x}" = "x" ]; then
	claude_binary=$CLAUDE_BIN
else
	claude_binary=$(command -v claude 2>/dev/null || true)
fi
if [ "${CODEX_BIN+x}" = "x" ]; then
	codex_binary=$CODEX_BIN
else
	codex_binary=$(command -v codex 2>/dev/null || true)
fi
if [ "${COPILOT_BIN+x}" = "x" ]; then
	copilot_binary=$COPILOT_BIN
else
	copilot_binary=$(command -v copilot 2>/dev/null || true)
fi
if [ "${GEMINI_BIN+x}" = "x" ]; then
	gemini_binary=$GEMINI_BIN
else
	gemini_binary=$(command -v gemini 2>/dev/null || true)
fi
if [ "${ANTIGRAVITY_BIN+x}" = "x" ]; then
	antigravity_binary=$ANTIGRAVITY_BIN
else
	antigravity_binary=$(command -v agy 2>/dev/null || command -v antigravity 2>/dev/null || true)
fi
validate_runtime_binary "$claude_binary"
claude_binary=$validated_runtime_binary
validate_runtime_binary "$codex_binary"
codex_binary=$validated_runtime_binary
validate_runtime_binary "$copilot_binary"
copilot_binary=$validated_runtime_binary
validate_runtime_binary "$gemini_binary"
gemini_binary=$validated_runtime_binary
validate_runtime_binary "$antigravity_binary"
antigravity_binary=$validated_runtime_binary
validate_strict_runtime_binary_identity "Claude" "$claude_binary" "$claude_expected_version" "$claude_expected_sha256"
validate_strict_runtime_binary_identity "Codex" "$codex_binary" "$codex_expected_version" "$codex_expected_sha256"
validate_strict_runtime_binary_identity "Copilot CLI" "$copilot_binary" "$copilot_expected_version" "$copilot_expected_sha256"
GEMINI_CLI_HOME="$gemini_cli_home" \
CLOUDSDK_CONFIG="$gemini_cloudsdk_config" \
	validate_strict_runtime_binary_identity "Gemini" "$gemini_binary" "$gemini_expected_version" "$gemini_expected_sha256"
validate_strict_runtime_binary_identity "Antigravity CLI" "$antigravity_binary" "$antigravity_expected_version" "$antigravity_expected_sha256"
if [ -n "$claude_binary" ]; then
	run_claude_discovery
	if [ "$run_model_smokes" = "1" ]; then
		if claude_authentication_available; then
			claude_runtime_authentication_failed=0
			run_claude_smoke "skill" "/sth-runtime-skill" "CLAUDE_SKILL_$sentinel_suffix"
			if [ "$claude_runtime_authentication_failed" = "0" ]; then
				run_claude_smoke "agent" "Return the agent sentinel." "CLAUDE_AGENT_$sentinel_suffix" --agent sth-runtime-agent
			else
				report_result "NON_EXECUTE" "claude" "agent" "authentification absente ou invalide"
			fi
			if [ "$claude_runtime_authentication_failed" = "0" ]; then
				run_claude_smoke "command" "/sth-runtime-command" "CLAUDE_COMMAND_$sentinel_suffix"
			else
				report_result "NON_EXECUTE" "claude" "command" "authentification absente ou invalide"
			fi
			if [ "$claude_runtime_authentication_failed" = "0" ]; then
				run_claude_smoke "instruction" "Return the STH instruction sentinel." "CLAUDE_INSTRUCTION_$sentinel_suffix"
			else
				report_result "NON_EXECUTE" "claude" "instruction" "authentification absente ou invalide"
			fi
		else
			for artifact in skill agent command instruction; do
				report_result "NON_EXECUTE" "claude" "$artifact" "authentification absente"
			done
		fi
	else
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "claude" "$artifact" "RUN_MODEL_SMOKES=0"
		done
	fi
else
	for artifact in skill agent command instruction; do
		report_result "NON_EXECUTE" "claude" "$artifact" "binaire absent"
	done
fi

if [ -n "$codex_binary" ]; then
	prompt_input_file="$runtime_directory/codex-prompt-input.json"
	codex_discovery_home=${CODEX_COMPAT_HOME:-$runtime_directory/codex-home}
	if (cd "$runtime_directory/codex-project" && CODEX_HOME="$codex_discovery_home" "$codex_binary" debug prompt-input "Inspect STH runtime customizations.") >"$prompt_input_file" 2>&1 && grep -Fq "sth-runtime-skill" "$prompt_input_file"; then
		report_result "OK" "codex" "skill-discovery" ".agents/skills chargé"
	else
		report_result "KO" "codex" "skill-discovery" "skill absent du prompt"
	fi
	if grep -Fq "CODEX_INSTRUCTION_$sentinel_suffix" "$prompt_input_file"; then
		report_result "OK" "codex" "instruction-discovery" "AGENTS.md chargé"
	else
		report_result "KO" "codex" "instruction-discovery" "AGENTS.md absent du prompt"
	fi
	if grep -Fq "sth-runtime-command" "$prompt_input_file"; then
		report_result "OK" "codex" "command-discovery" "commande adaptée en skill chargée"
	else
		report_result "KO" "codex" "command-discovery" "skill de commande absent du prompt"
	fi
	if [ "$run_model_smokes" = "1" ]; then
		codex_runtime_blocked=0
		run_codex_smoke "skill" '$sth-runtime-skill' "CODEX_SKILL_$sentinel_suffix"
		if [ "$codex_runtime_blocked" = "0" ]; then
			run_codex_smoke "agent" "Delegate this request to the sth-runtime-agent custom agent and return its response." "CODEX_AGENT_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "codex" "agent" "smoke précédent non exécutable"
		fi
		if [ "$codex_runtime_blocked" = "0" ]; then
			run_codex_smoke "command" '$sth-runtime-command' "CODEX_COMMAND_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "codex" "command" "smoke précédent non exécutable"
		fi
		if [ "$codex_runtime_blocked" = "0" ]; then
			run_codex_smoke "instruction" "Return the STH instruction sentinel." "CODEX_INSTRUCTION_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "codex" "instruction" "smoke précédent non exécutable"
		fi
	else
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "codex" "${artifact}-runtime" "RUN_MODEL_SMOKES=0"
		done
	fi
else
	for artifact in skill agent command instruction; do
		report_result "NON_EXECUTE" "codex" "$artifact" "binaire absent"
	done
fi

copilot_ide_prompt="$runtime_directory/copilot-project/.github/prompts/sth-runtime-command.prompt.md"
if [ -n "${COPILOT_IDE_TRANSCRIPT:-}" ]; then
	copilot_ide_proof_status=0
	copilot_ide_proof=$(sh "$repository_root/scripts/compat-copilot-ide-proof.sh" \
		"$COPILOT_IDE_TRANSCRIPT" \
		"$fixture_root/copilot-project" \
		"COPILOT_COMMAND_$sentinel_suffix" \
		"${COPILOT_IDE_EXTENSION_VERSION:-}" \
		"$runtime_challenge") || copilot_ide_proof_status=$?
	case "$copilot_ide_proof" in
		DISCOVERY_OK_RUNTIME_OK)
			report_result "OK" "copilot-ide" "command-discovery" ".github/prompts résolu par GitHub Copilot"
			report_result "OK" "copilot-ide" "command" "COPILOT_COMMAND_$sentinel_suffix"
			;;
		DISCOVERY_OK_RUNTIME_NON_EXECUTE)
			report_result "OK" "copilot-ide" "command-discovery" ".github/prompts résolu par GitHub Copilot"
			report_result "NON_EXECUTE" "copilot-ide" "command" "modèle indisponible ou authentification absente"
			;;
		DISCOVERY_OK_RUNTIME_KO)
			report_result "OK" "copilot-ide" "command-discovery" ".github/prompts résolu par GitHub Copilot"
			report_result "KO" "copilot-ide" "command" "sentinelle absente du transcript"
			;;
		*)
			report_result "KO" "copilot-ide" "command-discovery" "transcript IDE invalide"
			report_result "NON_EXECUTE" "copilot-ide" "command" "découverte IDE non prouvée"
			;;
	esac
elif [ -f "$copilot_ide_prompt" ] && grep -Fq "COPILOT_COMMAND_$sentinel_suffix" "$copilot_ide_prompt"; then
	report_result "NON_EXECUTE" "copilot-ide" "command-fixture" ".github/prompts valide, transcript IDE absent"
else
	report_result "KO" "copilot-ide" "command-fixture" ".github/prompts absent ou invalide"
fi

if [ -n "$copilot_binary" ]; then
	copilot_skills_file="$runtime_directory/copilot-skills.json"
	if run_copilot_cli -C "$runtime_directory/copilot-project" skill list --json >"$copilot_skills_file" 2>&1; then
		if grep -Fq 'sth-runtime-skill' "$copilot_skills_file"; then
			report_result "OK" "copilot-cli" "skill-discovery" ".agents/skills chargé"
		else
			report_result "KO" "copilot-cli" "skill-discovery" "skill absent"
		fi
	elif copilot_authentication_failed "$copilot_skills_file"; then
		report_result "NON_EXECUTE" "copilot-cli" "skill-discovery" "authentification absente ou invalide"
	else
		report_result "KO" "copilot-cli" "skill-discovery" "inspection des skills en échec"
	fi
	if [ "$compat_hermetic" = "1" ] && grep -Fq "COPILOT_INSTRUCTION_$sentinel_suffix" "$runtime_directory/copilot-project/AGENTS.md"; then
		report_result "OK" "copilot-cli" "instruction-discovery" "contrat AGENTS.md hermétique chargé"
	else
		report_result "NON_EXECUTE" "copilot-cli" "instruction-discovery" "inspection locale déterministe indisponible; smoke AGENTS.md requis"
	fi
	if [ "$run_model_smokes" != "1" ]; then
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "copilot-cli" "${artifact}-runtime" "RUN_MODEL_SMOKES=0"
		done
	else
		copilot_runtime_authentication_failed=0
		run_copilot_smoke "skill" "/sth-runtime-skill" "COPILOT_SKILL_$sentinel_suffix"
		if [ "$copilot_runtime_authentication_failed" = "0" ]; then
			run_copilot_smoke "agent" "Return the agent sentinel." "COPILOT_AGENT_$sentinel_suffix" --agent sth-runtime-agent
		else
			report_result "NON_EXECUTE" "copilot-cli" "agent" "authentification absente ou invalide"
		fi
		if [ "$copilot_runtime_authentication_failed" = "0" ]; then
			run_copilot_smoke "command" "/sth-runtime-command" "COPILOT_COMMAND_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "copilot-cli" "command" "authentification absente ou invalide"
		fi
		if [ "$copilot_runtime_authentication_failed" = "0" ]; then
			run_copilot_smoke "instruction" "Use @sth-runtime-instruction-target.txt as context. Return the STH instruction sentinel." "COPILOT_INSTRUCTION_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "copilot-cli" "instruction" "authentification absente ou invalide"
		fi
	fi
else
	for artifact in skill agent command instruction; do
		report_result "NON_EXECUTE" "copilot-cli" "$artifact" "binaire absent"
	done
fi

if [ -n "$gemini_binary" ]; then
	gemini_skills_file="$runtime_directory/gemini-skills.txt"
	if (cd "$runtime_directory/gemini-project" && run_gemini_cli skills list --all) >"$gemini_skills_file" 2>&1 && grep -Fq 'sth-runtime-skill' "$gemini_skills_file"; then
		report_result "OK" "gemini" "skill-discovery" ".agents/skills chargé"
	else
		report_result "KO" "gemini" "skill-discovery" "skill absent"
	fi
	if [ "$run_model_smokes" != "1" ]; then
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "gemini" "${artifact}-runtime" "RUN_MODEL_SMOKES=0"
		done
	elif [ "$gemini_system_policy_present" = "1" ]; then
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "gemini" "$artifact" "policy système neutralise --admin-policy"
		done
	else
		gemini_runtime_authentication_failed=0
		run_gemini_smoke "skill" "Activate sth-runtime-skill and follow it." "GEMINI_SKILL_$sentinel_suffix"
		if [ "$gemini_runtime_authentication_failed" = "0" ]; then
			run_gemini_smoke "agent" "@sth-runtime-agent Return the agent sentinel." "GEMINI_AGENT_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "gemini" "agent" "authentification absente ou invalide"
		fi
		if [ "$gemini_runtime_authentication_failed" = "0" ]; then
			run_gemini_smoke "command" "/sth-runtime-command" "GEMINI_COMMAND_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "gemini" "command" "authentification absente ou invalide"
		fi
		if [ "$gemini_runtime_authentication_failed" = "0" ]; then
			run_gemini_smoke "instruction" "Return the STH instruction sentinel." "GEMINI_INSTRUCTION_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "gemini" "instruction" "authentification absente ou invalide"
		fi
	fi
else
	for artifact in skill agent command instruction; do
		report_result "NON_EXECUTE" "gemini" "$artifact" "binaire absent"
	done
fi

if [ -n "$antigravity_binary" ]; then
	antigravity_agents_file="$runtime_directory/antigravity-agents.txt"
	if (
		cd "$runtime_directory/antigravity-project" &&
		"$antigravity_binary" --add-dir "$runtime_directory/antigravity-project" agents
	) >"$antigravity_agents_file" 2>&1 && grep -Fq 'sth-runtime-agent' "$antigravity_agents_file"; then
		report_result "OK" "antigravity-cli" "agent-discovery" ".agents/agents chargé"
	elif antigravity_authentication_failed "$antigravity_agents_file"; then
		report_result "NON_EXECUTE" "antigravity-cli" "agent-discovery" "authentification absente ou invalide"
	elif antigravity_environment_unavailable "$antigravity_agents_file"; then
		report_result "NON_EXECUTE" "antigravity-cli" "agent-discovery" "environnement d'exécution interdit"
	else
		report_result "KO" "antigravity-cli" "agent-discovery" "agent absent"
	fi
	if [ "$run_model_smokes" = "1" ]; then
		antigravity_runtime_blocked=0
		run_antigravity_smoke "skill" "/sth-runtime-skill" "ANTIGRAVITY_SKILL_$sentinel_suffix"
		if [ "$antigravity_runtime_blocked" = "0" ]; then
			run_antigravity_smoke "agent" "Return the agent sentinel." "ANTIGRAVITY_AGENT_$sentinel_suffix" --agent sth-runtime-agent
		else
			report_result "NON_EXECUTE" "antigravity-cli" "agent" "smoke précédent non exécutable"
		fi
		if [ "$antigravity_runtime_blocked" = "0" ]; then
			run_antigravity_smoke "command" "/sth-runtime-command" "ANTIGRAVITY_COMMAND_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "antigravity-cli" "command" "smoke précédent non exécutable"
		fi
		if [ "$antigravity_runtime_blocked" = "0" ]; then
			run_antigravity_smoke "instruction" "Return the STH instruction sentinel." "ANTIGRAVITY_INSTRUCTION_$sentinel_suffix"
		else
			report_result "NON_EXECUTE" "antigravity-cli" "instruction" "smoke précédent non exécutable"
		fi
	else
		for artifact in skill agent command instruction; do
			report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "RUN_MODEL_SMOKES=0"
		done
	fi
else
	for artifact in skill agent command instruction; do
		report_result "NON_EXECUTE" "antigravity-cli" "$artifact" "binaire absent"
	done
fi

if [ -n "${ANTIGRAVITY_IDE_TRANSCRIPT:-}" ]; then
	antigravity_ide_proof_stdout="$runtime_directory/antigravity-ide-proof.stdout"
	antigravity_ide_proof_stderr="$runtime_directory/antigravity-ide-proof.stderr"
	if sh "$repository_root/scripts/compat-antigravity-ide-proof.sh" \
		"$ANTIGRAVITY_IDE_TRANSCRIPT" \
		"$fixture_root/antigravity-project" \
		"$sentinel_suffix" \
		"$runtime_challenge" \
		"${ANTIGRAVITY_IDE_VERSION:-}" >"$antigravity_ide_proof_stdout" 2>"$antigravity_ide_proof_stderr"; then
		report_result "OK" "antigravity-ide" "command-discovery" ".agents/workflows chargé par le transcript natif"
		report_result "OK" "antigravity-ide" "command" "ANTIGRAVITY_COMMAND_$sentinel_suffix"
		report_result "OK" "antigravity-ide" "instruction-discovery" ".agents/rules chargé par le transcript natif"
		report_result "OK" "antigravity-ide" "instruction" "ANTIGRAVITY_INSTRUCTION_$sentinel_suffix"
	else
		for artifact in command-discovery command instruction-discovery instruction; do
			report_result "KO" "antigravity-ide" "$artifact" "transcript IDE invalide"
		done
	fi
else
	for artifact in command-discovery command instruction-discovery instruction; do
		report_result "NON_EXECUTE" "antigravity-ide" "$artifact" "transcript IDE absent"
	done
fi

cursor_contracts='skill:.agents/skills/sth-runtime-skill/SKILL.md:CURSOR_SKILL_
agent:.cursor/agents/sth-runtime-agent.md:CURSOR_AGENT_
instruction:AGENTS.md:CURSOR_INSTRUCTION_
rule:.cursor/rules/sth-runtime-rule.mdc:CURSOR_RULE_'
printf '%s\n' "$cursor_contracts" | while IFS=: read -r artifact relative_path sentinel_prefix; do
	if [ "$compat_hermetic" = "1" ]; then
		if [ -f "$runtime_directory/cursor-project/$relative_path" ] && grep -Fq "${sentinel_prefix}${sentinel_suffix}" "$runtime_directory/cursor-project/$relative_path"; then
			report_result "OK" "cursor-ide" "$artifact" "contrat de fixture hermétique chargé"
		else
			report_result "KO" "cursor-ide" "$artifact" "contrat de fixture hermétique invalide"
		fi
	else
		report_result "NON_EXECUTE" "cursor-ide" "$artifact" "Cursor IDE absent ou sans transcript runtime certifié"
	fi
done

if [ "$strict_runtime_certification" = "1" ]; then
	if [ ! -f "$strict_contracts_manifest" ] || \
		! sh "$repository_root/scripts/compat-runtime-matrix-proof.sh" \
			"$strict_contracts_manifest" "$runtime_results_file" >/dev/null; then
		printf 'Matrice runtime stricte incomplète ou non certifiante\n' >&2
		failures=$((failures + 1))
	fi
fi

if [ "$failures" -gt 0 ]; then
	exit 1
fi

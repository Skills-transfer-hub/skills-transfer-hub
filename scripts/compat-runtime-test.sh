#!/bin/sh

set -u

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner="$repository_root/scripts/compat-runtime.sh"
runtime_binary_proof="$repository_root/scripts/compat-runtime-binary-proof.sh"
copilot_ide_proof="$repository_root/scripts/compat-copilot-ide-proof.sh"
copilot_ide_launch="$repository_root/scripts/compat-copilot-ide-launch.sh"
antigravity_ide_proof="$repository_root/scripts/compat-antigravity-ide-proof.sh"
antigravity_ide_app_proof="$repository_root/scripts/compat-antigravity-ide-app-proof.sh"
antigravity_ide_launch="$repository_root/scripts/compat-antigravity-ide-launch.sh"
lifecycle_runner="$repository_root/scripts/compat-sth-lifecycle-test.sh"
expected_absent_contracts="$repository_root/testdata/runtime/expected-absent-contracts.tsv"
expected_strict_contracts="$repository_root/testdata/runtime/expected-strict-contracts.tsv"
fake_claude="$repository_root/testdata/runtime/fake-cli/claude-success.sh"
fake_codex="$repository_root/testdata/runtime/fake-cli/codex-auth-required.sh"
fake_codex_agent_transient="$repository_root/testdata/runtime/fake-cli/codex-agent-transient.sh"
fake_copilot="$repository_root/testdata/runtime/fake-cli/copilot-auth-required.sh"
fake_copilot_success="$repository_root/testdata/runtime/fake-cli/copilot-success.sh"
fake_gemini="$repository_root/testdata/runtime/fake-cli/gemini-auth-required.sh"
fake_gemini_success="$repository_root/testdata/runtime/fake-cli/gemini-success.sh"
fake_antigravity_success="$repository_root/testdata/runtime/fake-cli/antigravity-success.sh"
fake_antigravity_auth="$repository_root/testdata/runtime/fake-cli/antigravity-auth-required.sh"
test_directory=$(mktemp -d "${TMPDIR:-/tmp}/sth-compat-test.XXXXXX")
invalid_output="$test_directory/invalid.out"
invalid_error="$test_directory/invalid.err"

unset COMPAT_FIXTURE_ROOT COMPAT_STH_BIN STH_RUNTIME_SENTINEL_SUFFIX COMPAT_HERMETIC GEMINI_COMPAT_SYSTEM_POLICIES_DIR GOOGLE_APPLICATION_CREDENTIALS
unset CLAUDE_COMPAT_EXPECTED_VERSION CLAUDE_COMPAT_EXPECTED_SHA256 CODEX_COMPAT_EXPECTED_VERSION CODEX_COMPAT_EXPECTED_SHA256
unset COPILOT_COMPAT_EXPECTED_VERSION COPILOT_COMPAT_EXPECTED_SHA256 GEMINI_COMPAT_EXPECTED_VERSION GEMINI_COMPAT_EXPECTED_SHA256
unset ANTIGRAVITY_COMPAT_EXPECTED_VERSION ANTIGRAVITY_COMPAT_EXPECTED_SHA256
COPILOT_IDE_TRANSCRIPT=
ANTIGRAVITY_IDE_TRANSCRIPT=
export COPILOT_IDE_TRANSCRIPT ANTIGRAVITY_IDE_TRANSCRIPT

cleanup_test_directory() {
	case "$test_directory" in
		/tmp/sth-compat-test.*|/private/tmp/sth-compat-test.*|/var/folders/*/sth-compat-test.*)
			rm -rf -- "$test_directory"
			;;
	esac
}

trap cleanup_test_directory EXIT HUP INT TERM

fail_test() {
	printf 'KO: %s\n' "$1" >&2
	exit 1
}

run_without_consumers() {
	CLAUDE_BIN= \
	CODEX_BIN= \
	COPILOT_BIN= \
	GEMINI_BIN= \
	ANTIGRAVITY_BIN= \
	COPILOT_IDE_TRANSCRIPT= \
	ANTIGRAVITY_IDE_TRANSCRIPT= \
	STRICT_RUNTIME_CERTIFICATION=$1 \
	RUN_MODEL_SMOKES=$2 \
		sh "$runner"
}

assert_status() {
	expected_status=$1
	actual_status=$2
	test_name=$3
	if [ "$actual_status" -ne "$expected_status" ]; then
		fail_test "$test_name: statut $actual_status, attendu $expected_status"
	fi
}

copilot_transcript_unavailable="$test_directory/copilot-transcript-unavailable.jsonl"
copilot_transcript_success="$test_directory/copilot-transcript-success.jsonl"
copilot_prompt_path="$repository_root/testdata/runtime/copilot-project/.github/prompts/sth-runtime-command.prompt.md"
printf '{"kind":0,"v":{"requests":[{"agent":{"extensionId":{"value":"GitHub.copilot-chat"},"extensionVersion":"test"},"message":{"parts":[{"kind":"prompt","name":"sth-runtime-command"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"result":{"errorDetails":{"message":"Language model unavailable"}}}]}}\n' "$copilot_prompt_path" >"$copilot_transcript_unavailable"
printf '{"kind":0,"v":{"requests":[{"agent":{"extensionId":{"value":"GitHub.copilot-chat"},"extensionVersion":"test"},"message":{"parts":[{"kind":"prompt","name":"sth-runtime-command"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"result":{"response":"COPILOT_COMMAND_OK"}}]}}\n' "$copilot_prompt_path" >"$copilot_transcript_success"

copilot_unavailable_result=$(sh "$copilot_ide_proof" "$copilot_transcript_unavailable" "$repository_root/testdata/runtime/copilot-project")
copilot_unavailable_status=$?
assert_status 3 "$copilot_unavailable_status" "preuve Copilot IDE sans modèle"
if [ "$copilot_unavailable_result" != "DISCOVERY_OK_RUNTIME_NON_EXECUTE" ]; then
	fail_test "preuve Copilot IDE: découverte sans modèle inattendue"
fi

copilot_success_result=$(sh "$copilot_ide_proof" "$copilot_transcript_success" "$repository_root/testdata/runtime/copilot-project")
copilot_success_status=$?
assert_status 0 "$copilot_success_status" "preuve Copilot IDE avec sentinelle"
if [ "$copilot_success_result" != "DISCOVERY_OK_RUNTIME_OK" ]; then
	fail_test "preuve Copilot IDE: sentinelle non reconnue"
fi
sh "$copilot_ide_proof" "$copilot_transcript_success" "$repository_root/testdata/runtime/copilot-project" COPILOT_COMMAND_OK test AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA >"$invalid_output" 2>"$invalid_error"
copilot_challenge_status=$?
assert_status 1 "$copilot_challenge_status" "preuve Copilot IDE sans challenge frais"

copilot_incremental_success="$test_directory/copilot-transcript-incremental.jsonl"
printf '%s\n' \
	'{"kind":0,"v":{"requests":[]}}' \
	'{"kind":1,"k":["responderUsername"],"v":"GitHub Copilot"}' \
	"$(printf '{"kind":2,"k":["requests"],"v":[{"agent":{"extensionId":{"value":"GitHub.copilot-chat"},"extensionVersion":"test"},"message":{"text":"/sth-runtime-command","parts":[{"kind":"prompt","name":"sth-runtime-command"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"response":[{"kind":"thinking","value":[]}]}]}' "$copilot_prompt_path")" \
	'{"kind":2,"k":["requests",0,"response"],"v":[{"value":"COPILOT_COMMAND_OK"}]}' \
	>"$copilot_incremental_success"
copilot_incremental_result=$(sh "$copilot_ide_proof" "$copilot_incremental_success" "$repository_root/testdata/runtime/copilot-project")
copilot_incremental_status=$?
assert_status 0 "$copilot_incremental_status" "preuve Copilot IDE avec journal incrémental"
if [ "$copilot_incremental_result" != "DISCOVERY_OK_RUNTIME_OK" ]; then
	fail_test "preuve Copilot IDE incrémentale: sentinelle non reconnue"
fi

sh "$copilot_ide_proof" "$copilot_incremental_success" "$repository_root/testdata/runtime/copilot-project" COPILOT_COMMAND_OK autre >"$invalid_output" 2>"$invalid_error"
copilot_incremental_version_status=$?
assert_status 1 "$copilot_incremental_version_status" "preuve Copilot IDE incrémentale avec version d'extension différente"

copilot_incremental_orphan="$test_directory/copilot-transcript-orphan.jsonl"
printf '%s\n' \
	"$(printf '{"kind":2,"k":["requests"],"v":[{"agent":{"extensionId":{"value":"GitHub.copilot-chat"},"extensionVersion":"test"},"message":{"parts":[{"kind":"prompt","name":"sth-runtime-command"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"response":[{"value":"COPILOT_COMMAND_OK"}]}]}' "$copilot_prompt_path")" \
	>"$copilot_incremental_orphan"
sh "$copilot_ide_proof" "$copilot_incremental_orphan" "$repository_root/testdata/runtime/copilot-project" >"$invalid_output" 2>"$invalid_error"
copilot_incremental_orphan_status=$?
assert_status 1 "$copilot_incremental_orphan_status" "preuve Copilot IDE incrémentale sans instantané initial"

copilot_incremental_null_append="$test_directory/copilot-transcript-null-append.jsonl"
printf '%s\n' \
	'{"kind":0,"v":{"requests":null}}' \
	'{"kind":2,"k":["requests"],"v":[]}' \
	>"$copilot_incremental_null_append"
sh "$copilot_ide_proof" "$copilot_incremental_null_append" "$repository_root/testdata/runtime/copilot-project" >"$invalid_output" 2>"$invalid_error"
copilot_incremental_null_append_status=$?
assert_status 1 "$copilot_incremental_null_append_status" "preuve Copilot IDE incrémentale avec ajout sur null"
if ! grep -Fq 'ajout sur une valeur non séquentielle' "$invalid_error"; then
	fail_test "preuve Copilot IDE incrémentale avec ajout sur null: diagnostic absent"
fi

copilot_incremental_agent_host="$test_directory/copilot-transcript-agent-host.jsonl"
printf '%s\n' \
	'{"kind":0,"v":{"requests":[]}}' \
	"$(printf '{"kind":2,"k":["requests"],"v":[{"agent":{"extensionId":{"value":"GitHub.copilot"},"extensionVersion":"test"},"message":{"parts":[{"kind":"prompt","name":"sth-runtime-command"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"response":[{"value":"COPILOT_COMMAND_OK"}]}]}' "$copilot_prompt_path")" \
	>"$copilot_incremental_agent_host"
sh "$copilot_ide_proof" "$copilot_incremental_agent_host" "$repository_root/testdata/runtime/copilot-project" >"$invalid_output" 2>"$invalid_error"
copilot_incremental_agent_host_status=$?
assert_status 1 "$copilot_incremental_agent_host_status" "preuve Copilot IDE incrémentale hors GitHub.copilot-chat"

antigravity_transcript_directory="$test_directory/antigravity-transcript"
mkdir -p "$antigravity_transcript_directory"
antigravity_transcript_success="$antigravity_transcript_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"/sth-runtime-command","tool_calls":[]}' \
	'{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_OK","tool_calls":[]}' \
	'{"step_index":3,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"Return the STH instruction sentinel.","tool_calls":[]}' \
	'{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_OK","tool_calls":[]}' \
	>"$antigravity_transcript_success"
antigravity_success_result=$(sh "$antigravity_ide_proof" "$antigravity_transcript_success" "$repository_root/testdata/runtime/antigravity-project")
antigravity_success_status=$?
assert_status 0 "$antigravity_success_status" "preuve Antigravity IDE avec deux sentinelles"
if [ "$antigravity_success_result" != "ANTIGRAVITY_IDE_OK" ]; then
	fail_test "preuve Antigravity IDE: résultat inattendu"
fi

antigravity_native_transcript_directory="$antigravity_transcript_directory/native"
mkdir -p "$antigravity_native_transcript_directory"
antigravity_native_transcript_success="$antigravity_native_transcript_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"<USER_REQUEST>\n/sth-runtime-command\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nfixture\n</ADDITIONAL_METADATA>","tool_calls":[]}' \
	'{"step_index":2,"source":"SYSTEM","type":"CONVERSATION_HISTORY","status":"DONE","tool_calls":[]}' \
	'{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[]}' \
	'{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_OK","tool_calls":[]}' \
	'{"step_index":5,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"<USER_REQUEST>\nReturn the STH instruction sentinel.\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nfixture\n</ADDITIONAL_METADATA>","tool_calls":[]}' \
	'{"step_index":6,"source":"SYSTEM","type":"EPHEMERAL_MESSAGE","status":"DONE","tool_calls":[]}' \
	'{"step_index":7,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[]}' \
	'{"step_index":8,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_OK","tool_calls":[]}' \
	>"$antigravity_native_transcript_success"
antigravity_native_success_result=$(sh "$antigravity_ide_proof" "$antigravity_native_transcript_success" "$repository_root/testdata/runtime/antigravity-project")
antigravity_native_success_status=$?
assert_status 0 "$antigravity_native_success_status" "preuve Antigravity IDE avec transcript natif enveloppé"
if [ "$antigravity_native_success_result" != "ANTIGRAVITY_IDE_OK" ]; then
	fail_test "preuve Antigravity IDE native: résultat inattendu"
fi

antigravity_native_null_content_directory="$antigravity_transcript_directory/native-null-content"
mkdir -p "$antigravity_native_null_content_directory"
antigravity_native_null_content="$antigravity_native_null_content_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":null,"tool_calls":[]}' \
	>"$antigravity_native_null_content"
sh "$antigravity_ide_proof" "$antigravity_native_null_content" "$repository_root/testdata/runtime/antigravity-project" >"$invalid_output" 2>"$invalid_error"
antigravity_native_null_content_status=$?
assert_status 1 "$antigravity_native_null_content_status" "preuve Antigravity IDE avec contenu null"

antigravity_native_metadata_prompt_directory="$antigravity_transcript_directory/native-metadata-prompt"
mkdir -p "$antigravity_native_metadata_prompt_directory"
antigravity_native_metadata_prompt="$antigravity_native_metadata_prompt_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"<ADDITIONAL_METADATA>\n/sth-runtime-command\n</ADDITIONAL_METADATA>","tool_calls":[]}' \
	'{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_OK","tool_calls":[]}' \
	'{"step_index":3,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"<ADDITIONAL_METADATA>\nReturn the STH instruction sentinel.\n</ADDITIONAL_METADATA>","tool_calls":[]}' \
	'{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_OK","tool_calls":[]}' \
	>"$antigravity_native_metadata_prompt"
sh "$antigravity_ide_proof" "$antigravity_native_metadata_prompt" "$repository_root/testdata/runtime/antigravity-project" >"$invalid_output" 2>"$invalid_error"
antigravity_native_metadata_prompt_status=$?
assert_status 1 "$antigravity_native_metadata_prompt_status" "preuve Antigravity IDE avec prompt seulement dans métadonnées"

sh "$antigravity_ide_proof" "$antigravity_transcript_success" "$repository_root/testdata/runtime/antigravity-project" OK AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA test >"$invalid_output" 2>"$invalid_error"
antigravity_challenge_status=$?
assert_status 1 "$antigravity_challenge_status" "preuve Antigravity IDE sans challenge frais"

antigravity_false_positive_directory="$test_directory/antigravity-false-positive"
mkdir -p "$antigravity_false_positive_directory"
antigravity_false_positive="$antigravity_false_positive_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"/sth-runtime-command","tool_calls":[]}' \
	'{"step_index":2,"source":"SYSTEM","type":"SYSTEM_MESSAGE","status":"DONE","content":"ANTIGRAVITY_COMMAND_OK","tool_calls":[]}' \
	'{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"commande exécutée","tool_calls":[]}' \
	'{"step_index":4,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"Return the STH instruction sentinel.","tool_calls":[]}' \
	'{"step_index":5,"source":"SYSTEM","type":"SYSTEM_MESSAGE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_OK","tool_calls":[]}' \
	'{"step_index":6,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"instruction exécutée","tool_calls":[]}' \
	>"$antigravity_false_positive"
antigravity_false_positive_output="$test_directory/antigravity-false-positive.out"
antigravity_false_positive_error="$test_directory/antigravity-false-positive.err"
sh "$antigravity_ide_proof" "$antigravity_false_positive" "$repository_root/testdata/runtime/antigravity-project" >"$antigravity_false_positive_output" 2>"$antigravity_false_positive_error"
antigravity_false_positive_status=$?
assert_status 1 "$antigravity_false_positive_status" "preuve Antigravity IDE sans sentinelle modèle"

antigravity_replay_directory="$test_directory/antigravity-replay"
mkdir -p "$antigravity_replay_directory"
antigravity_replay="$antigravity_replay_directory/transcript_full.jsonl"
printf '%s\n' \
	'{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"/sth-runtime-command","tool_calls":[]}' \
	'{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_OK","tool_calls":[]}' \
	'{"step_index":3,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"Return the STH instruction sentinel.","tool_calls":[]}' \
	'{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_OK","tool_calls":[]}' \
	'{"step_index":5,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"/sth-runtime-command","tool_calls":[]}' \
	'{"step_index":6,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"commande échouée","tool_calls":[]}' \
	'{"step_index":7,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","content":"Return the STH instruction sentinel.","tool_calls":[]}' \
	'{"step_index":8,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"instruction échouée","tool_calls":[]}' \
	>"$antigravity_replay"
sh "$antigravity_ide_proof" "$antigravity_replay" "$repository_root/testdata/runtime/antigravity-project" >"$invalid_output" 2>"$invalid_error"
antigravity_replay_status=$?
assert_status 1 "$antigravity_replay_status" "preuve Antigravity IDE avec dernière tentative en échec"

copilot_replay="$test_directory/copilot-transcript-replay.jsonl"
printf '%s\n' \
	"{\"kind\":0,\"v\":{\"requests\":[{\"agent\":{\"extensionId\":{\"value\":\"GitHub.copilot-chat\"},\"extensionVersion\":\"test\"},\"message\":{\"parts\":[{\"kind\":\"prompt\",\"name\":\"sth-runtime-command\"}]},\"variableData\":{\"variables\":[{\"kind\":\"promptFile\",\"isRoot\":true,\"automaticallyAdded\":true,\"value\":{\"path\":\"$copilot_prompt_path\"}}]},\"result\":{\"response\":\"COPILOT_COMMAND_OK\"}},{\"agent\":{\"extensionId\":{\"value\":\"GitHub.copilot-chat\"},\"extensionVersion\":\"test\"},\"message\":{\"parts\":[{\"kind\":\"prompt\",\"name\":\"sth-runtime-command\"}]},\"variableData\":{\"variables\":[]},\"result\":{\"response\":\"échec\"}}]}}" \
	>"$copilot_replay"
sh "$copilot_ide_proof" "$copilot_replay" "$repository_root/testdata/runtime/copilot-project" >"$invalid_output" 2>"$invalid_error"
copilot_replay_status=$?
assert_status 1 "$copilot_replay_status" "preuve Copilot IDE avec dernière résolution cassée"

actual_fixture_manifest="$test_directory/fixture-manifest.txt"
(
	cd "$repository_root" &&
	find testdata/runtime/antigravity-project \
		testdata/runtime/claude-project \
		testdata/runtime/codex-project \
		testdata/runtime/copilot-project \
		testdata/runtime/gemini-project \
		-type f -print | sort
) >"$actual_fixture_manifest"
if ! diff -u "$repository_root/testdata/runtime/fixture-manifest.txt" "$actual_fixture_manifest"; then
	fail_test "manifeste des fixtures incomplet ou obsolète"
fi

gemini_parent_env_root="$test_directory/gemini-parent-env"
mkdir -p "$gemini_parent_env_root/.gemini"
printf 'GOOGLE_GEMINI_BASE_URL=https://untrusted.invalid\n' >"$gemini_parent_env_root/.gemini/.env"
TMPDIR="$gemini_parent_env_root" \
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=0 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
gemini_parent_env_status=$?
assert_status 2 "$gemini_parent_env_status" "audit Gemini avec .gemini/.env dans un parent"

audit_output="$test_directory/audit-absent.tsv"
audit_error="$test_directory/audit-absent.err"
run_without_consumers 0 1 >"$audit_output" 2>"$audit_error"
audit_status=$?
assert_status 0 "$audit_status" "audit sans binaires"

if ! awk -F '\t' 'NR == 1 && $1 == "status" && $2 == "target" && $3 == "artifact" && $4 == "evidence" && $5 == "mode" && NF == 5 { valid = 1 } END { exit valid ? 0 : 1 }' "$audit_output"; then
	fail_test "audit sans binaires: en-tête TSV invalide"
fi
if ! awk -F '\t' 'NR > 1 && ($5 != "real" || NF != 5) { invalid = 1 } END { exit invalid ? 1 : 0 }' "$audit_output"; then
	fail_test "audit sans binaires: attestation de mode réel absente"
fi

audit_contracts="$test_directory/audit-absent-contracts.tsv"
awk 'NR > 1 { print $1 "\t" $2 "\t" $3 }' "$audit_output" | sort -k2,2 -k3,3 >"$audit_contracts"
if ! diff -u "$expected_absent_contracts" "$audit_contracts"; then
	fail_test "audit sans binaires: manifeste de contrats inattendu"
fi
if ! awk 'END { if (NR != 26) exit 1 }' "$audit_output"; then
	fail_test "audit sans binaires: un en-tête et 25 preuves attendus"
fi

strict_output="$test_directory/strict-absent.tsv"
strict_error="$test_directory/strict-absent.err"
run_without_consumers 1 1 >"$strict_output" 2>"$strict_error"
strict_status=$?
assert_status 2 "$strict_status" "certification stricte sans fixtures générées"

invalid_output="$test_directory/invalid.out"
invalid_error="$test_directory/invalid.err"
STRICT_RUNTIME_CERTIFICATION=invalid RUN_MODEL_SMOKES=1 sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "valeur STRICT_RUNTIME_CERTIFICATION invalide"

STRICT_RUNTIME_CERTIFICATION=0 RUN_MODEL_SMOKES=invalid sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "valeur RUN_MODEL_SMOKES invalide"

COMPAT_HERMETIC=invalid sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "valeur COMPAT_HERMETIC invalide"

STRICT_RUNTIME_CERTIFICATION=1 RUN_MODEL_SMOKES=0 sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "smokes désactivés en certification stricte"

runtime_identity_binary="$test_directory/runtime-identity-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  printf "runtime identity 1.2.3\\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$runtime_identity_binary"
chmod 755 "$runtime_identity_binary"
runtime_identity_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$runtime_identity_binary") || fail_test "preuve identité runtime: calcul SHA-256 impossible"
runtime_identity_result=$(sh "$runtime_binary_proof" "$runtime_identity_binary" 1.2.3 "$runtime_identity_sha256")
runtime_identity_status=$?
assert_status 0 "$runtime_identity_status" "preuve identité runtime positive"
if [ "$runtime_identity_result" != "RUNTIME_BINARY_IDENTITY_OK" ]; then
	fail_test "preuve identité runtime positive: résultat inattendu"
fi

runtime_identity_link="$test_directory/runtime-identity-link"
ln -s "$runtime_identity_binary" "$runtime_identity_link"
runtime_identity_result=$(sh "$runtime_binary_proof" "$runtime_identity_link" 1.2.3 "$runtime_identity_sha256")
runtime_identity_status=$?
assert_status 0 "$runtime_identity_status" "preuve identité runtime positive via exécutable résolu"
if [ "$runtime_identity_result" != "RUNTIME_BINARY_IDENTITY_OK" ]; then
	fail_test "preuve identité runtime via exécutable résolu: résultat inattendu"
fi

sh "$runtime_binary_proof" "$runtime_identity_binary" "" "$runtime_identity_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime sans version attendue"

sh "$runtime_binary_proof" "$runtime_identity_binary" 1.2.3 "" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime sans SHA-256 attendu"

sh "$runtime_binary_proof" "$runtime_identity_binary" 1.2.4 "$runtime_identity_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec version différente"

sh "$runtime_binary_proof" "$runtime_identity_binary" 1.2.3 0000000000000000000000000000000000000000000000000000000000000000 >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec SHA-256 différent"

runtime_identity_sentence_binary="$test_directory/runtime-identity-sentence-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  printf "GitHub Copilot CLI 1.2.3.\\nRun update to check for updates.\\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$runtime_identity_sentence_binary"
chmod 755 "$runtime_identity_sentence_binary"
runtime_identity_sentence_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$runtime_identity_sentence_binary") || fail_test "preuve identité runtime ponctuée: calcul SHA-256 impossible"
runtime_identity_result=$(sh "$runtime_binary_proof" "$runtime_identity_sentence_binary" 1.2.3 "$runtime_identity_sentence_sha256")
runtime_identity_status=$?
assert_status 0 "$runtime_identity_status" "preuve identité runtime avec version suivie d'un point final"
if [ "$runtime_identity_result" != "RUNTIME_BINARY_IDENTITY_OK" ]; then
	fail_test "preuve identité runtime ponctuée: résultat inattendu"
fi

sh "$runtime_binary_proof" "$runtime_identity_sentence_binary" 1.2 "$runtime_identity_sentence_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec préfixe de version ponctuée"

runtime_identity_longer_binary="$test_directory/runtime-identity-longer-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  printf "runtime identity 1.2.30\\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$runtime_identity_longer_binary"
chmod 755 "$runtime_identity_longer_binary"
runtime_identity_longer_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$runtime_identity_longer_binary") || fail_test "preuve identité runtime étendue: calcul SHA-256 impossible"
sh "$runtime_binary_proof" "$runtime_identity_longer_binary" 1.2.3 "$runtime_identity_longer_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec version plus longue"

runtime_identity_suffix_binary="$test_directory/runtime-identity-suffix-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  printf "runtime identity 1.2.3.4\\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$runtime_identity_suffix_binary"
chmod 755 "$runtime_identity_suffix_binary"
runtime_identity_suffix_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$runtime_identity_suffix_binary") || fail_test "preuve identité runtime suffixée: calcul SHA-256 impossible"
sh "$runtime_binary_proof" "$runtime_identity_suffix_binary" 1.2.3 "$runtime_identity_suffix_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec version prolongée par un point"

runtime_identity_prerelease_binary="$test_directory/runtime-identity-prerelease-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  printf "runtime identity 1.2.3-beta\\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$runtime_identity_prerelease_binary"
chmod 755 "$runtime_identity_prerelease_binary"
runtime_identity_prerelease_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$runtime_identity_prerelease_binary") || fail_test "preuve identité runtime préversion: calcul SHA-256 impossible"
sh "$runtime_binary_proof" "$runtime_identity_prerelease_binary" 1.2.3 "$runtime_identity_prerelease_sha256" >"$invalid_output" 2>"$invalid_error"
runtime_identity_status=$?
assert_status 1 "$runtime_identity_status" "preuve identité runtime avec préversion"

CLAUDE_BIN="$fake_claude" \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
RUN_MODEL_SMOKES=0 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "faux binaire interdit hors mode hermétique"

linked_fake_claude="$test_directory/linked-fake-claude"
ln -s "$fake_claude" "$linked_fake_claude"
CLAUDE_BIN="$linked_fake_claude" \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
RUN_MODEL_SMOKES=0 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "faux binaire lié symboliquement interdit hors mode hermétique"

hard_linked_fake_claude="$test_directory/hard-linked-fake-claude"
ln "$fake_claude" "$hard_linked_fake_claude"
CLAUDE_BIN="$hard_linked_fake_claude" \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
RUN_MODEL_SMOKES=0 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "faux binaire lié physiquement interdit hors mode hermétique"

copied_fake_claude="$test_directory/copied-fake-claude"
cp "$fake_claude" "$copied_fake_claude"
CLAUDE_BIN="$copied_fake_claude" \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
RUN_MODEL_SMOKES=0 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "copie d'un faux binaire interdite hors mode hermétique"

claude_model_calls="$test_directory/claude-model-calls.txt"
claude_discovery_calls="$test_directory/claude-discovery-calls.txt"
claude_output="$test_directory/claude-success.tsv"
claude_error="$test_directory/claude-success.err"
CLAUDE_BIN="$fake_claude" \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
FAKE_CLAUDE_MODEL_CALLS="$claude_model_calls" \
	FAKE_CLAUDE_DISCOVERY_CALLS="$claude_discovery_calls" \
	COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$claude_output" 2>"$claude_error"
claude_status=$?
assert_status 0 "$claude_status" "audit Claude avec CLI authentifié"

if ! awk 'END { if (NR != 1) exit 1 }' "$claude_discovery_calls"; then
	fail_test "Claude: l'initialisation locale de découverte n'a pas été exécutée exactement une fois"
fi
if ! awk 'END { if (NR != 4) exit 1 }' "$claude_model_calls"; then
	fail_test "Claude: les quatre smokes modèle n'ont pas été conservés"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "OK" && $2 == "claude" && $3 == expected_artifact { found = 1 } END { exit found ? 0 : 1 }' "$claude_output"; then
		fail_test "Claude $artifact: smoke modèle OK absent"
	fi
done
for discovery in skill-discovery agent-discovery command-discovery instruction-discovery; do
	if ! awk -v expected_discovery="$discovery" '$1 == "OK" && $2 == "claude" && $3 == expected_discovery { found = 1 } END { exit found ? 0 : 1 }' "$claude_output"; then
		fail_test "Claude $discovery: preuve locale sans modèle absente"
	fi
done

codex_calls="$test_directory/codex-model-calls.txt"
codex_output="$test_directory/codex-auth.tsv"
codex_error="$test_directory/codex-auth.err"
CLAUDE_BIN= \
CODEX_BIN="$fake_codex" \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
	FAKE_CODEX_MODEL_CALLS="$codex_calls" \
	COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$codex_output" 2>"$codex_error"
codex_status=$?
assert_status 0 "$codex_status" "audit Codex sans authentification"

if ! awk 'END { if (NR != 1) exit 1 }' "$codex_calls"; then
	fail_test "Codex: les smokes agent, command et instruction n'ont pas été court-circuités"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "NON_EXECUTE" && $2 == "codex" && $3 == expected_artifact { found = 1 } END { exit found ? 0 : 1 }' "$codex_output"; then
		fail_test "Codex $artifact: résultat d'authentification absent"
	fi
done
for discovery in skill-discovery command-discovery instruction-discovery; do
	if ! awk -v expected_discovery="$discovery" '$1 == "OK" && $2 == "codex" && $3 == expected_discovery { found = 1 } END { exit found ? 0 : 1 }' "$codex_output"; then
		fail_test "Codex $discovery: preuve absente"
	fi
done

codex_retry_calls="$test_directory/codex-retry-model-calls.txt"
codex_retry_agent_calls="$test_directory/codex-retry-agent-calls.txt"
codex_retry_output="$test_directory/codex-retry.tsv"
codex_retry_error="$test_directory/codex-retry.err"
CLAUDE_BIN= \
CODEX_BIN="$fake_codex_agent_transient" \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
FAKE_CODEX_MODEL_CALLS="$codex_retry_calls" \
	FAKE_CODEX_AGENT_CALLS="$codex_retry_agent_calls" \
	COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$codex_retry_output" 2>"$codex_retry_error"
codex_retry_status=$?
assert_status 0 "$codex_retry_status" "audit Codex avec délégation transitoire"

if ! awk 'END { if (NR != 5) exit 1 }' "$codex_retry_calls"; then
	fail_test "Codex: nombre inattendu d'appels modèle avec retry agent"
fi
if ! awk 'END { if (NR != 2) exit 1 }' "$codex_retry_agent_calls"; then
	fail_test "Codex agent: la délégation transitoire n'a pas été retentée une fois"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "OK" && $2 == "codex" && $3 == expected_artifact { found = 1 } END { exit found ? 0 : 1 }' "$codex_retry_output"; then
		fail_test "Codex $artifact: preuve OK absente après retry"
	fi
done

antigravity_calls="$test_directory/antigravity-model-calls.txt"
antigravity_output="$test_directory/antigravity-success.tsv"
antigravity_error="$test_directory/antigravity-success.err"
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN="$fake_antigravity_success" \
	FAKE_ANTIGRAVITY_MODEL_CALLS="$antigravity_calls" \
	COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$antigravity_output" 2>"$antigravity_error"
antigravity_status=$?
assert_status 0 "$antigravity_status" "audit Antigravity avec CLI authentifié"

if ! awk 'END { if (NR != 4) exit 1 }' "$antigravity_calls"; then
	fail_test "Antigravity: les quatre smokes CLI n'ont pas été exécutés"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "OK" && $2 == "antigravity-cli" && $3 == expected_artifact { found = 1 } END { exit found ? 0 : 1 }' "$antigravity_output"; then
		fail_test "Antigravity $artifact: preuve OK absente"
	fi
done
if ! awk '$1 == "OK" && $2 == "antigravity-cli" && $3 == "agent-discovery" { found = 1 } END { exit found ? 0 : 1 }' "$antigravity_output"; then
	fail_test "Antigravity agent-discovery: preuve OK absente"
fi

antigravity_auth_calls="$test_directory/antigravity-auth-model-calls.txt"
antigravity_auth_output="$test_directory/antigravity-auth.tsv"
antigravity_auth_error="$test_directory/antigravity-auth.err"
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN= \
ANTIGRAVITY_BIN="$fake_antigravity_auth" \
FAKE_ANTIGRAVITY_MODEL_CALLS="$antigravity_auth_calls" \
COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$antigravity_auth_output" 2>"$antigravity_auth_error"
antigravity_auth_status=$?
assert_status 0 "$antigravity_auth_status" "audit Antigravity sans authentification"
if ! awk 'END { if (NR != 1) exit 1 }' "$antigravity_auth_calls"; then
	fail_test "Antigravity: les smokes suivants n'ont pas été court-circuités après l'erreur OAuth"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "NON_EXECUTE" && $2 == "antigravity-cli" && $3 == expected_artifact { found = 1 } END { exit found ? 0 : 1 }' "$antigravity_auth_output"; then
		fail_test "Antigravity $artifact: erreur OAuth réelle non classée"
	fi
done

copilot_calls="$test_directory/copilot-model-calls.txt"
copilot_output="$test_directory/copilot-auth.tsv"
copilot_error="$test_directory/copilot-auth.err"
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN="$fake_copilot" \
GEMINI_BIN= \
ANTIGRAVITY_BIN= \
	FAKE_COPILOT_MODEL_CALLS="$copilot_calls" \
	COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$copilot_output" 2>"$copilot_error"
copilot_status=$?
assert_status 0 "$copilot_status" "audit Copilot sans authentification"

if ! awk 'END { if (NR != 1) exit 1 }' "$copilot_calls"; then
	fail_test "Copilot: les smokes agent, command et instruction n'ont pas été court-circuités"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "NON_EXECUTE" && $2 == "copilot-cli" && $3 == expected_artifact && $4 == "authentification" { found = 1 } END { exit found ? 0 : 1 }' "$copilot_output"; then
		fail_test "Copilot $artifact: résultat d'authentification absent"
	fi
done
for discovery in skill-discovery command-discovery; do
	if ! awk -v expected_discovery="$discovery" '$1 == "OK" && $2 == "copilot-cli" && $3 == expected_discovery { found = 1 } END { exit found ? 0 : 1 }' "$copilot_output"; then
		fail_test "Copilot $discovery: preuve absente"
	fi
done
if ! awk '$1 == "INFO" && $2 == "copilot-cli" && $3 == "instruction-discovery" && $4 == "inspection" { found = 1 } END { exit found ? 0 : 1 }' "$copilot_output"; then
	fail_test "Copilot instruction-discovery: indisponibilité non classée"
fi

gemini_calls="$test_directory/gemini-model-calls.txt"
gemini_output="$test_directory/gemini-auth.tsv"
gemini_error="$test_directory/gemini-auth.err"
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN="$fake_gemini" \
ANTIGRAVITY_BIN= \
FAKE_GEMINI_MODEL_CALLS="$gemini_calls" \
COMPAT_HERMETIC=1 \
GEMINI_CLI_ACTIVITY_LOG_TARGET=ambient \
GEMINI_CLI_HOME="$test_directory/ambient-gemini-home" \
GEMINI_CLI_NO_RELAUNCH=false \
GEMINI_CLI_SYSTEM_DEFAULTS_PATH="$test_directory/ambient-gemini-defaults.json" \
GEMINI_CLI_SYSTEM_SETTINGS_PATH="$test_directory/ambient-gemini-settings.json" \
CODE_ASSIST_ENDPOINT=https://ambient.invalid \
GEMINI_CLI_CUSTOM_HEADERS=ambient \
GEMINI_COMPAT_APPROVAL_MODE=yolo \
GEMINI_SANDBOX=true \
GEMINI_TELEMETRY_ENABLED=true \
GEMINI_TELEMETRY_LOG_PROMPTS=true \
GEMINI_TELEMETRY_TRACES_ENABLED=true \
GOOGLE_GEMINI_BASE_URL=https://ambient.invalid \
GOOGLE_VERTEX_BASE_URL=https://ambient.invalid \
SANDBOX=ambient \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$gemini_output" 2>"$gemini_error"
gemini_status=$?
assert_status 0 "$gemini_status" "audit Gemini sans authentification"

if ! awk 'END { if (NR != 1) exit 1 }' "$gemini_calls"; then
	fail_test "Gemini: les smokes agent, command et instruction n'ont pas été court-circuités"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "NON_EXECUTE" && $2 == "gemini" && $3 == expected_artifact && $4 == "authentification" { found = 1 } END { exit found ? 0 : 1 }' "$gemini_output"; then
		fail_test "Gemini $artifact: résultat d'authentification absent"
	fi
done

gemini_system_policies="$test_directory/gemini-system-policies"
gemini_system_policy_output="$test_directory/gemini-system-policy.tsv"
gemini_system_policy_error="$test_directory/gemini-system-policy.err"
gemini_system_policy_calls="$test_directory/gemini-system-policy-calls.txt"
mkdir -p "$gemini_system_policies"
printf '[[rule]]\ntoolName = "*"\ndecision = "deny"\npriority = 999\n' >"$gemini_system_policies/managed.toml"
: >"$gemini_system_policy_calls"
CLAUDE_BIN= \
CODEX_BIN= \
COPILOT_BIN= \
GEMINI_BIN="$fake_gemini_success" \
ANTIGRAVITY_BIN= \
FAKE_GEMINI_MODEL_CALLS="$gemini_system_policy_calls" \
GEMINI_COMPAT_SYSTEM_POLICIES_DIR="$gemini_system_policies" \
COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=0 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$gemini_system_policy_output" 2>"$gemini_system_policy_error"
gemini_system_policy_status=$?
assert_status 0 "$gemini_system_policy_status" "audit Gemini avec policy système prioritaire"
if [ -s "$gemini_system_policy_calls" ]; then
	fail_test "Gemini: un modèle a été appelé malgré la policy système prioritaire"
fi
for artifact in skill agent command instruction; do
	if ! awk -v expected_artifact="$artifact" '$1 == "NON_EXECUTE" && $2 == "gemini" && $3 == expected_artifact && $4 == "policy" { found = 1 } END { exit found ? 0 : 1 }' "$gemini_system_policy_output"; then
		fail_test "Gemini $artifact: neutralisation de --admin-policy non signalée"
	fi
done

strict_sth_binary="$test_directory/sth-strict"
if ! (
	cd "$repository_root" &&
	GOPROXY=off GOSUMDB=off go build -o "$strict_sth_binary" .
); then
	fail_test "certification stricte: compilation du vrai binaire STH échouée"
fi

strict_fixture_root="$test_directory/strict-fixtures"
strict_lifecycle_output="$test_directory/strict-lifecycle.out"
strict_lifecycle_error="$test_directory/strict-lifecycle.err"
if ! COMPAT_FIXTURE_EXPORT_ROOT="$strict_fixture_root" \
	STH_BIN="$strict_sth_binary" \
	sh "$lifecycle_runner" >"$strict_lifecycle_output" 2>"$strict_lifecycle_error"; then
	sed -n '1,160p' "$strict_lifecycle_output" >&2
	sed -n '1,160p' "$strict_lifecycle_error" >&2
	fail_test "certification stricte: export des fixtures par le vrai binaire STH échoué"
fi
strict_runtime_challenge=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["runtime_challenge"])' "$strict_fixture_root/.sth-compat-provenance.json") || fail_test "certification stricte: challenge runtime absent"

COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=0 \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
strict_identity_missing_status=$?
assert_status 2 "$strict_identity_missing_status" "certification stricte réelle sans identité CLI attendue"
if ! grep -Fq 'CLAUDE_COMPAT_EXPECTED_VERSION est obligatoire en certification stricte réelle' "$invalid_error"; then
	fail_test "certification stricte réelle sans identité CLI: diagnostic absent"
fi

fake_vscode="$test_directory/fake-vscode.sh"
fake_vscode_marker="$test_directory/fake-vscode.marker"
fake_vscode_arguments="$test_directory/fake-vscode-arguments.txt"
printf '%s\n' \
	'#!/bin/sh' \
	'set -u' \
	': "${FAKE_VSCODE_MARKER:?}"' \
	': "${FAKE_VSCODE_ARGUMENTS:?}"' \
	'if [ -n "${VSCODE_IPC_HOOK_CLI:-}" ]; then' \
	'	exit 3' \
	'fi' \
	'printf "invoked\n" >"$FAKE_VSCODE_MARKER"' \
	'printf "%s\n" "$@" >"$FAKE_VSCODE_ARGUMENTS"' \
	'user_data_directory=' \
	'while [ "$#" -gt 0 ]; do' \
	'	if [ "$1" = "--user-data-dir" ]; then' \
	'		shift' \
	'		user_data_directory=${1:-}' \
	'	fi' \
	'	shift' \
	'done' \
	'if [ -z "$user_data_directory" ]; then' \
	'	exit 2' \
	'fi' \
	'transcript_directory="$user_data_directory/User/workspaceStorage/fake/chatSessions"' \
	'mkdir -p "$transcript_directory"' \
	'printf "{}\n" >"$transcript_directory/fake.jsonl"' \
	>"$fake_vscode"
chmod 755 "$fake_vscode"

copilot_ide_tmp="$test_directory/copilot-ide-tmp"
mkdir -p "$copilot_ide_tmp"
copilot_ide_tmp=$(CDPATH= cd -- "$copilot_ide_tmp" && pwd)
strict_copilot_project=$(CDPATH= cd -- "$strict_fixture_root/copilot-project" && pwd)

TMPDIR="$copilot_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN= \
COPILOT_IDE_VSCODE_BIN="$fake_vscode" \
	sh "$copilot_ide_launch" --prepare-only >"$invalid_output" 2>"$invalid_error"
copilot_ide_missing_binary_status=$?
assert_status 2 "$copilot_ide_missing_binary_status" "préparation Copilot IDE sans binaire STH exact"

copilot_ide_prepare_output="$test_directory/copilot-ide-prepare.out"
copilot_ide_prepare_error="$test_directory/copilot-ide-prepare.err"
TMPDIR="$copilot_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
COPILOT_IDE_VSCODE_BIN="$fake_vscode" \
	sh "$copilot_ide_launch" --prepare-only >"$copilot_ide_prepare_output" 2>"$copilot_ide_prepare_error"
copilot_ide_prepare_status=$?
assert_status 0 "$copilot_ide_prepare_status" "préparation isolée Copilot IDE sans lancement"
if [ -e "$fake_vscode_marker" ]; then
	fail_test "préparation isolée Copilot IDE: le binaire VS Code a été lancé"
fi
if ! grep -Fxq "Challenge runtime IDE : $strict_runtime_challenge" "$copilot_ide_prepare_output"; then
	fail_test "préparation isolée Copilot IDE: challenge frais absent"
fi
if ! grep -Fxq "Requête Copilot IDE : /sth-runtime-command STH_RUNTIME_CHALLENGE_$strict_runtime_challenge" "$copilot_ide_prepare_output"; then
	fail_test "préparation isolée Copilot IDE: requête avec challenge absente"
fi
copilot_ide_prepare_root=$(sed -n 's/^Session Copilot IDE : //p' "$copilot_ide_prepare_output")
case "$copilot_ide_prepare_root" in
	"$copilot_ide_tmp"/sth-copilot-ide.*)
		;;
	*)
		fail_test "préparation isolée Copilot IDE: racine temporaire inattendue"
		;;
esac
if [ ! -d "$copilot_ide_prepare_root/user-data" ] || [ ! -d "$copilot_ide_prepare_root/extensions" ]; then
	fail_test "préparation isolée Copilot IDE: dossiers isolés absents"
fi
if ! python3 - "$copilot_ide_prepare_root/vscode-arguments.txt" "$copilot_ide_prepare_root" "$strict_copilot_project" <<'PY'
import sys

arguments_path, session_root, project_directory = sys.argv[1:]
with open(arguments_path, encoding="utf-8") as arguments_stream:
    arguments = arguments_stream.read().splitlines()
expected_arguments = [
    "--new-window",
    "--wait",
    "--user-data-dir",
    f"{session_root}/user-data",
    "--extensions-dir",
    f"{session_root}/extensions",
    "--sync",
    "off",
    "--use-inmemory-secretstorage",
    project_directory,
]
if arguments != expected_arguments:
    raise SystemExit(f"arguments VS Code inattendus: {arguments!r}")
PY
then
	fail_test "préparation isolée Copilot IDE: arguments VS Code invalides"
fi

copilot_ide_launch_output="$test_directory/copilot-ide-launch.out"
copilot_ide_launch_error="$test_directory/copilot-ide-launch.err"
TMPDIR="$copilot_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
COPILOT_IDE_VSCODE_BIN="$fake_vscode" \
FAKE_VSCODE_MARKER="$fake_vscode_marker" \
FAKE_VSCODE_ARGUMENTS="$fake_vscode_arguments" \
VSCODE_IPC_HOOK_CLI="$test_directory/unexpected-ipc-hook" \
	sh "$copilot_ide_launch" >"$copilot_ide_launch_output" 2>"$copilot_ide_launch_error"
copilot_ide_launch_status=$?
assert_status 0 "$copilot_ide_launch_status" "lancement isolé Copilot IDE avec faux binaire"
if [ ! -f "$fake_vscode_marker" ] || [ ! -f "$fake_vscode_arguments" ]; then
	fail_test "lancement isolé Copilot IDE: faux binaire non exécuté"
fi
copilot_ide_launch_root=$(sed -n 's/^Session Copilot IDE : //p' "$copilot_ide_launch_output")
copilot_ide_transcript=$(sed -n 's/^Transcript Copilot IDE : //p' "$copilot_ide_launch_output")
case "$copilot_ide_transcript" in
	"$copilot_ide_launch_root"/user-data/User/workspaceStorage/*/chatSessions/*.jsonl)
		;;
	*)
		fail_test "lancement isolé Copilot IDE: chemin de transcript inattendu"
		;;
esac
if [ ! -f "$copilot_ide_transcript" ]; then
	fail_test "lancement isolé Copilot IDE: transcript récupéré absent"
fi
expected_transcript_entry=$(printf 'transcript\t%s' "$copilot_ide_transcript")
if ! grep -Fxq "$expected_transcript_entry" "$copilot_ide_launch_root/session-info.tsv"; then
	fail_test "lancement isolé Copilot IDE: transcript absent des métadonnées"
fi
if ! python3 - "$fake_vscode_arguments" "$copilot_ide_launch_root" "$strict_copilot_project" <<'PY'
import sys

arguments_path, session_root, project_directory = sys.argv[1:]
with open(arguments_path, encoding="utf-8") as arguments_stream:
    arguments = arguments_stream.read().splitlines()
expected_arguments = [
    "--new-window",
    "--wait",
    "--user-data-dir",
    f"{session_root}/user-data",
    "--extensions-dir",
    f"{session_root}/extensions",
    "--sync",
    "off",
    "--use-inmemory-secretstorage",
    project_directory,
]
if arguments != expected_arguments:
    raise SystemExit(f"arguments reçus par VS Code inattendus: {arguments!r}")
if any(argument.startswith("--password-store") for argument in arguments):
    raise SystemExit("stockage basic interdit dans le helper macOS")
PY
then
	fail_test "lancement isolé Copilot IDE: arguments reçus invalides"
fi

COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=1 \
COPILOT_IDE_EXTENSION_VERSION= \
ANTIGRAVITY_IDE_VERSION= \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "certification stricte sans versions IDE"

strict_copilot_transcript="$test_directory/strict-copilot-transcript.jsonl"
strict_copilot_prompt_path="$strict_fixture_root/copilot-project/.github/prompts/sth-runtime-command.prompt.md"
printf '{"kind":0,"v":{"requests":[{"agent":{"extensionId":{"value":"GitHub.copilot-chat"},"extensionVersion":"test"},"message":{"parts":[{"kind":"prompt","name":"sth-runtime-command"},{"kind":"text","text":"STH_RUNTIME_CHALLENGE_%s"}]},"variableData":{"variables":[{"kind":"promptFile","isRoot":true,"automaticallyAdded":true,"value":{"path":"%s"}}]},"result":{"response":"COPILOT_COMMAND_UPDATED"}}]}}\n' \
	"$strict_runtime_challenge" "$strict_copilot_prompt_path" >"$strict_copilot_transcript"

strict_antigravity_transcript_directory="$test_directory/strict-antigravity-transcript"
mkdir -p "$strict_antigravity_transcript_directory"
strict_antigravity_transcript="$strict_antigravity_transcript_directory/transcript_full.jsonl"
printf '%s\n' \
	"{\"step_index\":1,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"status\":\"DONE\",\"content\":\"/sth-runtime-command STH_RUNTIME_CHALLENGE_$strict_runtime_challenge ANTIGRAVITY_IDE_VERSION_test\",\"tool_calls\":[]}" \
	'{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_UPDATED","tool_calls":[]}' \
	"{\"step_index\":3,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"status\":\"DONE\",\"content\":\"Return the STH instruction sentinel. STH_RUNTIME_CHALLENGE_$strict_runtime_challenge ANTIGRAVITY_IDE_VERSION_test\",\"tool_calls\":[]}" \
	'{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_UPDATED","tool_calls":[]}' \
	>"$strict_antigravity_transcript"

strict_antigravity_application="$test_directory/Antigravity IDE.app"
strict_antigravity_application_cli="$strict_antigravity_application/Contents/Resources/app/bin/antigravity-ide"
mkdir -p "$strict_antigravity_application/Contents/MacOS" "$(dirname -- "$strict_antigravity_application_cli")"
printf '%s\n' \
	'<?xml version="1.0" encoding="UTF-8"?>' \
	'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	'<plist version="1.0"><dict>' \
	'<key>CFBundleIdentifier</key><string>com.google.antigravity-ide</string>' \
	'<key>CFBundleShortVersionString</key><string>test</string>' \
	'<key>CFBundleExecutable</key><string>Electron</string>' \
	'</dict></plist>' \
	>"$strict_antigravity_application/Contents/Info.plist"
printf '#!/bin/sh\nexit 0\n' >"$strict_antigravity_application/Contents/MacOS/Electron"
printf '%s\n' \
	'#!/bin/sh' \
	'set -u' \
	': "${FAKE_ANTIGRAVITY_IDE_MARKER:?}"' \
	': "${FAKE_ANTIGRAVITY_IDE_ARGUMENTS:?}"' \
	': "${FAKE_ANTIGRAVITY_CHALLENGE:?}"' \
	': "${FAKE_EXPECTED_HOME:?}"' \
	': "${ANTIGRAVITY_IDE_DATA_ROOT:?}"' \
	': "${ANTIGRAVITY_IDE_VERSION:?}"' \
	'if [ -n "${VSCODE_IPC_HOOK_CLI:-}" ] || [ "$HOME" != "$FAKE_EXPECTED_HOME" ]; then' \
	'  exit 3' \
	'fi' \
	'printf "invoked\n" >"$FAKE_ANTIGRAVITY_IDE_MARKER"' \
	'printf "%s\n" "$@" >"$FAKE_ANTIGRAVITY_IDE_ARGUMENTS"' \
	'transcript_directory="$ANTIGRAVITY_IDE_DATA_ROOT/brain/fake/.system_generated/logs"' \
	'mkdir -p "$transcript_directory"' \
	'printf "%s\n" \' \
	'  "{\"step_index\":1,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"status\":\"DONE\",\"content\":\"/sth-runtime-command STH_RUNTIME_CHALLENGE_$FAKE_ANTIGRAVITY_CHALLENGE ANTIGRAVITY_IDE_VERSION_$ANTIGRAVITY_IDE_VERSION\",\"tool_calls\":[]}" \' \
	'  '\''{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_COMMAND_UPDATED","tool_calls":[]}'\'' \' \
	'  "{\"step_index\":3,\"source\":\"USER_EXPLICIT\",\"type\":\"USER_INPUT\",\"status\":\"DONE\",\"content\":\"Return the STH instruction sentinel. STH_RUNTIME_CHALLENGE_$FAKE_ANTIGRAVITY_CHALLENGE ANTIGRAVITY_IDE_VERSION_$ANTIGRAVITY_IDE_VERSION\",\"tool_calls\":[]}" \' \
	'  '\''{"step_index":4,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","content":"ANTIGRAVITY_INSTRUCTION_UPDATED","tool_calls":[]}'\'' \' \
	'  >"$transcript_directory/transcript_full.jsonl"' \
	'if [ "${FAKE_ANTIGRAVITY_DUPLICATE:-0}" = "1" ]; then' \
	'  duplicate_directory="$ANTIGRAVITY_IDE_DATA_ROOT/brain/duplicate/.system_generated/logs"' \
	'  mkdir -p "$duplicate_directory"' \
	'  cp "$transcript_directory/transcript_full.jsonl" "$duplicate_directory/transcript_full.jsonl"' \
	'fi' \
	>"$strict_antigravity_application_cli"
chmod 755 "$strict_antigravity_application/Contents/MacOS/Electron" "$strict_antigravity_application_cli"
if ! sh -n "$strict_antigravity_application_cli"; then
	fail_test "faux CLI Antigravity IDE: syntaxe invalide"
fi

fake_security_tools="$test_directory/fake-security-tools"
mkdir -p "$fake_security_tools"
printf '%s\n' \
	'#!/bin/sh' \
	'printf "Darwin\n"' \
	>"$fake_security_tools/uname"
printf '%s\n' \
	'#!/bin/sh' \
	'case "$1" in' \
	'  --verify) exit 0 ;;' \
	'  -dv) printf "TeamIdentifier=%s\n" "${FAKE_TEAM_IDENTIFIER:-EQHXZ8M8AV}" >&2; exit 0 ;;' \
	'esac' \
	'exit 1' \
	>"$fake_security_tools/codesign"
printf '%s\n' \
	'#!/bin/sh' \
	'exit 0' \
	>"$fake_security_tools/spctl"
chmod 755 "$fake_security_tools/uname" "$fake_security_tools/codesign" "$fake_security_tools/spctl"
antigravity_application_proof_output=$(PATH="$fake_security_tools:$PATH" sh "$antigravity_ide_app_proof" "$strict_antigravity_application" test 0)
antigravity_application_proof_status=$?
assert_status 0 "$antigravity_application_proof_status" "preuve app Antigravity avec TeamIdentifier Google"
if [ "$antigravity_application_proof_output" != "ANTIGRAVITY_IDE_APP_OK" ]; then
	fail_test "preuve app Antigravity: résultat inattendu"
fi
FAKE_TEAM_IDENTIFIER=INVALID PATH="$fake_security_tools:$PATH" sh "$antigravity_ide_app_proof" "$strict_antigravity_application" test 0 >"$invalid_output" 2>"$invalid_error"
antigravity_application_proof_status=$?
assert_status 1 "$antigravity_application_proof_status" "preuve app Antigravity avec TeamIdentifier tiers"

gemini_runtime_identity_binary="$test_directory/gemini-runtime-identity-cli"
printf '%s\n' \
	'#!/bin/sh' \
	'if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then' \
	'  [ -n "${GEMINI_CLI_HOME:-}" ] && [ -d "$GEMINI_CLI_HOME" ] || exit 4' \
	'  [ -n "${CLOUDSDK_CONFIG:-}" ] && [ -d "$CLOUDSDK_CONFIG" ] || exit 5' \
	'  printf "runtime identity 1.2.3\n"' \
	'  exit 0' \
	'fi' \
	'exit 1' \
	>"$gemini_runtime_identity_binary"
chmod 755 "$gemini_runtime_identity_binary"
gemini_runtime_identity_sha256=$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$gemini_runtime_identity_binary") || fail_test "preuve identité Gemini: calcul SHA-256 impossible"

strict_real_identity_output="$test_directory/strict-real-identity.tsv"
strict_real_identity_error="$test_directory/strict-real-identity.err"
PATH="$fake_security_tools:$PATH" \
CLAUDE_BIN="$runtime_identity_binary" \
CODEX_BIN="$runtime_identity_binary" \
COPILOT_BIN="$runtime_identity_binary" \
GEMINI_BIN="$gemini_runtime_identity_binary" \
ANTIGRAVITY_BIN="$runtime_identity_binary" \
CLAUDE_COMPAT_EXPECTED_VERSION=1.2.3 \
CLAUDE_COMPAT_EXPECTED_SHA256="$runtime_identity_sha256" \
CODEX_COMPAT_EXPECTED_VERSION=1.2.3 \
CODEX_COMPAT_EXPECTED_SHA256="$runtime_identity_sha256" \
COPILOT_COMPAT_EXPECTED_VERSION=1.2.3 \
COPILOT_COMPAT_EXPECTED_SHA256="$runtime_identity_sha256" \
GEMINI_COMPAT_EXPECTED_VERSION=1.2.3 \
GEMINI_COMPAT_EXPECTED_SHA256="$gemini_runtime_identity_sha256" \
ANTIGRAVITY_COMPAT_EXPECTED_VERSION=1.2.3 \
ANTIGRAVITY_COMPAT_EXPECTED_SHA256="$runtime_identity_sha256" \
COPILOT_IDE_TRANSCRIPT="$strict_copilot_transcript" \
COPILOT_IDE_EXTENSION_VERSION=test \
ANTIGRAVITY_IDE_TRANSCRIPT="$strict_antigravity_transcript" \
ANTIGRAVITY_IDE_VERSION=test \
ANTIGRAVITY_IDE_APP="$strict_antigravity_application" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=0 \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$strict_real_identity_output" 2>"$strict_real_identity_error"
strict_real_identity_status=$?
assert_status 1 "$strict_real_identity_status" "certification stricte réelle avec identités CLI valides mais smokes non certifiants"
for expected_runtime_identity in \
	"Identité runtime vérifiée: Claude version=1.2.3 sha256=$runtime_identity_sha256" \
	"Identité runtime vérifiée: Codex version=1.2.3 sha256=$runtime_identity_sha256" \
	"Identité runtime vérifiée: Copilot CLI version=1.2.3 sha256=$runtime_identity_sha256" \
	"Identité runtime vérifiée: Gemini version=1.2.3 sha256=$gemini_runtime_identity_sha256" \
	"Identité runtime vérifiée: Antigravity CLI version=1.2.3 sha256=$runtime_identity_sha256"; do
	if ! grep -Fxq "$expected_runtime_identity" "$strict_real_identity_error"; then
		fail_test "certification stricte réelle: attestation identité absente: $expected_runtime_identity"
	fi
done
strict_real_identity_count=$(grep -Fc 'Identité runtime vérifiée:' "$strict_real_identity_error")
if [ "$strict_real_identity_count" -ne 5 ]; then
	fail_test "certification stricte réelle: $strict_real_identity_count attestations identité, attendu 5"
fi

antigravity_ide_tmp="$test_directory/antigravity-ide-tmp"
antigravity_ide_data_root="$test_directory/antigravity-ide-data"
antigravity_ide_marker="$test_directory/fake-antigravity-ide.marker"
antigravity_ide_arguments="$test_directory/fake-antigravity-ide-arguments.txt"
mkdir -p "$antigravity_ide_tmp" "$antigravity_ide_data_root"
antigravity_ide_tmp=$(CDPATH= cd -- "$antigravity_ide_tmp" && pwd)
antigravity_ide_data_root=$(CDPATH= cd -- "$antigravity_ide_data_root" && pwd -P)
strict_antigravity_project=$(CDPATH= cd -- "$strict_fixture_root/antigravity-project" && pwd)
stale_antigravity_transcript="$antigravity_ide_data_root/brain/stale/.system_generated/logs/transcript_full.jsonl"
mkdir -p "$(dirname -- "$stale_antigravity_transcript")"
printf '{}\n' >"$stale_antigravity_transcript"
touch -t 200001010000 "$stale_antigravity_transcript"

antigravity_ide_prepare_output="$test_directory/antigravity-ide-prepare.out"
antigravity_ide_prepare_error="$test_directory/antigravity-ide-prepare.err"
PATH="$fake_security_tools:$PATH" \
TMPDIR="$antigravity_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
ANTIGRAVITY_IDE_APP="$strict_antigravity_application" \
ANTIGRAVITY_IDE_VERSION=test \
ANTIGRAVITY_IDE_DATA_ROOT="$antigravity_ide_data_root" \
COMPAT_HERMETIC=1 \
	sh "$antigravity_ide_launch" --prepare-only >"$antigravity_ide_prepare_output" 2>"$antigravity_ide_prepare_error"
antigravity_ide_prepare_status=$?
assert_status 0 "$antigravity_ide_prepare_status" "préparation isolée Antigravity IDE sans lancement"
if [ -e "$antigravity_ide_marker" ]; then
	fail_test "préparation isolée Antigravity IDE: le CLI application a été lancé"
fi
if ! grep -Fxq "Challenge runtime IDE : $strict_runtime_challenge" "$antigravity_ide_prepare_output"; then
	fail_test "préparation isolée Antigravity IDE: challenge frais absent"
fi
if ! grep -Fxq "Requête workflow Antigravity IDE : /sth-runtime-command STH_RUNTIME_CHALLENGE_$strict_runtime_challenge ANTIGRAVITY_IDE_VERSION_test" "$antigravity_ide_prepare_output"; then
	fail_test "préparation isolée Antigravity IDE: requête workflow exacte absente"
fi
if ! grep -Fxq "Requête règle Antigravity IDE : Return the STH instruction sentinel. STH_RUNTIME_CHALLENGE_$strict_runtime_challenge ANTIGRAVITY_IDE_VERSION_test" "$antigravity_ide_prepare_output"; then
	fail_test "préparation isolée Antigravity IDE: requête règle exacte absente"
fi
antigravity_ide_prepare_root=$(sed -n 's/^Session Antigravity IDE : //p' "$antigravity_ide_prepare_output")
case "$antigravity_ide_prepare_root" in
	"$antigravity_ide_tmp"/sth-antigravity-ide.*)
		;;
	*)
		fail_test "préparation isolée Antigravity IDE: racine temporaire inattendue"
		;;
esac
if [ ! -d "$antigravity_ide_prepare_root/user-data" ] || [ ! -d "$antigravity_ide_prepare_root/extensions" ]; then
	fail_test "préparation isolée Antigravity IDE: dossiers isolés absents"
fi
for expected_metadata in \
	'compat_mode	hermetic' \
	'fixture_proof	FIXTURE_ROOT_OK' \
	'application_proof	ANTIGRAVITY_IDE_APP_OK' \
	'application_status	not-run'; do
	if ! grep -Fxq "$expected_metadata" "$antigravity_ide_prepare_root/session-info.tsv"; then
		fail_test "préparation isolée Antigravity IDE: métadonnée absente: $expected_metadata"
	fi
done

antigravity_ide_launch_output="$test_directory/antigravity-ide-launch.out"
antigravity_ide_launch_error="$test_directory/antigravity-ide-launch.err"
PATH="$fake_security_tools:$PATH" \
TMPDIR="$antigravity_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
ANTIGRAVITY_IDE_APP="$strict_antigravity_application" \
ANTIGRAVITY_IDE_VERSION=test \
ANTIGRAVITY_IDE_DATA_ROOT="$antigravity_ide_data_root" \
FAKE_ANTIGRAVITY_IDE_MARKER="$antigravity_ide_marker" \
FAKE_ANTIGRAVITY_IDE_ARGUMENTS="$antigravity_ide_arguments" \
FAKE_ANTIGRAVITY_CHALLENGE="$strict_runtime_challenge" \
FAKE_EXPECTED_HOME="$HOME" \
VSCODE_IPC_HOOK_CLI="$test_directory/unexpected-antigravity-ipc-hook" \
	sh "$antigravity_ide_launch" >"$antigravity_ide_launch_output" 2>"$antigravity_ide_launch_error"
antigravity_ide_launch_status=$?
if [ "$antigravity_ide_launch_status" -ne 0 ]; then
	sed -n '1,160p' "$antigravity_ide_launch_output" >&2
	sed -n '1,160p' "$antigravity_ide_launch_error" >&2
fi
assert_status 0 "$antigravity_ide_launch_status" "lancement isolé Antigravity IDE avec faux CLI"
if [ ! -f "$antigravity_ide_marker" ] || [ ! -f "$antigravity_ide_arguments" ]; then
	fail_test "lancement isolé Antigravity IDE: faux CLI non exécuté"
fi
antigravity_ide_launch_root=$(sed -n 's/^Session Antigravity IDE : //p' "$antigravity_ide_launch_output")
antigravity_ide_transcript=$(sed -n 's/^Transcript Antigravity IDE : //p' "$antigravity_ide_launch_output")
expected_antigravity_transcript="$antigravity_ide_data_root/brain/fake/.system_generated/logs/transcript_full.jsonl"
if [ "$antigravity_ide_transcript" != "$expected_antigravity_transcript" ] || [ ! -f "$antigravity_ide_transcript" ]; then
	printf 'Transcript Antigravity IDE reçu : %s\n' "$antigravity_ide_transcript" >&2
	printf 'Transcript Antigravity IDE attendu : %s\n' "$expected_antigravity_transcript" >&2
	fail_test "lancement isolé Antigravity IDE: transcript complet frais inattendu"
fi
if ! grep -Fxq 'Preuve Antigravity IDE : ANTIGRAVITY_IDE_OK' "$antigravity_ide_launch_output"; then
	fail_test "lancement isolé Antigravity IDE: preuve automatique absente"
fi
expected_antigravity_transcript_entry=$(printf 'transcript\t%s' "$antigravity_ide_transcript")
expected_antigravity_proof_entry=$(printf 'proof\tANTIGRAVITY_IDE_OK')
if ! grep -Fxq "$expected_antigravity_transcript_entry" "$antigravity_ide_launch_root/session-info.tsv" || \
	! grep -Fxq "$expected_antigravity_proof_entry" "$antigravity_ide_launch_root/session-info.tsv"; then
	fail_test "lancement isolé Antigravity IDE: transcript ou preuve absent des métadonnées"
fi
if ! python3 - "$antigravity_ide_arguments" "$antigravity_ide_launch_root" "$strict_antigravity_project" <<'PY'
import sys

arguments_path, session_root, project_directory = sys.argv[1:]
with open(arguments_path, encoding="utf-8") as arguments_stream:
    arguments = arguments_stream.read().splitlines()
expected_arguments = [
    "--new-window",
    "--wait",
    "--user-data-dir",
    f"{session_root}/user-data",
    "--extensions-dir",
    f"{session_root}/extensions",
    "--sync",
    "off",
    "--use-inmemory-secretstorage",
    project_directory,
]
if arguments != expected_arguments:
    raise SystemExit(f"arguments Antigravity IDE inattendus: {arguments!r}")
PY
then
	fail_test "lancement isolé Antigravity IDE: arguments reçus invalides"
fi

antigravity_ide_duplicate_output="$test_directory/antigravity-ide-duplicate.out"
antigravity_ide_duplicate_error="$test_directory/antigravity-ide-duplicate.err"
PATH="$fake_security_tools:$PATH" \
TMPDIR="$antigravity_ide_tmp" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
ANTIGRAVITY_IDE_APP="$strict_antigravity_application" \
ANTIGRAVITY_IDE_VERSION=test \
ANTIGRAVITY_IDE_DATA_ROOT="$antigravity_ide_data_root" \
FAKE_ANTIGRAVITY_IDE_MARKER="$antigravity_ide_marker" \
FAKE_ANTIGRAVITY_IDE_ARGUMENTS="$antigravity_ide_arguments" \
FAKE_ANTIGRAVITY_CHALLENGE="$strict_runtime_challenge" \
FAKE_EXPECTED_HOME="$HOME" \
FAKE_ANTIGRAVITY_DUPLICATE=1 \
	sh "$antigravity_ide_launch" >"$antigravity_ide_duplicate_output" 2>"$antigravity_ide_duplicate_error"
antigravity_ide_duplicate_status=$?
assert_status 1 "$antigravity_ide_duplicate_status" "lancement Antigravity IDE avec deux transcripts frais"

strict_claude_model_calls="$test_directory/strict-claude-model-calls.txt"
strict_claude_discovery_calls="$test_directory/strict-claude-discovery-calls.txt"
strict_codex_model_calls="$test_directory/strict-codex-model-calls.txt"
strict_codex_agent_calls="$test_directory/strict-codex-agent-calls.txt"
strict_copilot_model_calls="$test_directory/strict-copilot-model-calls.txt"
strict_gemini_model_calls="$test_directory/strict-gemini-model-calls.txt"
strict_antigravity_model_calls="$test_directory/strict-antigravity-model-calls.txt"
strict_output="$test_directory/strict-success.tsv"
strict_error="$test_directory/strict-success.err"
CLAUDE_BIN="$fake_claude" \
CODEX_BIN="$fake_codex_agent_transient" \
COPILOT_BIN="$fake_copilot_success" \
GEMINI_BIN="$fake_gemini_success" \
ANTIGRAVITY_BIN="$fake_antigravity_success" \
FAKE_CLAUDE_MODEL_CALLS="$strict_claude_model_calls" \
FAKE_CLAUDE_DISCOVERY_CALLS="$strict_claude_discovery_calls" \
FAKE_CODEX_MODEL_CALLS="$strict_codex_model_calls" \
FAKE_CODEX_AGENT_CALLS="$strict_codex_agent_calls" \
FAKE_COPILOT_MODEL_CALLS="$strict_copilot_model_calls" \
FAKE_GEMINI_MODEL_CALLS="$strict_gemini_model_calls" \
FAKE_ANTIGRAVITY_MODEL_CALLS="$strict_antigravity_model_calls" \
GEMINI_COMPAT_HOME= \
COPILOT_IDE_TRANSCRIPT="$strict_copilot_transcript" \
ANTIGRAVITY_IDE_TRANSCRIPT="$strict_antigravity_transcript" \
COPILOT_IDE_EXTENSION_VERSION=test \
ANTIGRAVITY_IDE_VERSION=test \
ANTIGRAVITY_IDE_APP="$strict_antigravity_application" \
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
COMPAT_HERMETIC=1 \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$strict_output" 2>"$strict_error"
strict_status=$?
if [ "$strict_status" -ne 0 ]; then
	sed -n '1,200p' "$strict_output" >&2
	sed -n '1,200p' "$strict_error" >&2
	fail_test "certification stricte hermétique: statut $strict_status, attendu 0"
fi
if ! awk 'END { if (NR != 39) exit 1 }' "$strict_output"; then
	fail_test "certification stricte hermétique: un en-tête et 38 preuves attendus"
fi
if ! awk -F '\t' 'NR == 1 && $5 != "mode" { invalid = 1 } NR > 1 && ($5 != "hermetic" || NF != 5) { invalid = 1 } END { exit invalid ? 1 : 0 }' "$strict_output"; then
	fail_test "certification stricte hermétique: attestation de mode absente"
fi
if awk 'NR > 1 && ($1 == "KO" || $1 == "NON_EXECUTE") { found = 1 } END { exit found ? 0 : 1 }' "$strict_output"; then
	fail_test "certification stricte hermétique: statut non certifiant présent"
fi
strict_results="$test_directory/strict-success-results.tsv"
awk 'NR > 1 { print $1 "\t" $2 "\t" $3 }' "$strict_output" >"$strict_results"
if ! sh "$repository_root/scripts/compat-runtime-matrix-proof.sh" "$expected_strict_contracts" "$strict_results" >/dev/null; then
	fail_test "certification stricte hermétique: matrice 38/38 invalide"
fi

for call_contract in \
	"$strict_claude_discovery_calls:1" \
	"$strict_claude_model_calls:4" \
	"$strict_codex_model_calls:5" \
	"$strict_codex_agent_calls:2" \
	"$strict_copilot_model_calls:4" \
	"$strict_gemini_model_calls:4" \
	"$strict_antigravity_model_calls:4"; do
	call_file=${call_contract%:*}
	expected_calls=${call_contract##*:}
	actual_calls=$(awk 'END { print NR }' "$call_file")
	if [ "$actual_calls" -ne "$expected_calls" ]; then
		fail_test "certification stricte hermétique: $actual_calls appels dans $call_file, attendu $expected_calls"
	fi
done

COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN=/bin/sh \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "certification stricte avec binaire STH sans correspondance de provenance"

strict_provenance="$strict_fixture_root/.sth-compat-provenance.json"
strict_provenance_backup="$test_directory/strict-provenance-backup.json"
cp "$strict_provenance" "$strict_provenance_backup"
python3 - "$strict_provenance" <<'PY'
import json
import sys

provenance_path = sys.argv[1]
with open(provenance_path, encoding="utf-8") as provenance_stream:
    provenance = json.load(provenance_stream)
provenance["generated_at"] = "2000-01-01T00:00:00Z"
with open(provenance_path, "w", encoding="utf-8") as provenance_stream:
    json.dump(provenance, provenance_stream, indent=2, sort_keys=True)
    provenance_stream.write("\n")
PY
COMPAT_FIXTURE_ROOT="$strict_fixture_root" \
COMPAT_STH_BIN="$strict_sth_binary" \
STH_RUNTIME_SENTINEL_SUFFIX=UPDATED \
STRICT_RUNTIME_CERTIFICATION=1 \
RUN_MODEL_SMOKES=1 \
	sh "$runner" >"$invalid_output" 2>"$invalid_error"
invalid_status=$?
assert_status 2 "$invalid_status" "certification stricte avec provenance expirée"
cp "$strict_provenance_backup" "$strict_provenance"

printf 'OK: certification hermétique du runner\n'

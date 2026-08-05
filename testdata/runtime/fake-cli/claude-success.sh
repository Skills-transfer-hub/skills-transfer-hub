#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}

has_argument() {
	expected_argument=$1
	shift
	for actual_argument in "$@"; do
		if [ "$actual_argument" = "$expected_argument" ]; then
			return 0
		fi
	done
	return 1
}

has_argument_pair() {
	expected_name=$1
	expected_value=$2
	shift 2
	while [ "$#" -gt 0 ]; do
		if [ "$1" = "$expected_name" ] && [ "${2:-}" = "$expected_value" ]; then
			return 0
		fi
		shift
	done
	return 1
}

validate_fixture() {
	for fixture_path in \
		.claude/agents/sth-runtime-agent.md \
		.claude/commands/sth-runtime-command.md \
		.claude/rules/sth-runtime-rule.md \
		.claude/skills/sth-runtime-skill/SKILL.md; do
		[ -f "$fixture_path" ] || exit 1
	done
	grep -Fq "CLAUDE_AGENT_$sentinel_suffix" .claude/agents/sth-runtime-agent.md || exit 1
	grep -Fq "CLAUDE_COMMAND_$sentinel_suffix" .claude/commands/sth-runtime-command.md || exit 1
	grep -Fq "CLAUDE_INSTRUCTION_$sentinel_suffix" .claude/rules/sth-runtime-rule.md || exit 1
	grep -Fq "CLAUDE_SKILL_$sentinel_suffix" .claude/skills/sth-runtime-skill/SKILL.md || exit 1
}

if [ "${1:-}" = "auth" ] && [ "${2:-}" = "status" ] && [ "${3:-}" = "--json" ]; then
	printf '{"loggedIn":true}\n'
	exit 0
fi

if has_argument_pair -p /help "$@" && has_argument_pair --output-format stream-json "$@"; then
	validate_fixture
	has_argument --include-hook-events "$@" || exit 1
	has_argument --verbose "$@" || exit 1
	has_argument_pair --setting-sources project "$@" || exit 1
	has_argument --no-session-persistence "$@" || exit 1
	has_argument --settings "$@" || exit 1
	: "${FAKE_CLAUDE_DISCOVERY_CALLS:?}"
	: "${STH_CLAUDE_INSTRUCTIONS_FILE:?}"
	printf 'discovery\n' >>"$FAKE_CLAUDE_DISCOVERY_CALLS"
	printf '{"hook_event_name":"InstructionsLoaded","file_path":"%s/.claude/rules/sth-runtime-rule.md","load_reason":"session_start","memory_type":"Project"}\n' "$PWD" >"$STH_CLAUDE_INSTRUCTIONS_FILE"
	printf '{"type":"system","subtype":"init","agents":["sth-runtime-agent"],"slash_commands":["sth-runtime-skill","sth-runtime-command"],"skills":["sth-runtime-skill"]}\n'
	printf '{"type":"result","subtype":"success","duration_api_ms":0,"num_turns":0,"total_cost_usd":0,"usage":{"input_tokens":0,"output_tokens":0},"modelUsage":{}}\n'
	exit 0
fi

validate_fixture
: "${FAKE_CLAUDE_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_CLAUDE_MODEL_CALLS"
printf 'Claude diagnostic on stderr\n' >&2

if has_argument_pair -p /sth-runtime-skill "$@"; then
	printf '{"result":"CLAUDE_SKILL_%s"}\n' "$sentinel_suffix"
	exit 0
fi
if has_argument_pair --agent sth-runtime-agent "$@"; then
	printf '{"result":"CLAUDE_AGENT_%s"}\n' "$sentinel_suffix"
	exit 0
fi
if has_argument_pair -p /sth-runtime-command "$@"; then
	printf '{"result":"CLAUDE_COMMAND_%s"}\n' "$sentinel_suffix"
	exit 0
fi
if has_argument_pair -p 'Return the STH instruction sentinel.' "$@"; then
	printf '{"result":"CLAUDE_INSTRUCTION_%s"}\n' "$sentinel_suffix"
	exit 0
fi

printf 'Unexpected Claude invocation\n' >&2
exit 1

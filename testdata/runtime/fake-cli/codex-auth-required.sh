#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}

if [ "${1:-}" = "debug" ] && [ "${2:-}" = "prompt-input" ]; then
	for fixture_path in \
		.agents/skills/sth-runtime-skill/SKILL.md \
		.agents/skills/sth-runtime-command/SKILL.md \
		.codex/agents/sth-runtime-agent.toml \
		.codex/sth/instructions/sth-runtime-instruction.md \
		AGENTS.md; do
		[ -f "$fixture_path" ] || exit 1
	done
	grep -Fq "CODEX_SKILL_$sentinel_suffix" .agents/skills/sth-runtime-skill/SKILL.md || exit 1
	grep -Fq "CODEX_COMMAND_$sentinel_suffix" .agents/skills/sth-runtime-command/SKILL.md || exit 1
	grep -Fq "CODEX_AGENT_$sentinel_suffix" .codex/agents/sth-runtime-agent.toml || exit 1
	grep -Fq "CODEX_INSTRUCTION_$sentinel_suffix" .codex/sth/instructions/sth-runtime-instruction.md || exit 1
	grep -Fq '<!-- sth:managed:codex-instructions-v1:start -->' AGENTS.md || exit 1
	grep -Fq "CODEX_INSTRUCTION_$sentinel_suffix" AGENTS.md || exit 1
	printf 'sth-runtime-skill\n'
	printf 'sth-runtime-command\n'
	cat AGENTS.md
	exit 0
fi

: "${FAKE_CODEX_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_CODEX_MODEL_CALLS"
printf 'Not logged in\n' >&2
exit 1

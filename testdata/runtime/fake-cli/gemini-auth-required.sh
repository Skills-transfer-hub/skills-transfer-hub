#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}
fake_cli_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$fake_cli_directory/gemini-runtime-contract.sh"

validate_gemini_runtime_environment || exit 2

if gemini_skills_list_requested "$@"; then
	for fixture_path in \
		.agents/skills/sth-runtime-skill/SKILL.md \
		.gemini/agents/sth-runtime-agent.md \
		.gemini/commands/sth-runtime-command.toml \
		.gemini/sth/instructions/sth-runtime-instruction.md \
		GEMINI.md; do
		[ -f "$fixture_path" ] || exit 1
	done
	grep -Fq "GEMINI_SKILL_$sentinel_suffix" .agents/skills/sth-runtime-skill/SKILL.md || exit 1
	grep -Fq "GEMINI_AGENT_$sentinel_suffix" .gemini/agents/sth-runtime-agent.md || exit 1
	grep -Fq "GEMINI_COMMAND_$sentinel_suffix" .gemini/commands/sth-runtime-command.toml || exit 1
	grep -Fq "GEMINI_INSTRUCTION_$sentinel_suffix" .gemini/sth/instructions/sth-runtime-instruction.md || exit 1
	grep -Fq '@./.gemini/sth/instructions/sth-runtime-instruction.md' GEMINI.md || exit 1
	printf 'sth-runtime-skill\n'
	exit 0
fi

validate_gemini_isolated_arguments "$@" || exit 2
validate_gemini_model_arguments "$@" || exit 2
: "${FAKE_GEMINI_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_GEMINI_MODEL_CALLS"
printf 'Authentication required\n' >&2
exit 1

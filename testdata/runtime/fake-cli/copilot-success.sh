#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}
arguments=$*
project_directory=
prompt=
selected_agent=

while [ "$#" -gt 0 ]; do
	case "$1" in
		-C)
			shift
			project_directory=${1:-}
			;;
		-p)
			shift
			prompt=${1:-}
			;;
		--agent)
			shift
			selected_agent=${1:-}
			;;
	esac
	shift
done

[ -n "$project_directory" ] || exit 2
cd "$project_directory" || exit 2

for fixture_path in \
	.agents/skills/sth-runtime-skill/SKILL.md \
	.github/agents/sth-runtime-agent.agent.md \
	.github/prompts/sth-runtime-command.prompt.md \
	.github/sth/instructions/sth-runtime.md \
	AGENTS.md; do
	[ -f "$fixture_path" ] || exit 2
done
grep -Fq "COPILOT_SKILL_$sentinel_suffix" .agents/skills/sth-runtime-skill/SKILL.md || exit 2
grep -Fq "COPILOT_AGENT_$sentinel_suffix" .github/agents/sth-runtime-agent.agent.md || exit 2
grep -Fq "COPILOT_COMMAND_$sentinel_suffix" .github/prompts/sth-runtime-command.prompt.md || exit 2
grep -Fq "COPILOT_INSTRUCTION_$sentinel_suffix" AGENTS.md || exit 2

case " $arguments " in
	*" skill list --json "*)
		printf '[{"name":"sth-runtime-skill"}]\n'
		exit 0
		;;
esac

: "${FAKE_COPILOT_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_COPILOT_MODEL_CALLS"
printf 'Copilot diagnostic on stderr\n' >&2

case "$prompt" in
	/sth-runtime-skill)
		printf 'COPILOT_SKILL_%s\n' "$sentinel_suffix"
		;;
	/sth-runtime-command)
		printf 'COPILOT_COMMAND_%s\n' "$sentinel_suffix"
		;;
	'Return the agent sentinel.')
		[ "$selected_agent" = "sth-runtime-agent" ] || exit 2
		printf 'COPILOT_AGENT_%s\n' "$sentinel_suffix"
		;;
	'Use @sth-runtime-instruction-target.txt as context. Return the STH instruction sentinel.')
		printf 'COPILOT_INSTRUCTION_%s\n' "$sentinel_suffix"
		;;
	*)
		exit 2
		;;
esac

#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}

: "${FAKE_ANTIGRAVITY_MODEL_CALLS:?}"

prompt=
selected_agent=
output_format=
added_directory=
log_file=
subcommand=
while [ "$#" -gt 0 ]; do
	case "$1" in
		-p|--print|--prompt)
			shift
			prompt=${1:-}
			;;
		--agent)
			shift
			selected_agent=${1:-}
			;;
		--output-format)
			shift
			output_format=${1:-}
			;;
		--add-dir)
			shift
			added_directory=${1:-}
			;;
		--log-file)
			shift
			log_file=${1:-}
			;;
		agents)
			subcommand=agents
			;;
	esac
	shift
done

canonical_added_directory=$(CDPATH= cd -- "$added_directory" && pwd) || exit 2
[ "$canonical_added_directory" = "$(pwd)" ] || exit 2
[ -f .agents/skills/sth-runtime-skill/SKILL.md ] || exit 2
[ -f .agents/agents/sth-runtime-agent.md ] || exit 2
[ -f .agents/workflows/sth-runtime-command.md ] || exit 2
[ -f .agents/rules/sth-runtime-rule.md ] || exit 2

if [ "$subcommand" = "agents" ]; then
	printf 'Available agents:\n  sth-runtime-agent\n'
	exit 0
fi

[ "$output_format" = "json" ] || exit 2
printf 'model\n' >>"$FAKE_ANTIGRAVITY_MODEL_CALLS"
if [ -n "$log_file" ]; then
	printf 'Antigravity diagnostic log\n' >"$log_file"
fi
printf 'Antigravity diagnostic on stderr\n' >&2

case "$prompt" in
	/sth-runtime-skill)
		printf '{"status":"SUCCESS","response":"ANTIGRAVITY_SKILL_%s"}\n' "$sentinel_suffix"
		;;
	/sth-runtime-command)
		printf '{"status":"SUCCESS","response":"ANTIGRAVITY_COMMAND_%s"}\n' "$sentinel_suffix"
		;;
	'Return the STH instruction sentinel.')
		printf '{"status":"SUCCESS","response":"ANTIGRAVITY_INSTRUCTION_%s"}\n' "$sentinel_suffix"
		;;
	*)
		[ "$selected_agent" = "sth-runtime-agent" ] || exit 2
		printf '{"status":"SUCCESS","agent":"sth-runtime-agent","response":"ANTIGRAVITY_AGENT_%s"}\n' "$sentinel_suffix"
		;;
esac

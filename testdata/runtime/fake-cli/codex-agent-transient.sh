#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}

if [ "${1:-}" = "debug" ] && [ "${2:-}" = "prompt-input" ]; then
	printf 'sth-runtime-skill\n'
	printf 'sth-runtime-command\n'
	cat AGENTS.md
	exit 0
fi

: "${FAKE_CODEX_MODEL_CALLS:?}"
: "${FAKE_CODEX_AGENT_CALLS:?}"

prompt=
for argument in "$@"; do
	prompt=$argument
done

printf 'model\n' >>"$FAKE_CODEX_MODEL_CALLS"
printf 'Codex diagnostic on stderr\n' >&2
case "$prompt" in
	*sth-runtime-skill*)
		printf '{"type":"item.completed","item":{"type":"agent_message","text":"CODEX_SKILL_%s"}}\n' "$sentinel_suffix"
		;;
	*sth-runtime-agent*)
		printf 'agent\n' >>"$FAKE_CODEX_AGENT_CALLS"
		agent_attempts=$(awk 'END { print NR }' "$FAKE_CODEX_AGENT_CALLS")
		if [ "$agent_attempts" -eq 1 ]; then
			printf 'collab spawn failed: no thread with id transient\n' >&2
			exit 1
		fi
		printf '{"type":"item.completed","item":{"type":"agent_message","text":"CODEX_AGENT_%s"}}\n' "$sentinel_suffix"
		;;
	*sth-runtime-command*)
		printf '{"type":"item.completed","item":{"type":"agent_message","text":"CODEX_COMMAND_%s"}}\n' "$sentinel_suffix"
		;;
	*)
		printf '{"type":"item.completed","item":{"type":"agent_message","text":"CODEX_INSTRUCTION_%s"}}\n' "$sentinel_suffix"
		;;
esac

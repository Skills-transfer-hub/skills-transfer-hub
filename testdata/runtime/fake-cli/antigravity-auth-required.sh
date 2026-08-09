#!/bin/sh

set -u
sentinel_suffix=${STH_RUNTIME_SENTINEL_SUFFIX:-OK}
prompt=
log_file=
subcommand=

while [ "$#" -gt 0 ]; do
	case "$1" in
		-p|--print|--prompt)
			shift
			prompt=${1:-}
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

if [ "$subcommand" = "agents" ]; then
	printf 'Available agents:\n  sth-runtime-agent\n'
	exit 0
fi

: "${FAKE_ANTIGRAVITY_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_ANTIGRAVITY_MODEL_CALLS"
if [ -n "$log_file" ]; then
	printf 'Failed to get OAuth token: error getting token source: You are not logged into Antigravity.\n' >"$log_file"
else
	printf 'Failed to get OAuth token: state syncing error: key not found\n' >&2
fi
[ -n "$prompt" ] || exit 2
exit 1

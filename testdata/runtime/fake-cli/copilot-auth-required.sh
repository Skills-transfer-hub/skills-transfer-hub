#!/bin/sh

set -u

case " $* " in
	*" skill list --json "*)
		printf '[{"name":"sth-runtime-skill"},{"name":"sth-runtime-command"}]\n'
		exit 0
		;;
	*" plugins list --kind instruction --json "*)
		printf 'The plugins command is not available.\n' >&2
		exit 1
		;;
esac

: "${FAKE_COPILOT_MODEL_CALLS:?}"
printf 'model\n' >>"$FAKE_COPILOT_MODEL_CALLS"
printf 'No authentication information found.\n' >&2
exit 1

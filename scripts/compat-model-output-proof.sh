#!/bin/sh

set -u

runtime=${1:-}
output_file=${2:-}
expected_sentinel=${3:-}

if [ -z "$runtime" ] || [ ! -f "$output_file" ] || [ -z "$expected_sentinel" ]; then
	printf 'MODEL_OUTPUT_KO\n'
	exit 1
fi

python3 - "$runtime" "$output_file" "$expected_sentinel" <<'PY'
import json
import sys

runtime, output_path, expected_sentinel = sys.argv[1:]

with open(output_path, encoding="utf-8") as output_stream:
    output_text = output_stream.read()

def exact(value):
    return isinstance(value, str) and value.strip() == expected_sentinel

valid = False

if runtime == "copilot":
    valid = output_text.strip() == expected_sentinel
elif runtime in {"claude", "gemini", "antigravity"}:
    try:
        output = json.loads(output_text)
    except json.JSONDecodeError:
        output = None
    if isinstance(output, dict):
        if runtime == "claude":
            valid = exact(output.get("result"))
        elif runtime == "gemini":
            valid = exact(output.get("response"))
        else:
            valid = output.get("status") == "SUCCESS" and exact(output.get("response"))
elif runtime == "codex":
    final_messages = []
    try:
        for line_number, output_line in enumerate(output_text.splitlines(), 1):
            if not output_line.strip():
                continue
            event = json.loads(output_line)
            if (
                event.get("type") == "item.completed"
                and event.get("item", {}).get("type") == "agent_message"
            ):
                final_messages.append(event.get("item", {}).get("text"))
    except json.JSONDecodeError as err:
        raise SystemExit(f"JSONL Codex invalide ligne {line_number}: {err}")
    valid = bool(final_messages) and exact(final_messages[-1])
else:
    raise SystemExit(f"runtime inconnu: {runtime}")

if valid:
    print("MODEL_OUTPUT_OK")
    raise SystemExit(0)

print("MODEL_OUTPUT_KO")
raise SystemExit(1)
PY

#!/bin/sh

set -u

transcript=${1:-}
project_directory=${2:-}
sentinel_suffix=${3:-OK}
runtime_challenge=${4:-}
expected_ide_version=${5:-}

if [ -z "$transcript" ] || [ ! -f "$transcript" ] || [ -z "$project_directory" ] || [ ! -d "$project_directory" ]; then
	printf 'ANTIGRAVITY_IDE_KO\n'
	exit 1
fi

case "$sentinel_suffix" in
	OK|UPDATED)
		;;
	*)
		printf 'ANTIGRAVITY_IDE_KO\n'
		exit 1
		;;
esac

python3 - "$transcript" "$project_directory" "$sentinel_suffix" "$runtime_challenge" "$expected_ide_version" <<'PY'
import json
import os
import sys

transcript_path, project_directory, sentinel_suffix, runtime_challenge, expected_ide_version = sys.argv[1:]
project_directory = os.path.realpath(project_directory)
transcript_path = os.path.realpath(transcript_path)

challenge_context = ""
if runtime_challenge:
    challenge_context += f" STH_RUNTIME_CHALLENGE_{runtime_challenge}"
if expected_ide_version:
    challenge_context += f" ANTIGRAVITY_IDE_VERSION_{expected_ide_version}"

if os.path.basename(transcript_path) != "transcript_full.jsonl":
    raise SystemExit("transcript Antigravity complet requis")

fixtures = {
    "command": (
        ".agents/workflows/sth-runtime-command.md",
        "/sth-runtime-command" + challenge_context,
        f"ANTIGRAVITY_COMMAND_{sentinel_suffix}",
    ),
    "instruction": (
        ".agents/rules/sth-runtime-rule.md",
        "Return the STH instruction sentinel." + challenge_context,
        f"ANTIGRAVITY_INSTRUCTION_{sentinel_suffix}",
    ),
}

fixture_mtimes = []
for relative_path, _, expected_sentinel in fixtures.values():
    fixture_path = os.path.realpath(os.path.join(project_directory, relative_path))
    if os.path.commonpath([project_directory, fixture_path]) != project_directory:
        raise SystemExit("fixture Antigravity hors projet")
    if not os.path.isfile(fixture_path):
        raise SystemExit(f"fixture Antigravity absente: {relative_path}")
    with open(fixture_path, encoding="utf-8") as fixture_stream:
        fixture_content = fixture_stream.read()
    if expected_sentinel not in fixture_content:
        raise SystemExit(f"sentinelle absente de la fixture: {relative_path}")
    fixture_mtimes.append(os.path.getmtime(fixture_path))

if os.path.getmtime(transcript_path) < max(fixture_mtimes):
    raise SystemExit("transcript Antigravity antérieur aux fixtures")

def reject_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"clé JSON dupliquée: {key}")
        result[key] = value
    return result

steps = []
with open(transcript_path, encoding="utf-8") as transcript_stream:
    for line_number, transcript_line in enumerate(transcript_stream, 1):
        if not transcript_line.strip():
            continue
        try:
            step = json.loads(
                transcript_line,
                object_pairs_hook=reject_duplicate_keys,
            )
        except (json.JSONDecodeError, ValueError) as err:
            raise SystemExit(
                f"transcript Antigravity invalide ligne {line_number}: {err}"
            )
        if not isinstance(step, dict):
            raise SystemExit(f"étape Antigravity invalide ligne {line_number}")
        if "is_truncated" in step:
            raise SystemExit("transcript Antigravity tronqué interdit")
        step_index = step.get("step_index")
        if isinstance(step_index, bool) or not isinstance(step_index, int):
            raise SystemExit(f"index Antigravity invalide ligne {line_number}")
        if steps and step_index <= steps[-1]["step_index"]:
            raise SystemExit("index Antigravity non strictement croissant")
        for required_field in ("source", "type", "status"):
            if not isinstance(step.get(required_field), str):
                raise SystemExit(
                    f"champ Antigravity {required_field} invalide ligne {line_number}"
                )
        if "content" in step and not isinstance(step["content"], str):
            raise SystemExit(f"champ Antigravity content invalide ligne {line_number}")
        steps.append(step)

def is_exact_user_request(content, prompt):
    if content == prompt:
        return True
    return content.startswith(f"<USER_REQUEST>\n{prompt}\n</USER_REQUEST>\n")

def exact_pair(prompt, sentinel):
    prompt_positions = [
        step_position
        for step_position, step in enumerate(steps)
        if step["source"] == "USER_EXPLICIT"
        and step["type"] == "USER_INPUT"
        and step["status"] == "DONE"
        and is_exact_user_request(step.get("content", ""), prompt)
    ]
    if not prompt_positions:
        return False
    step_position = prompt_positions[-1]
    turn_end = len(steps)
    for following_position in range(step_position + 1, len(steps)):
        following_step = steps[following_position]
        if (
            following_step["source"] == "USER_EXPLICIT"
            and following_step["type"] == "USER_INPUT"
        ):
            turn_end = following_position
            break
    planner_responses = [
        candidate
        for candidate in steps[step_position + 1 : turn_end]
        if candidate["source"] == "MODEL"
        and candidate["type"] == "PLANNER_RESPONSE"
        and candidate["status"] == "DONE"
        and isinstance(candidate.get("content"), str)
    ]
    return len(planner_responses) == 1 and planner_responses[0]["content"] == sentinel

for _, prompt, expected_sentinel in fixtures.values():
    if not exact_pair(prompt, expected_sentinel):
        raise SystemExit(f"paire Antigravity absente: {expected_sentinel}")

print("ANTIGRAVITY_IDE_OK")
PY

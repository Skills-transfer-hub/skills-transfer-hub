#!/bin/sh

set -u

transcript=${1:-}
project_directory=${2:-}
expected_sentinel=${3:-COPILOT_COMMAND_OK}
expected_extension_version=${4:-}
runtime_challenge=${5:-}

if [ -z "$transcript" ] || [ ! -f "$transcript" ] || [ -z "$project_directory" ]; then
	printf 'DISCOVERY_KO\n'
	exit 1
fi

python3 - "$transcript" "$project_directory" "$expected_sentinel" "$expected_extension_version" "$runtime_challenge" <<'PY'
import json
import os
import sys

transcript_path = sys.argv[1]
project_directory = sys.argv[2]
expected_sentinel = sys.argv[3]
expected_extension_version = sys.argv[4]
runtime_challenge = sys.argv[5]
prompt_path = os.path.realpath(
    os.path.join(
        project_directory,
        ".github",
        "prompts",
        "sth-runtime-command.prompt.md",
    )
)

if not os.path.isfile(prompt_path):
    print("DISCOVERY_KO")
    raise SystemExit(1)

if os.path.getmtime(transcript_path) < os.path.getmtime(prompt_path):
    print("DISCOVERY_KO")
    raise SystemExit(1)

def reject_duplicate_keys(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"clé JSON dupliquée: {key}")
        result[key] = value
    return result

prompt_requests = []
with open(transcript_path, encoding="utf-8") as transcript_stream:
    for line_number, transcript_line in enumerate(transcript_stream, 1):
        transcript_line = transcript_line.strip()
        if not transcript_line:
            continue
        try:
            transcript_entry = json.loads(
                transcript_line,
                object_pairs_hook=reject_duplicate_keys,
            )
        except (json.JSONDecodeError, ValueError) as err:
            raise SystemExit(f"transcript JSONL invalide ligne {line_number}: {err}")
        if transcript_entry.get("kind") != 0:
            continue
        for request in transcript_entry.get("v", {}).get("requests", []):
            extension_identifier = (
                request.get("agent", {})
                .get("extensionId", {})
                .get("value", "")
                .lower()
            )
            if extension_identifier != "github.copilot-chat":
                continue
            prompt_resolved = any(
                part.get("kind") == "prompt"
                and part.get("name") == "sth-runtime-command"
                for part in request.get("message", {}).get("parts", [])
            )
            if prompt_resolved:
                prompt_requests.append(request)

if not prompt_requests:
    print("DISCOVERY_KO")
    raise SystemExit(1)

def response_texts(response):
    if isinstance(response, str):
        yield response
        return
    if isinstance(response, list):
        for response_part in response:
            yield from response_texts(response_part)
        return
    if not isinstance(response, dict):
        return
    for response_key in ("value", "text"):
        response_value = response.get(response_key)
        if isinstance(response_value, str):
            yield response_value
    content = response.get("content")
    if isinstance(content, str):
        yield content
    elif isinstance(content, dict):
        for content_key in ("value", "text"):
            content_value = content.get(content_key)
            if isinstance(content_value, str):
                yield content_value

def nested_strings(value):
    if isinstance(value, str):
        yield value
        return
    if isinstance(value, list):
        for item in value:
            yield from nested_strings(item)
        return
    if isinstance(value, dict):
        for item in value.values():
            yield from nested_strings(item)

latest_request = prompt_requests[-1]
latest_extension_version = latest_request.get("agent", {}).get("extensionVersion", "")
if expected_extension_version and latest_extension_version != expected_extension_version:
    print("DISCOVERY_KO")
    raise SystemExit(1)
file_resolved = any(
    variable.get("kind") == "promptFile"
    and variable.get("isRoot") is True
    and variable.get("automaticallyAdded") is True
    and os.path.realpath(variable.get("value", {}).get("path", "")) == prompt_path
    for variable in latest_request.get("variableData", {}).get("variables", [])
)
if not file_resolved:
    print("DISCOVERY_KO")
    raise SystemExit(1)
if runtime_challenge:
    expected_challenge = f"STH_RUNTIME_CHALLENGE_{runtime_challenge}"
    if not any(
        expected_challenge in candidate
        for candidate in nested_strings(latest_request.get("message", {}))
    ):
        print("DISCOVERY_KO")
        raise SystemExit(1)
result = latest_request.get("result", {})
error_message = result.get("errorDetails", {}).get("message", "")
if isinstance(error_message, str) and error_message:
    if "language model unavailable" in error_message.lower():
        print("DISCOVERY_OK_RUNTIME_NON_EXECUTE")
        raise SystemExit(3)
    print("DISCOVERY_OK_RUNTIME_KO")
    raise SystemExit(1)
response_candidates = list(response_texts(result.get("response")))
if any(candidate.strip() == expected_sentinel for candidate in response_candidates):
    print("DISCOVERY_OK_RUNTIME_OK")
    raise SystemExit(0)

print("DISCOVERY_OK_RUNTIME_KO")
raise SystemExit(1)
PY

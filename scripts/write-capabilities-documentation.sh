#!/bin/sh

set -eu

manifest_path=${1:-}
output_path=${2:-}

if [ -z "$manifest_path" ] || [ -z "$output_path" ]; then
	printf 'usage: write-capabilities-documentation.sh <manifest> <output>\n' >&2
	exit 1
fi

if [ ! -f "$manifest_path" ]; then
	printf 'capabilities manifest not found: %s\n' "$manifest_path" >&2
	exit 1
fi

output_directory=$(dirname "$output_path")
temporary_path="$output_path.tmp"
mkdir -p "$output_directory"
jq -r '
  def cell($target; $kind):
    first(.compatibility[] | select(.target_kind == $target and .artifact_kind == $kind) | .level);
  [
    "# STH capabilities",
    "",
    "Generated from schema version \(.schema_version) for CLI \(.cli_version).",
    "",
    "| Target | Agent | Command | Instruction | Rule | Skill | Workflow |",
    "|---|---|---|---|---|---|---|"
  ] +
  [
    .targets[] as $target |
    "| \($target) | \(cell($target; "agent")) | \(cell($target; "command")) | \(cell($target; "instruction")) | \(cell($target; "rule")) | \(cell($target; "skill")) | \(cell($target; "workflow")) |"
  ] +
  [
    "",
    "## Target contracts",
    ""
  ] +
  [
    .target_specs[] |
    "- **\(.kind)** — maturity: \(.maturity); minimum version: \(if (.minimum_version // "") == "" then "not declared" else .minimum_version end)"
  ] |
  .[]
' "$manifest_path" > "$temporary_path"
mv "$temporary_path" "$output_path"

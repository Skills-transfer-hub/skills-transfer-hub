#!/bin/sh

gemini_contract_failure() {
	printf 'Gemini runtime contract failed: %s\n' "$1" >&2
	return 1
}

validate_gemini_runtime_owned_path() {
	runtime_path=$1
	expected_name=$2
	if [ "$(basename -- "$runtime_path")" != "$expected_name" ]; then
		gemini_contract_failure "$expected_name path"
		return 1
	fi
	runtime_parent_name=$(basename -- "$(dirname -- "$runtime_path")")
	case "$runtime_parent_name" in
		sth-compat.*)
			;;
		*)
			gemini_contract_failure "$expected_name runtime parent"
			return 1
			;;
	esac
}

validate_gemini_runtime_environment() {
	if [ -z "${GEMINI_CLI_HOME:-}" ] || [ ! -d "$GEMINI_CLI_HOME" ]; then
		gemini_contract_failure 'GEMINI_CLI_HOME'
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$GEMINI_CLI_HOME" gemini-home; then
		return 1
	fi
	if [ -z "${CLOUDSDK_CONFIG:-}" ] || [ ! -d "$CLOUDSDK_CONFIG" ]; then
		gemini_contract_failure 'CLOUDSDK_CONFIG'
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$CLOUDSDK_CONFIG" gemini-cloudsdk; then
		return 1
	fi
	if [ "${GEMINI_CLI_NO_RELAUNCH:-}" != true ]; then
		gemini_contract_failure 'GEMINI_CLI_NO_RELAUNCH'
		return 1
	fi
	if [ "${GEMINI_CLI_TRUST_WORKSPACE:-}" != true ]; then
		gemini_contract_failure 'GEMINI_CLI_TRUST_WORKSPACE'
		return 1
	fi
	if [ "${GEMINI_SANDBOX:-}" != false ] || [ -n "${SANDBOX:-}" ]; then
		gemini_contract_failure 'sandbox environment'
		return 1
	fi
	if [ "${GEMINI_TELEMETRY_ENABLED:-}" != false ] || [ "${GEMINI_TELEMETRY_LOG_PROMPTS:-}" != false ] || [ "${GEMINI_TELEMETRY_TRACES_ENABLED:-}" != false ]; then
		gemini_contract_failure 'telemetry environment'
		return 1
	fi
	if [ -n "${GEMINI_CLI_ACTIVITY_LOG_TARGET:-}" ]; then
		gemini_contract_failure 'activity log target'
		return 1
	fi
	if [ -z "${GOOGLE_APPLICATION_CREDENTIALS:-}" ] || [ -e "$GOOGLE_APPLICATION_CREDENTIALS" ]; then
		gemini_contract_failure 'ambient ADC isolation'
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$GOOGLE_APPLICATION_CREDENTIALS" gemini-no-ambient-adc.json; then
		return 1
	fi
	if [ "${CODE_ASSIST_ENDPOINT:-}" != https://cloudcode-pa.googleapis.com ] || [ "${CODE_ASSIST_API_VERSION:-}" != v1internal ]; then
		gemini_contract_failure 'official Code Assist endpoint'
		return 1
	fi
	if [ "${GEMINI_CLI_CUSTOM_HEADERS+x}" != x ] || [ -n "$GEMINI_CLI_CUSTOM_HEADERS" ] || [ "${GOOGLE_GEMINI_BASE_URL+x}" != x ] || [ -n "$GOOGLE_GEMINI_BASE_URL" ] || [ "${GOOGLE_VERTEX_BASE_URL+x}" != x ] || [ -n "$GOOGLE_VERTEX_BASE_URL" ]; then
		gemini_contract_failure 'custom endpoint environment'
		return 1
	fi
	if [ -z "${GEMINI_CLI_SYSTEM_DEFAULTS_PATH:-}" ] || [ ! -f "$GEMINI_CLI_SYSTEM_DEFAULTS_PATH" ]; then
		gemini_contract_failure 'system defaults path'
		return 1
	fi
	if [ -z "${GEMINI_CLI_SYSTEM_SETTINGS_PATH:-}" ] || [ ! -f "$GEMINI_CLI_SYSTEM_SETTINGS_PATH" ]; then
		gemini_contract_failure 'system settings path'
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$GEMINI_CLI_SYSTEM_DEFAULTS_PATH" gemini-system-defaults.json; then
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$GEMINI_CLI_SYSTEM_SETTINGS_PATH" gemini-system-settings.json; then
		return 1
	fi
	for required_setting in \
		'"ignoreLocalEnv": true' \
		'"hooksConfig": {"enabled": false}' \
		'"usageStatisticsEnabled": false' \
		'"telemetry": {"enabled": false, "logPrompts": false}' \
		'"security": {"disableYoloMode": true, "disableAlwaysAllow": true}'; do
		if ! grep -Fq "$required_setting" "$GEMINI_CLI_SYSTEM_SETTINGS_PATH"; then
			gemini_contract_failure "system setting $required_setting"
			return 1
		fi
	done
}

validate_gemini_isolated_arguments() {
	extensions_value=
	allowed_mcp_value=
	pending_isolation_option=
	for gemini_argument in "$@"; do
		if [ -n "$pending_isolation_option" ]; then
			case "$pending_isolation_option" in
				extensions)
					extensions_value=$gemini_argument
					;;
				allowed-mcp)
					allowed_mcp_value=$gemini_argument
					;;
			esac
			pending_isolation_option=
			continue
		fi
		case "$gemini_argument" in
			--extensions|-e)
				pending_isolation_option=extensions
				;;
			--extensions=*|-e=*)
				extensions_value=${gemini_argument#*=}
				;;
			--allowed-mcp-server-names)
				pending_isolation_option=allowed-mcp
				;;
			--allowed-mcp-server-names=*)
				allowed_mcp_value=${gemini_argument#*=}
				;;
		esac
	done
	if [ -n "$pending_isolation_option" ] || [ "$extensions_value" != none ] || ! printf '%s\n' "$allowed_mcp_value" | grep -Eq '^sth-runtime-disabled-[0-9a-f]{32}$'; then
		gemini_contract_failure 'isolated CLI arguments'
		return 1
	fi
}

gemini_skills_list_requested() {
	previous_argument=
	for gemini_argument in "$@"; do
		if [ "$previous_argument" = skills ] && [ "$gemini_argument" = list ]; then
			return 0
		fi
		previous_argument=$gemini_argument
	done
	return 1
}

validate_gemini_model_arguments() {
	approval_mode=
	admin_policy=
	pending_gemini_option=
	for gemini_argument in "$@"; do
		if [ -n "$pending_gemini_option" ]; then
			case "$pending_gemini_option" in
				approval-mode)
					approval_mode=$gemini_argument
					;;
				admin-policy)
					admin_policy=$gemini_argument
					;;
			esac
			pending_gemini_option=
			continue
		fi
		case "$gemini_argument" in
			-s|--sandbox|--sandbox=*|-y|--yolo|--yolo=*)
				gemini_contract_failure "forbidden argument $gemini_argument"
				return 1
				;;
			--approval-mode)
				if [ -n "$approval_mode" ]; then
					gemini_contract_failure 'duplicate approval mode'
					return 1
				fi
				pending_gemini_option=approval-mode
				;;
			--approval-mode=*)
				approval_mode=${gemini_argument#--approval-mode=}
				;;
			--admin-policy)
				if [ -n "$admin_policy" ]; then
					gemini_contract_failure 'duplicate admin policy'
					return 1
				fi
				pending_gemini_option=admin-policy
				;;
			--admin-policy=*)
				admin_policy=${gemini_argument#--admin-policy=}
				;;
		esac
	done
	if [ -n "$pending_gemini_option" ]; then
		gemini_contract_failure "missing value for $pending_gemini_option"
		return 1
	fi
	if [ "$approval_mode" != plan ]; then
		gemini_contract_failure 'approval mode is not plan'
		return 1
	fi
	if [ -z "$admin_policy" ] || [ ! -f "$admin_policy" ]; then
		gemini_contract_failure 'admin policy path'
		return 1
	fi
	if ! validate_gemini_runtime_owned_path "$admin_policy" gemini-admin-policy.toml; then
		return 1
	fi
	for required_policy in \
		'toolName = "*"' \
		'decision = "deny"' \
		'priority = 900' \
		'toolName = ["activate_skill", "sth-runtime-agent"]' \
		'decision = "allow"' \
		'priority = 999' \
		'modes = ["plan"]' \
		'interactive = false'; do
		if ! grep -Fq "$required_policy" "$admin_policy"; then
			gemini_contract_failure "admin policy $required_policy"
			return 1
		fi
	done
}

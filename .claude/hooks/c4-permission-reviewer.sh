#!/bin/bash
# C4 Permission Reviewer Hook (PermissionRequest)
# Reviews gray-zone tool requests using Haiku API.
# Reads config from $CLAUDE_PROJECT_DIR/.c4/hook-config.json.
#
# Matched tools: Bash|Read|Edit|Write|NotebookEdit|WebFetch|WebSearch|Search|Skill
# Explicitly excluded: AskUserQuestion (not in matcher list)
#
# Exit codes:
#   0 - Allow (or no decision)
#   2 - Deny
#
# Protocol output (stdout, PermissionRequest format):
#   {"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow|deny","message":"..."}}}

# =============================================================================
# Read input from stdin
# =============================================================================
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)
# For Bash: use command; for file tools: use file_path
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)

# Use CLAUDE_PROJECT_DIR if set; otherwise walk up from PWD
_find_c4_root() {
    if [[ -n "${CLAUDE_PROJECT_DIR:-}" ]] && [[ -d "${CLAUDE_PROJECT_DIR}/.c4" ]]; then
        echo "$CLAUDE_PROJECT_DIR"
        return 0
    fi
    local dir="$PWD"
    while [[ "$dir" != "/" ]] && [[ -n "$dir" ]]; do
        if [[ -d "$dir/.c4" ]]; then
            echo "$dir"
            return 0
        fi
        dir="${dir%/*}"
    done
    return 1
}

C4_ROOT=$(_find_c4_root) || exit 0

# Load config
HOOK_CONFIG_JSON="${C4_ROOT}/.c4/hook-config.json"

PERMISSION_MODE="model"
SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

if [[ -f "$HOOK_CONFIG_JSON" ]] && command -v jq &>/dev/null; then
    _enabled=$(jq -r '.enabled // true' "$HOOK_CONFIG_JSON" 2>/dev/null)
    if [[ "$_enabled" == "false" ]]; then
        exit 0
    fi
    PERMISSION_MODE=$(jq -r '.mode // "model"' "$HOOK_CONFIG_JSON" 2>/dev/null)
    SUPERVISOR_MODEL=$(jq -r '.model // "claude-haiku-4-5-20251001"' "$HOOK_CONFIG_JSON" 2>/dev/null)
    while IFS= read -r _p; do [[ -n "$_p" ]] && ALLOW_PATTERNS+=("$_p"); done < <(jq -r '.allow_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
    while IFS= read -r _p; do [[ -n "$_p" ]] && BLOCK_PATTERNS+=("$_p"); done < <(jq -r '.block_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
else
    # No config: C4 project found but hook-config.json not yet generated — allow all
    exit 0
fi

# =============================================================================
# Helper emit functions (PermissionRequest format)
# =============================================================================
_emit_allow() {
    local message="${1:-Approved}"
    jq -n --arg msg "$message" '{
        hookSpecificOutput: {
            hookEventName: "PermissionRequest",
            decision: {
                behavior: "allow",
                message: $msg
            }
        }
    }' 2>/dev/null || echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PermissionRequest\",\"decision\":{\"behavior\":\"allow\",\"message\":\"${message//\'/}\"}}}"
    exit 0
}

_emit_deny() {
    local message="${1:-Denied}"
    jq -n --arg msg "$message" '{
        hookSpecificOutput: {
            hookEventName: "PermissionRequest",
            decision: {
                behavior: "deny",
                message: $msg
            }
        }
    }' 2>/dev/null || echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PermissionRequest\",\"decision\":{\"behavior\":\"deny\",\"message\":\"${message//\'/}\"}}}"
    exit 2
}

# =============================================================================
# Allow patterns (fast path — no API call)
# =============================================================================
TARGET="${COMMAND:-$FILE_PATH}"
for pattern in "${ALLOW_PATTERNS[@]}"; do
    if [[ "$TARGET" =~ $pattern ]]; then
        _emit_allow "Allow pattern match"
    fi
done

# Block patterns (fast path)
for pattern in "${BLOCK_PATTERNS[@]}"; do
    if [[ "$TARGET" =~ $pattern ]]; then
        _emit_deny "Blocked by config: matches '$pattern'"
    fi
done

# =============================================================================
# MODEL MODE — Haiku API judgment
# =============================================================================
if [[ "$PERMISSION_MODE" == "model" ]]; then
    api_key="${ANTHROPIC_API_KEY:-}"
    # Fallback: read from CQ secret store if env var not set
    if [[ -z "$api_key" ]] && command -v cq &>/dev/null; then
        api_key=$(cq secret get anthropic.api_key 2>/dev/null)
    fi
    if [[ -z "$api_key" ]] || ! command -v jq &>/dev/null; then
        # No API key or jq — allow (fail open)
        exit 0
    fi

    # Build context string for the model
    if [[ -n "$COMMAND" ]]; then
        CONTEXT="Tool: $TOOL_NAME\nCommand: $COMMAND"
    else
        CONTEXT="Tool: $TOOL_NAME\nFile: $FILE_PATH"
    fi

    payload=$(jq -n \
        --arg model "$SUPERVISOR_MODEL" \
        --arg context "$CONTEXT" \
        '{
            model: $model,
            max_tokens: 150,
            messages: [{
                role: "user",
                content: ("You are a security reviewer for an AI-managed C4 project. Respond ONLY with JSON.\n\nShould this tool use be allowed?\n\n" + $context + "\n\nAllow unless: (1) could cause irreversible data loss; (2) contains credentials/private keys; (3) writes to system directories; (4) pipes remote code to shell.\n\nNormal dev operations (build, test, read, edit code) are safe.\n\nRespond: {\"allow\": true, \"reason\": \"brief\"} or {\"allow\": false, \"reason\": \"brief\"}")
            }]
        }')

    response=$(curl -s --max-time 8 \
        "https://api.anthropic.com/v1/messages" \
        -H "x-api-key: $api_key" \
        -H "anthropic-version: 2023-06-01" \
        -H "content-type: application/json" \
        -d "$payload" 2>/dev/null)

    if [[ -n "$response" ]]; then
        text=$(echo "$response" | jq -r '.content[0].text' 2>/dev/null)
        # Strip markdown code fences (```json ... ```)
        text=$(echo "$text" | sed 's/^```[a-z]*//;s/```$//' | tr -d '\n' | sed 's/^ *//')
        if [[ -n "$text" && "$text" != "null" ]]; then
            allow=$(echo "$text" | jq -r '.allow // "null"' 2>/dev/null)
            reason=$(echo "$text" | jq -r '.reason // ""' 2>/dev/null)
            if [[ "$allow" == "false" ]]; then
                _emit_deny "AI reviewer blocked: $reason"
            elif [[ "$allow" == "true" ]]; then
                _emit_allow "AI reviewer approved: $reason"
            fi
            # allow == "null" (parse failure) → fall through to fail-open exit 0
        fi
    fi
fi

# =============================================================================
# Default: allow (fail open — hook mode or API unavailable)
# =============================================================================
exit 0

#!/bin/bash
# C4 Edit/Write Security Hook
# Reviews file edits for safety — hybrid pattern-first + Haiku model review.
# Same architecture as c4-bash-security-hook.sh.
#
# Exit codes:
#   0 - Allow (with permissionDecision JSON for auto-approve when auto_approve=true)
#   2 - Block (with permissionDecision JSON for deny)
#
# Config: .c4/hook-config.json (shared SSOT with bash hook)
#   enable/disable, mode (model|hook), allow_patterns, block_patterns

# =============================================================================
# Read input from stdin (Claude Code passes hook data as JSON on stdin)
# =============================================================================
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // "Edit"' 2>/dev/null)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
# Edit uses new_string; Write uses content
NEW_CONTENT=$(echo "$INPUT" | jq -r '.tool_input.new_string // .tool_input.content // empty' 2>/dev/null)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty' 2>/dev/null)
[[ -z "$CWD" ]] && CWD="$PWD"

# Walk up directory tree to find nearest .c4/ (supports subdirectory and monorepo usage)
_find_c4_root() {
    local dir="${1:-$CWD}"
    while [[ "$dir" != "/" ]] && [[ -n "$dir" ]]; do
        if [[ -d "$dir/.c4" ]]; then
            echo "$dir"
            return 0
        fi
        dir="${dir%/*}"
    done
    return 1
}

C4_ROOT=$(_find_c4_root "$CWD") || C4_ROOT=$(_find_c4_root "$PWD") || exit 0

# Skip if no file path
if [[ -z "$FILE_PATH" ]]; then
    exit 0
fi

# =============================================================================
# Load Configuration (same hook-config.json as bash hook)
# =============================================================================
HOOK_CONFIG_JSON="${C4_ROOT}/.c4/hook-config.json"

PERMISSION_MODE="model"
AUTO_APPROVE="true"
SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

if [[ -f "$HOOK_CONFIG_JSON" ]] && command -v jq &>/dev/null; then
    _enabled=$(jq -r '.enabled // true' "$HOOK_CONFIG_JSON" 2>/dev/null)
    if [[ "$_enabled" == "false" ]]; then
        exit 0
    fi
    PERMISSION_MODE=$(jq -r '.mode // "model"' "$HOOK_CONFIG_JSON" 2>/dev/null)
    AUTO_APPROVE=$(jq -r '.auto_approve // true' "$HOOK_CONFIG_JSON" 2>/dev/null)
    SUPERVISOR_MODEL=$(jq -r '.model // "claude-haiku-4-5-20251001"' "$HOOK_CONFIG_JSON" 2>/dev/null)
    # bash 3.2 compatible (no mapfile)
    while IFS= read -r _p; do [[ -n "$_p" ]] && ALLOW_PATTERNS+=("$_p"); done < <(jq -r '.allow_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
    while IFS= read -r _p; do [[ -n "$_p" ]] && BLOCK_PATTERNS+=("$_p"); done < <(jq -r '.block_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
else
    # No config: use hook mode with built-in patterns only
    PERMISSION_MODE="hook"
fi

# =============================================================================
# Helper: emit approval/denial JSON for Claude Code hooks protocol
# =============================================================================
_emit_allow() {
    local reason="${1:-Safe edit}"
    if [[ "$AUTO_APPROVE" == "true" ]]; then
        jq -n --arg reason "$reason" '{
            hookSpecificOutput: {
                hookEventName: "PreToolUse",
                permissionDecision: "allow",
                permissionDecisionReason: $reason
            }
        }' 2>/dev/null || cat << EOF
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"$reason"}}
EOF
    fi
    exit 0
}

_emit_deny() {
    local reason="$1"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || cat << EOF
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"$reason"}}
EOF
    exit 2
}

_log_decision() {
    local decision="$1"
    local reason="$2"
    [[ -n "${C4_ROOT}" ]] || return 0
    local ts
    ts=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
    printf '%s | %s | %s | edit:%s %s\n' "$ts" "$decision" "$reason" "$TOOL_NAME" "$FILE_PATH" \
        >> "${C4_ROOT}/.c4/hook-decisions.log" 2>/dev/null
}

# =============================================================================
# BUILT-IN ALLOW: Always-safe locations (before user patterns)
# =============================================================================

# .c4/ system files — C4 metadata, never dangerous
if [[ "$FILE_PATH" =~ (^|/)\.c4/ ]]; then
    _emit_allow "C4 system file"
fi

# Temporary directory — scratch space, safe
if [[ "$FILE_PATH" =~ ^/tmp/ ]]; then
    _emit_allow "Temporary directory"
fi

# =============================================================================
# USER ALLOW PATTERNS (from hook-config.json — applied to file_path)
# =============================================================================
for pattern in "${ALLOW_PATTERNS[@]}"; do
    if [[ "$FILE_PATH" =~ $pattern ]]; then
        _emit_allow "User allow pattern match"
    fi
done

# =============================================================================
# USER BLOCK PATTERNS (from hook-config.json — applied to file_path)
# =============================================================================
for pattern in "${BLOCK_PATTERNS[@]}"; do
    if [[ "$FILE_PATH" =~ $pattern ]]; then
        _emit_deny "Blocked by config: matches '$pattern'"
    fi
done

# =============================================================================
# BUILT-IN BLOCK: Dangerous content or paths
# =============================================================================
CONTENT_PREVIEW="${NEW_CONTENT:0:2000}"

# Private key material in new content
if printf '%s' "$CONTENT_PREVIEW" | grep -qE '^-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY' 2>/dev/null; then
    _emit_deny "Private key content detected in file write"
fi

# Writing to system directories
if [[ "$FILE_PATH" =~ ^/(etc|usr/bin|usr/local/bin|bin|sbin)/ ]]; then
    _emit_deny "Writing to system directory: $FILE_PATH"
fi

# =============================================================================
# MODEL MODE — Haiku API review (same pattern as bash hook)
# Falls through on API failure (no key, network error) to hook mode below.
# =============================================================================
if [[ "$PERMISSION_MODE" == "model" ]]; then

    _model_check() {
        local api_key="${ANTHROPIC_API_KEY:-}"
        [[ -z "$api_key" ]] && return 1
        command -v jq &>/dev/null || return 1

        local content_snippet="${NEW_CONTENT:0:500}"

        local payload
        payload=$(jq -n \
            --arg model "$SUPERVISOR_MODEL" \
            --arg path "$FILE_PATH" \
            --arg tool "$TOOL_NAME" \
            --arg content "$content_snippet" \
            '{
                model: $model,
                max_tokens: 150,
                messages: [{
                    role: "user",
                    content: ("You are a file-edit safety supervisor for a C4 AI-managed project. Respond ONLY with JSON, no other text.\n\nTool: " + $tool + "\nFile: " + $path + "\nContent preview (first 500 chars):\n" + $content + "\n\nIs this edit safe? Flag ONLY if: (1) new content contains plaintext API keys, tokens, or passwords; (2) writing private key PEM blocks; (3) overwriting critical system config files.\n\nNormal code, docs, config changes without credentials are safe.\n\nRespond exactly: {\"safe\": true, \"reason\": \"brief\"} or {\"safe\": false, \"reason\": \"brief\"}")
                }]
            }')

        local response
        response=$(curl -s --max-time 8 \
            "https://api.anthropic.com/v1/messages" \
            -H "x-api-key: $api_key" \
            -H "anthropic-version: 2023-06-01" \
            -H "content-type: application/json" \
            -d "$payload" 2>/dev/null)

        [[ -z "$response" ]] && return 1

        local text
        text=$(echo "$response" | jq -r '.content[0].text' 2>/dev/null)
        [[ -z "$text" || "$text" == "null" ]] && return 1

        local safe reason
        safe=$(echo "$text" | jq -r '.safe' 2>/dev/null)
        reason=$(echo "$text" | jq -r '.reason' 2>/dev/null)

        if [[ "$safe" == "false" ]]; then
            _log_decision "deny" "$reason"
            _emit_deny "AI supervisor (Haiku) blocked: $reason"
        fi

        # Model says safe
        _log_decision "safe" "$reason"
        _emit_allow "AI supervisor: $reason"
    }

    # Run model check; on failure fall through to hook-mode patterns below
    _model_check
fi

# =============================================================================
# HOOK MODE — pattern-based fallback
# (used when mode=hook or when model API is unavailable)
# =============================================================================

# All checks passed — allow the edit
_emit_allow "Edit security check passed"

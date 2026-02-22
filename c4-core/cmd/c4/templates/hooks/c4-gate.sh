#!/bin/bash
# C4 Gate Hook (PreToolUse)
# Rule-based fast gate for Bash/Edit/Write tools.
# Reads allow/block patterns from $CLAUDE_PROJECT_DIR/.c4/hook-config.json.
# Gray-zone commands (no pattern match) exit 0 — passed to PermissionRequest hook.
#
# Exit codes:
#   0 - No decision (pass to PermissionRequest, or allow if no further hook)
#   2 - Block command (deny)
#
# Protocol output (stdout):
#   allow: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow",...}}
#   deny:  {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",...}}

# =============================================================================
# Read input from stdin
# =============================================================================
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)
# For Bash: use command; for Edit/Write: use file_path
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
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

if [[ -f "$HOOK_CONFIG_JSON" ]] && command -v jq &>/dev/null; then
    _enabled=$(jq -r '.enabled // true' "$HOOK_CONFIG_JSON" 2>/dev/null)
    if [[ "$_enabled" == "false" ]]; then
        exit 0
    fi
    while IFS= read -r _p; do [[ -n "$_p" ]] && ALLOW_PATTERNS+=("$_p"); done < <(jq -r '.allow_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
    while IFS= read -r _p; do [[ -n "$_p" ]] && BLOCK_PATTERNS+=("$_p"); done < <(jq -r '.block_patterns[]? // empty' "$HOOK_CONFIG_JSON" 2>/dev/null)
else
    # No config: C4 project exists but MCP not started yet — use built-in safe allow patterns
    ALLOW_PATTERNS=(
        "^git (log|show|diff|status|branch|tag|remote|stash list|ls-files|blame)( |$)"
        "^go (build|test|vet|run|env|list|mod)( |$)"
        "^uv (run|sync|pip list|pip show|lock|python)( |$)"
        "^(ls|ll|la|cat|head|tail|wc|stat)( |$)"
        "^find \\. "
        "^(grep|rg) "
        "^(echo|pwd|which|env|printenv|date|uname|whoami)( |$)"
        "^(cargo|rustc) (build|test|check|clippy)( |$)"
        "^pnpm (build|test|lint|install|dev)( |$)"
        "^cq (status|doctor|mcp|version|help)( |$)"
        "^sqlite3 "
    )
fi

# =============================================================================
# Helper emit functions
# =============================================================================
_emit_allow() {
    local reason="${1:-Safe}"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "allow",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"allow\",\"permissionDecisionReason\":\"$reason\"}}"
    exit 0
}

_emit_deny() {
    local reason="${1:-Blocked}"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"${reason//\'/}\"}}"
    exit 2
}

# =============================================================================
# Bash tool: check allow/block patterns against command
# =============================================================================
if [[ "$TOOL_NAME" == "Bash" ]] && [[ -n "$COMMAND" ]]; then
    # Allow patterns — user whitelist
    for pattern in "${ALLOW_PATTERNS[@]}"; do
        if [[ "$COMMAND" =~ $pattern ]]; then
            _emit_allow "Allow pattern match"
        fi
    done

    # Block patterns — user blacklist
    for pattern in "${BLOCK_PATTERNS[@]}"; do
        if [[ "$COMMAND" =~ $pattern ]]; then
            _emit_deny "Blocked by config: matches '$pattern'"
        fi
    done

    # Built-in dangerous patterns — always block
    # System destruction
    if [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+/ ]] || \
       [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+~ ]] || \
       [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\* ]]; then
        _emit_deny "Recursive delete of system/home directory"
    fi

    # Git force push to main/master
    if [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]].*--force-with-lease ]]; then
        : # --force-with-lease is safe
    elif [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(-f|--force)[[:space:]]+(origin[[:space:]]+)?(main|master) ]] || \
         [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(origin[[:space:]]+)?(main|master)[[:space:]]+(-f|--force) ]]; then
        _emit_deny "Force push to main/master branch"
    fi

    # Remote code execution via curl/wget pipe
    if [[ "$COMMAND" =~ (curl|wget)[[:space:]].*\|[[:space:]]*(sh|bash|zsh|python|perl|ruby) ]]; then
        _emit_deny "Piping remote content to shell"
    fi
fi

# =============================================================================
# Edit/Write tool: check patterns against file_path
# Priority: built-in allow > user allow > user block > built-in block
# =============================================================================
if [[ "$TOOL_NAME" == "Edit" || "$TOOL_NAME" == "Write" ]] && [[ -n "$FILE_PATH" ]]; then
    # Built-in allow: .c4/ system files and /tmp/ — checked first so user block patterns can't override
    if [[ "$FILE_PATH" =~ (^|/)\.c4/ ]] || [[ "$FILE_PATH" =~ ^/tmp/ ]]; then
        _emit_allow "C4 system or temp file"
    fi

    # User allow patterns — checked before user block patterns
    for pattern in "${ALLOW_PATTERNS[@]}"; do
        if [[ "$FILE_PATH" =~ $pattern ]]; then
            _emit_allow "Allow pattern match"
        fi
    done

    # User block patterns
    for pattern in "${BLOCK_PATTERNS[@]}"; do
        if [[ "$FILE_PATH" =~ $pattern ]]; then
            _emit_deny "Blocked by config: matches '$pattern'"
        fi
    done

    # Built-in block: system directories
    if [[ "$FILE_PATH" =~ ^/(etc|usr/bin|usr/local/bin|bin|sbin)/ ]]; then
        _emit_deny "Writing to system directory: $FILE_PATH"
    fi
fi

# No pattern matched — exit 0 (gray zone, let PermissionRequest handle it)
exit 0

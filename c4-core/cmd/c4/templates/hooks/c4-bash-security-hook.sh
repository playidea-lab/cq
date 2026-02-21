#!/bin/bash
# C4 Bash Security Hook
# Reviews Bash commands for safety — blocks dangerous, auto-approves safe ones.
#
# Exit codes:
#   0 - Allow command (with permissionDecision JSON for auto-approve)
#   2 - Block command (with permissionDecision JSON for deny)
#
# Configuration: ~/.claude/hooks/c4-bash-security.conf
#
#   PERMISSION_MODE="model"   # "model" (default) or "hook"
#   AUTO_APPROVE="true"       # "true" (default) or "false"
#   SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
#
#   ALLOW_PATTERNS=(          # Whitelist (always checked first)
#       "rm -rf /tmp/test"
#   )
#   BLOCK_PATTERNS=(          # Extra block patterns (hook mode / fallback)
#       "docker rm"
#   )

# =============================================================================
# Read input from stdin (Claude Code passes hook data as JSON on stdin)
# =============================================================================
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)

# Fallback: if jq is not available or stdin was empty, try positional args
if [[ -z "$COMMAND" ]] && [[ -n "$*" ]]; then
    COMMAND="$*"
fi

# Skip if not in a C4 project (.c4/ directory must exist)
if [[ ! -d ".c4" ]]; then
    exit 0
fi

# Skip empty commands
if [[ -z "$COMMAND" ]]; then
    exit 0
fi

# =============================================================================
# Load Configuration
# =============================================================================
CONFIG_FILE="$HOME/.claude/hooks/c4-bash-security.conf"

PERMISSION_MODE="model"
AUTO_APPROVE="true"
SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

# Verify conf is owned by current user and not world-writable before sourcing
if [[ -f "$CONFIG_FILE" ]]; then
    conf_owner=$(stat -c "%u" "$CONFIG_FILE" 2>/dev/null || stat -f "%u" "$CONFIG_FILE" 2>/dev/null)
    conf_mode=$(stat -c "%a" "$CONFIG_FILE" 2>/dev/null || stat -f "%OLp" "$CONFIG_FILE" 2>/dev/null)
    if [[ -z "$conf_mode" ]] || [[ "$conf_owner" != "$(id -u)" ]] || [[ "${conf_mode: -2:1}" =~ [2367] ]] || [[ "${conf_mode: -1}" =~ [2367] ]]; then
        echo "c4-security-hook: WARNING: $CONFIG_FILE is not trusted (owner/perms), skipping." >&2
    else
        # shellcheck source=/dev/null
        source "$CONFIG_FILE"
    fi
fi

# =============================================================================
# Helper: emit approval/denial JSON for Claude Code hooks protocol
# =============================================================================
_emit_allow() {
    local reason="${1:-Safe command}"
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
    local instructions="${2:-Run manually if you are sure this is safe.}"
    jq -n --arg reason "$reason" --arg instr "$instructions" '{
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

# =============================================================================
# User Whitelist — always checked first, regardless of mode
# =============================================================================
for pattern in "${ALLOW_PATTERNS[@]}"; do
    if [[ "$COMMAND" =~ $pattern ]]; then
        _emit_allow "User whitelist match"
    fi
done

# =============================================================================
# MODEL MODE — Haiku API supervision
# =============================================================================
if [[ "$PERMISSION_MODE" == "model" ]]; then

    _model_check() {
        local api_key="${ANTHROPIC_API_KEY:-}"
        [[ -z "$api_key" ]] && return 1
        command -v jq &>/dev/null || return 1

        local payload
        payload=$(jq -n \
            --arg model "$SUPERVISOR_MODEL" \
            --arg cmd "$COMMAND" \
            '{
                model: $model,
                max_tokens: 150,
                messages: [{
                    role: "user",
                    content: ("You are a bash command safety supervisor for a C4 project (AI-managed codebase). Respond ONLY with JSON, no other text.\n\nIs this bash command dangerous? Consider: data loss, irreversible operations, system damage, security risks, accidental deletion, force operations.\n\nCommand: " + $cmd + "\n\nRespond exactly: {\"safe\": true, \"reason\": \"brief\"} or {\"safe\": false, \"reason\": \"brief\"}")
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
            _emit_deny "AI supervisor (Haiku) blocked: $reason"
        fi

        # Model says safe
        _emit_allow "AI supervisor: $reason"
    }

    # Run model check; on failure (no API key, network error, jq missing)
    # fall through to hook-based patterns below
    _model_check
fi

# =============================================================================
# HOOK MODE — pattern-based checks
# (also used as fallback when model mode is unavailable)
# =============================================================================

# User block patterns
for pattern in "${BLOCK_PATTERNS[@]}"; do
    if [[ "$COMMAND" =~ $pattern ]]; then
        _emit_deny "Blocked by user config: matches '$pattern'" \
            "This command is blocked by your c4-bash-security.conf. Remove the pattern to allow."
    fi
done

# System destruction
if [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+/ ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+~ ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\* ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\.\* ]]; then
    _emit_deny "Recursive delete of system/home directory"
fi

# Sudo with dangerous commands
if [[ "$COMMAND" =~ sudo[[:space:]]+(rm|chmod|chown|mkfs|dd|truncate|shred) ]]; then
    _emit_deny "sudo with potentially destructive command"
fi

# Dangerous permission changes
if [[ "$COMMAND" =~ chmod[[:space:]]+(-R[[:space:]]+)?777 ]] || \
   [[ "$COMMAND" =~ chmod[[:space:]]+(-R[[:space:]]+)?a\+rwx ]]; then
    _emit_deny "Setting world-writable permissions (chmod 777)"
fi

# Disk/filesystem operations
if [[ "$COMMAND" =~ mkfs ]] || \
   [[ "$COMMAND" =~ dd[[:space:]]+if= ]] || \
   [[ "$COMMAND" =~ \>[[:space:]]*/dev/ ]]; then
    _emit_deny "Direct disk/filesystem operation"
fi

# Fork bomb and similar
if [[ "$COMMAND" =~ :\(\)\{[[:space:]]*:\|: ]]; then
    _emit_deny "Potential fork bomb detected"
fi

# Git force push to main/master
if [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(-f|--force)[[:space:]]+(origin[[:space:]]+)?(main|master) ]] || \
   [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(origin[[:space:]]+)?(main|master)[[:space:]]+(-f|--force) ]]; then
    _emit_deny "Force push to main/master branch"
fi

# Remote code execution via curl/wget pipe
if [[ "$COMMAND" =~ (curl|wget)[[:space:]].*\|[[:space:]]*(sh|bash|zsh|python|perl|ruby) ]]; then
    _emit_deny "Piping remote content to shell"
fi

# Eval with command substitution (potential injection)
if [[ "$COMMAND" =~ eval[[:space:]]+\$\( ]]; then
    _emit_deny "eval with command substitution (injection risk)"
fi

# Writing to system config directories
if [[ "$COMMAND" =~ \>[[:space:]]*/etc/ ]] || \
   [[ "$COMMAND" =~ tee[[:space:]]+/etc/ ]]; then
    _emit_deny "Writing to /etc/ directory"
fi

# npm/pnpm publish without explicit flag
if [[ "$COMMAND" =~ (npm|pnpm)[[:space:]]+publish ]] && \
   [[ ! "$COMMAND" =~ --dry-run ]]; then
    _emit_deny "Publishing to npm registry (use --dry-run first)"
fi

# Git hard reset
if [[ "$COMMAND" =~ git[[:space:]]+reset[[:space:]]+--hard ]]; then
    _emit_deny "git reset --hard can lose uncommitted changes"
fi

# =============================================================================
# All checks passed - allow command
# =============================================================================
_emit_allow "Pattern checks passed"

#!/bin/bash
# C4 Bash Security Hook
# Blocks dangerous commands to prevent accidental system damage
#
# Exit codes:
#   0 - Allow command
#   2 - Block command (with JSON response)
#
# Configuration: ~/.claude/hooks/c4-bash-security.conf
#
#   PERMISSION_MODE="model"   # "model" (default) or "hook"
#   SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
#
#   ALLOW_PATTERNS=(          # Whitelist (always checked first)
#       "rm -rf /tmp/test"
#   )
#   BLOCK_PATTERNS=(          # Extra block patterns (hook mode / fallback)
#       "docker rm"
#   )

COMMAND="$*"

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
SUPERVISOR_MODEL="claude-haiku-4-5-20251001"
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

# Verify conf is owned by current user and not world-writable before sourcing
if [[ -f "$CONFIG_FILE" ]]; then
    conf_owner=$(stat -c "%u" "$CONFIG_FILE" 2>/dev/null || stat -f "%u" "$CONFIG_FILE" 2>/dev/null)
    conf_mode=$(stat -c "%a" "$CONFIG_FILE" 2>/dev/null || stat -f "%OLp" "$CONFIG_FILE" 2>/dev/null)
    if [[ "$conf_owner" != "$(id -u)" ]] || [[ "${conf_mode: -1}" =~ [2367] ]]; then
        echo "c4-security-hook: WARNING: $CONFIG_FILE is not trusted (owner/perms), skipping." >&2
    else
        # shellcheck source=/dev/null
        source "$CONFIG_FILE"
    fi
fi

# =============================================================================
# User Whitelist — always checked first, regardless of mode
# =============================================================================
for pattern in "${ALLOW_PATTERNS[@]}"; do
    if [[ "$COMMAND" =~ $pattern ]]; then
        exit 0
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
            jq -n --arg reason "$reason" '{
                decision: "block",
                reason: ("AI supervisor (Haiku) blocked: " + $reason),
                instructions: "The model determined this command may be unsafe. Run manually if you are sure."
            }'
            exit 2
        fi

        # Model says safe
        exit 0
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
        cat << EOF
{
    "decision": "block",
    "reason": "Blocked by user config: matches '$pattern'",
    "instructions": "This command is blocked by your c4-bash-security.conf. Remove the pattern to allow."
}
EOF
        exit 2
    fi
done

# System destruction
if [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+/ ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+~ ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\* ]] || \
   [[ "$COMMAND" =~ rm[[:space:]]+-rf?[[:space:]]+\.\* ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Recursive delete of system/home directory",
    "instructions": "This command could delete critical files. Please specify exact paths or ask user to run manually."
}
EOF
    exit 2
fi

# Sudo with dangerous commands
if [[ "$COMMAND" =~ sudo[[:space:]]+(rm|chmod|chown|mkfs|dd|truncate|shred) ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: sudo with potentially destructive command",
    "instructions": "Running destructive commands with sudo is not allowed. Ask user to run manually if needed."
}
EOF
    exit 2
fi

# Dangerous permission changes
if [[ "$COMMAND" =~ chmod[[:space:]]+(-R[[:space:]]+)?777 ]] || \
   [[ "$COMMAND" =~ chmod[[:space:]]+(-R[[:space:]]+)?a\+rwx ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Setting world-writable permissions",
    "instructions": "chmod 777 is a security risk. Use more restrictive permissions like 755 or 644."
}
EOF
    exit 2
fi

# Disk/filesystem operations
if [[ "$COMMAND" =~ mkfs ]] || \
   [[ "$COMMAND" =~ dd[[:space:]]+if= ]] || \
   [[ "$COMMAND" =~ \>[[:space:]]*/dev/ ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Direct disk/filesystem operation",
    "instructions": "Disk operations are blocked for safety. Ask user to run manually."
}
EOF
    exit 2
fi

# Fork bomb and similar
if [[ "$COMMAND" =~ :\(\)\{[[:space:]]*:\|: ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Potential fork bomb detected",
    "instructions": "This pattern could crash the system."
}
EOF
    exit 2
fi

# Git force push to main/master
if [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(-f|--force)[[:space:]]+(origin[[:space:]]+)?(main|master) ]] || \
   [[ "$COMMAND" =~ git[[:space:]]+push[[:space:]]+(origin[[:space:]]+)?(main|master)[[:space:]]+(-f|--force) ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Force push to main/master branch",
    "instructions": "Force pushing to main/master can cause data loss. Use a feature branch or ask user to run manually."
}
EOF
    exit 2
fi

# Remote code execution via curl/wget pipe
if [[ "$COMMAND" =~ (curl|wget)[[:space:]].*\|[[:space:]]*(sh|bash|zsh|python|perl|ruby) ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Piping remote content to shell",
    "instructions": "Executing remote scripts directly is dangerous. Download first, review, then execute."
}
EOF
    exit 2
fi

# Eval with command substitution (potential injection)
if [[ "$COMMAND" =~ eval[[:space:]]+\$\( ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: eval with command substitution",
    "instructions": "eval with dynamic content can be exploited. Use safer alternatives."
}
EOF
    exit 2
fi

# Writing to system config directories
if [[ "$COMMAND" =~ \>[[:space:]]*/etc/ ]] || \
   [[ "$COMMAND" =~ tee[[:space:]]+/etc/ ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Writing to /etc/ directory",
    "instructions": "System configuration changes require manual user action."
}
EOF
    exit 2
fi

# npm/pnpm publish without explicit flag
if [[ "$COMMAND" =~ (npm|pnpm)[[:space:]]+publish ]] && \
   [[ ! "$COMMAND" =~ --dry-run ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: Publishing to npm registry",
    "instructions": "Publishing packages requires user confirmation. Use --dry-run to test first."
}
EOF
    exit 2
fi

# Git hard reset
if [[ "$COMMAND" =~ git[[:space:]]+reset[[:space:]]+--hard ]]; then
    cat << 'EOF'
{
    "decision": "block",
    "reason": "Blocked: git reset --hard can lose uncommitted changes",
    "instructions": "Use git stash or commit changes first. Ask user if hard reset is truly needed."
}
EOF
    exit 2
fi

# =============================================================================
# All checks passed - allow command
# =============================================================================
exit 0

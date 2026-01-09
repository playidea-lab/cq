#!/bin/bash
# C4 Bash Security Hook
# Blocks dangerous commands to prevent accidental system damage
#
# Exit codes:
#   0 - Allow command
#   2 - Block command (with JSON response)
#
# Customization:
#   Create ~/.claude/hooks/c4-bash-security.conf to customize:
#
#   # Allow specific commands (regex patterns, one per line)
#   ALLOW_PATTERNS=(
#       "rm -rf /tmp/test"
#       "npm publish --dry-run"
#   )
#
#   # Block additional commands
#   BLOCK_PATTERNS=(
#       "docker rm"
#       "kubectl delete"
#   )

COMMAND="$*"

# Skip empty commands
if [[ -z "$COMMAND" ]]; then
    exit 0
fi

# =============================================================================
# User Configuration (optional)
# =============================================================================
CONFIG_FILE="$HOME/.claude/hooks/c4-bash-security.conf"

# Initialize arrays
ALLOW_PATTERNS=()
BLOCK_PATTERNS=()

# Load user config if exists
if [[ -f "$CONFIG_FILE" ]]; then
    source "$CONFIG_FILE"
fi

# Check user allow patterns first (whitelist takes priority)
for pattern in "${ALLOW_PATTERNS[@]}"; do
    if [[ "$COMMAND" =~ $pattern ]]; then
        exit 0  # Explicitly allowed
    fi
done

# Check user block patterns
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

# =============================================================================
# Dangerous Patterns - BLOCK these commands
# =============================================================================

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

#!/bin/bash
# Global AntiPattern Hook (PreToolUse)
# Blocks common Python anti-patterns: pip install, python *.py, bare pytest.
# Applies to ALL Claude projects (global ~/.claude/hooks/ registration).
#
# Exit codes:
#   0 - No decision (allow)
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
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)

# Only check Bash tool
[[ "$TOOL_NAME" != "Bash" ]] && exit 0
[[ -z "$COMMAND" ]] && exit 0

# =============================================================================
# Helper emit functions
# =============================================================================
_emit_deny() {
    local reason="${1:-Blocked}"
    jq -n --arg reason "$reason" '{
        hookSpecificOutput: {
            hookEventName: "PreToolUse",
            permissionDecision: "deny",
            permissionDecisionReason: $reason
        }
    }' 2>/dev/null || {
        local _r="${reason//\\/\\\\}"; _r="${_r//\"/\\\"}"
        echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"$_r\"}}"
    }
    exit 2
}

# =============================================================================
# Anti-pattern rules
# =============================================================================

# REQ-001: pip install → uv add
# Block: pip install <pkg>, pip3 install <pkg>
# Allow: uv pip install (uv's own pip wrapper)
if [[ "$COMMAND" =~ ^pip[[:space:]]+install ]] || \
   [[ "$COMMAND" =~ ^pip3[[:space:]]+install ]]; then
    _emit_deny "pip install 금지. 대신: uv add <패키지> 또는 uv pip install <패키지>"
fi

# REQ-002: python *.py / python3 *.py → uv run python
# Block: python script.py, python3 script.py, python3.11 script.py (bare, no uv run prefix)
# Allow: uv run python ..., echo | python, python --version, python -c "...", python -m module
if [[ "$COMMAND" =~ (^|[[:space:];&|])(python[0-9.]*)[[:space:]]+[^-] ]]; then
    # Exclude: uv run python, python --version/-V, python -c/python -m
    if ! [[ "$COMMAND" =~ uv[[:space:]]+run[[:space:]]+python ]] && \
       ! [[ "$COMMAND" =~ python[0-9.]*[[:space:]]+(--version|-V|-c|-m) ]]; then
        _emit_deny "python *.py 직접 실행 금지. 대신: uv run python <스크립트>"
    fi
fi

# REQ-003: bare pytest → uv run pytest
# Block: pytest, pytest -v, pytest tests/
# Allow: uv run pytest, python -m pytest
if [[ "$COMMAND" =~ (^|[[:space:];&|])pytest([[:space:]]|$) ]]; then
    if ! [[ "$COMMAND" =~ uv[[:space:]]+run[[:space:]]+pytest ]] && \
       ! [[ "$COMMAND" =~ python[[:space:]]+-m[[:space:]]+pytest ]]; then
        _emit_deny "pytest 직접 실행 금지. 대신: uv run pytest"
    fi
fi

# No pattern matched — allow
exit 0

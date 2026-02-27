#!/usr/bin/env bats
# test_c4gate_mcp.bats
# Tests for .claude/hooks/c4-gate.sh — MCP tool blocking (TodoWrite/EnterPlanMode).
# Requires CLAUDE_PROJECT_DIR or a .c4/ directory reachable from the working dir.

# Resolve project root: look for .claude/hooks/c4-gate.sh walking up from this file
_find_project_root() {
    # Prefer CLAUDE_PROJECT_DIR if already set and valid
    if [[ -n "${CLAUDE_PROJECT_DIR:-}" ]] && \
       [[ -f "${CLAUDE_PROJECT_DIR}/.claude/hooks/c4-gate.sh" ]]; then
        echo "$CLAUDE_PROJECT_DIR"
        return 0
    fi
    local dir
    dir="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
    while [[ "$dir" != "/" ]]; do
        [[ -f "$dir/.claude/hooks/c4-gate.sh" ]] && echo "$dir" && return 0
        dir="${dir%/*}"
    done
    return 1
}

setup() {
    PROJECT_ROOT=$(_find_project_root) || skip "No .c4/ directory found — not a C4 project"
    HOOK="$PROJECT_ROOT/.claude/hooks/c4-gate.sh"
    [[ -f "$HOOK" ]] || skip "c4-gate.sh not found at $HOOK"
    export CLAUDE_PROJECT_DIR="$PROJECT_ROOT"
}

_run_hook() {
    local json="$1"
    run bash -c "echo $(printf '%q' "$json") | CLAUDE_PROJECT_DIR='$CLAUDE_PROJECT_DIR' bash '$HOOK'"
}

# ─── TodoWrite 차단 ──────────────────────────────────────────────────────────

@test "TodoWrite → denied (exit 2)" {
    _run_hook '{"tool_name":"TodoWrite","tool_input":{"todos":[]}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "TodoWrite deny message mentions c4_add_todo" {
    _run_hook '{"tool_name":"TodoWrite","tool_input":{"todos":[]}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "c4_add_todo" ]]
}

# ─── EnterPlanMode 차단 ──────────────────────────────────────────────────────

@test "EnterPlanMode → denied (exit 2)" {
    _run_hook '{"tool_name":"EnterPlanMode","tool_input":{}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "EnterPlanMode deny message mentions c4-plan" {
    _run_hook '{"tool_name":"EnterPlanMode","tool_input":{}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "c4-plan" ]]
}

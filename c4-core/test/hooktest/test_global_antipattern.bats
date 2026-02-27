#!/usr/bin/env bats
# test_global_antipattern.bats
# Tests for ~/.claude/hooks/global-antipattern.sh
# Verifies pip/python/pytest anti-pattern blocking.

HOOK="$HOME/.claude/hooks/global-antipattern.sh"

setup() {
    if [[ ! -f "$HOOK" ]]; then
        skip "global-antipattern.sh not installed at $HOOK"
    fi
}

_run_hook() {
    local json="$1"
    run bash -c "echo $(printf '%q' "$json") | bash '$HOOK'"
}

# ─── pip / pip3 차단 (REQ-001) ───────────────────────────────────────────────

@test "pip install requests → denied (exit 2)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"pip install requests"}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "pip3 install pandas → denied (exit 2)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"pip3 install pandas"}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "uv pip install requests → allowed (exit 0)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"uv pip install requests"}}'
    [ "$status" -eq 0 ]
}

# ─── python *.py 차단 (REQ-002) ──────────────────────────────────────────────

@test "python train.py → denied (exit 2)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"python train.py"}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "uv run python train.py → allowed (exit 0)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"uv run python train.py"}}'
    [ "$status" -eq 0 ]
}

@test "python --version → allowed (exit 0)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"python --version"}}'
    [ "$status" -eq 0 ]
}

# ─── pytest 차단 (REQ-003) ───────────────────────────────────────────────────

@test "pytest tests/ → denied (exit 2)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"pytest tests/"}}'
    [ "$status" -eq 2 ]
    [[ "$output" =~ "deny" ]]
}

@test "uv run pytest → allowed (exit 0)" {
    _run_hook '{"tool_name":"Bash","tool_input":{"command":"uv run pytest"}}'
    [ "$status" -eq 0 ]
}

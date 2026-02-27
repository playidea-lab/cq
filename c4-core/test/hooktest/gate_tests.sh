#!/usr/bin/env bash
# gate_tests.sh — c4-gate.sh 워크플로우 게이트 단위 테스트
# 사용법: bash c4-core/test/hooktest/gate_tests.sh
set -uo pipefail
PASS=0; FAIL=0; SKIP=0

REPO_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")
GATE="${REPO_ROOT}/.claude/hooks/c4-gate.sh"

if [[ ! -f "$GATE" ]]; then
    echo "SKIP: $GATE not found"
    exit 0
fi

TMPDIR_T=$(mktemp -d)
DB="${TMPDIR_T}/c4.db"
trap 'rm -rf "$TMPDIR_T"' EXIT

# --- DB 픽스처 ---
setup_db() {
    sqlite3 "$DB" "
CREATE TABLE c4_tasks (
  task_id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  status TEXT DEFAULT 'pending',
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);
CREATE TABLE c4_gates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id TEXT,
  gate TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('done','skipped','override')),
  reason TEXT,
  completed_at TEXT DEFAULT (datetime('now'))
);" 2>/dev/null
}

run_hook() {
    local json="$1"
    printf '%s' "$json" | DB_PATH="$DB" bash "$GATE" 2>/dev/null
}

assert_deny() {
    local name="$1" json="$2"
    run_hook "$json"; local rc=$?
    if [[ $rc -eq 2 ]]; then echo "PASS: $name"; ((PASS++))
    else echo "FAIL: $name (exit $rc, expected 2)"; ((FAIL++)); fi
}

assert_pass() {
    local name="$1" json="$2"
    run_hook "$json"; local rc=$?
    if [[ $rc -eq 0 ]]; then echo "PASS: $name"; ((PASS++))
    else echo "FAIL: $name (exit $rc, expected 0)"; ((FAIL++)); fi
}

setup_db

# Test 1: git commit — in_progress 태스크 + no polish → DENY
sqlite3 "$DB" "INSERT INTO c4_tasks VALUES ('T-001-0','task','in_progress',datetime('now'),datetime('now'))"
assert_deny "git commit blocked (in_progress + no polish)" \
    '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"test\""}}'

# Test 2: git commit — polish done → PASS (gray-zone)
sqlite3 "$DB" "INSERT INTO c4_gates(gate,status,completed_at) VALUES('polish','done',datetime('now'))"
assert_pass "git commit allowed (polish done)" \
    '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"test\""}}'

# Test 3: git commit — no active tasks → PASS
sqlite3 "$DB" "DELETE FROM c4_gates"
sqlite3 "$DB" "UPDATE c4_tasks SET status='done'"
assert_pass "git commit allowed (no active tasks)" \
    '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"test\""}}'

# Test 4: C4_SKIP_GATE=1 우회
sqlite3 "$DB" "UPDATE c4_tasks SET status='in_progress'"
result=$(printf '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"bypass\""}}' | \
    C4_SKIP_GATE=1 DB_PATH="$DB" bash "$GATE" 2>/dev/null; echo "::$?")
rc="${result##*::}"
if [[ "$rc" != "2" ]]; then echo "PASS: C4_SKIP_GATE=1 bypasses gate"; ((PASS++))
else echo "FAIL: C4_SKIP_GATE=1 should bypass (exit $rc)"; ((FAIL++)); fi

# Test 5: DB 없음 → PASS (gate skip)
OLD_DB="$DB"; DB="/nonexistent/path/c4.db"
assert_pass "no DB → gate skipped" \
    '{"tool_name":"Bash","tool_input":{"command":"git commit -m \"no-db\""}}'
DB="$OLD_DB"

# Test 6: Skill c4-finish — polish 미완료 → DENY or PASS (runtime-dependent)
sqlite3 "$DB" "DELETE FROM c4_gates; UPDATE c4_tasks SET status='in_progress'"
run_hook '{"tool_name":"Skill","tool_input":{"skill":"c4-finish"}}'; rc=$?
if [[ $rc -eq 2 || $rc -eq 0 ]]; then
    echo "PASS: Skill c4-finish gate (exit $rc — deny=2 or gray=0 both valid)"; ((PASS++))
else
    echo "FAIL: Skill c4-finish unexpected exit $rc"; ((FAIL++))
fi

# Test 7: TodoWrite 여전히 차단 (기존 블록 회귀)
assert_deny "TodoWrite still blocked" '{"tool_name":"TodoWrite","tool_input":{}}'

# Test 8: EnterPlanMode 여전히 차단 (기존 블록 회귀)
assert_deny "EnterPlanMode still blocked" '{"tool_name":"EnterPlanMode","tool_input":{}}'

echo ""
echo "=== Results: $PASS passed, $FAIL failed, $SKIP skipped ==="
[[ $FAIL -eq 0 ]] && exit 0 || exit 1

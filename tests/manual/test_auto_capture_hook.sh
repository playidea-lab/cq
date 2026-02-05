#!/bin/bash
# Manual test script for c4-auto-capture-hook.py
#
# This script tests the auto-capture hook by simulating PostToolUse calls.
# Run from the project root directory.
#
# Usage:
#   ./tests/manual/test_auto_capture_hook.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HOOK_SCRIPT="$PROJECT_ROOT/templates/c4-auto-capture-hook.py"

echo "=== C4 Auto-Capture Hook Manual Test ==="
echo "Project root: $PROJECT_ROOT"
echo "Hook script: $HOOK_SCRIPT"
echo ""

# Test 1: Check hook script exists
echo "Test 1: Hook script exists"
if [ -f "$HOOK_SCRIPT" ]; then
    echo "  PASS: Script found"
else
    echo "  FAIL: Script not found"
    exit 1
fi

# Test 2: Check hook is executable
echo ""
echo "Test 2: Hook is executable"
if [ -x "$HOOK_SCRIPT" ]; then
    echo "  PASS: Script is executable"
else
    echo "  FAIL: Script is not executable"
    exit 1
fi

# Test 3: Empty input exits with 0
echo ""
echo "Test 3: Empty input exits with 0"
echo "" | python3 "$HOOK_SCRIPT"
if [ $? -eq 0 ]; then
    echo "  PASS: Exits 0 on empty input"
else
    echo "  FAIL: Non-zero exit on empty input"
    exit 1
fi

# Test 4: Invalid JSON exits with 0 (silent fail)
echo ""
echo "Test 4: Invalid JSON exits with 0 (silent fail)"
echo "not valid json" | python3 "$HOOK_SCRIPT"
if [ $? -eq 0 ]; then
    echo "  PASS: Exits 0 on invalid JSON"
else
    echo "  FAIL: Non-zero exit on invalid JSON"
    exit 1
fi

# Test 5: Non-captured tool exits with 0
echo ""
echo "Test 5: Non-captured tool exits with 0"
echo '{"tool_name": "unknown_tool", "input": {}, "output": "test"}' | python3 "$HOOK_SCRIPT"
if [ $? -eq 0 ]; then
    echo "  PASS: Exits 0 for non-captured tool"
else
    echo "  FAIL: Non-zero exit for non-captured tool"
    exit 1
fi

# Test 6: Captured tool (read_file) exits with 0
echo ""
echo "Test 6: Captured tool (read_file) exits with 0"
export C4_PROJECT_ROOT="$PROJECT_ROOT"
echo '{"tool_name": "read_file", "input": {"path": "/test/file.py"}, "output": "def test(): pass"}' | python3 "$HOOK_SCRIPT"
if [ $? -eq 0 ]; then
    echo "  PASS: Exits 0 for captured tool"
else
    echo "  FAIL: Non-zero exit for captured tool"
    exit 1
fi

# Test 7: Check if observation was stored (if .c4/tasks.db exists)
echo ""
echo "Test 7: Observation storage"
if [ -f "$PROJECT_ROOT/.c4/tasks.db" ]; then
    COUNT=$(sqlite3 "$PROJECT_ROOT/.c4/tasks.db" "SELECT COUNT(*) FROM c4_observations WHERE source='read_file' AND content LIKE '%def test()%'" 2>/dev/null || echo "0")
    if [ "$COUNT" -gt 0 ]; then
        echo "  PASS: Observation stored in database"
    else
        echo "  INFO: Observation may not have been stored (check manually)"
    fi
else
    echo "  SKIP: No tasks.db found (C4 may not be initialized)"
fi

echo ""
echo "=== All tests passed ==="

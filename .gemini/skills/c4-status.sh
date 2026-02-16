#!/bin/bash
# c4-status for Gemini CLI
# Usage: ./c4-status.sh

echo "📊 Checking C4 Project Status..."

# 1. Check Git Status
echo "## Git Status:"
git status -s

# 2. Check Active Tasks (c5)
if [ -f "c5/c5.db" ]; then
    echo "## Active Tasks (from c5.db):"
    if command -v sqlite3 >/dev/null; then
        sqlite3 c5/c5.db "SELECT id, title, status FROM tasks WHERE status IN ('IN_PROGRESS', 'BLOCKED');" || echo "No active tasks found."
    else
        echo "sqlite3 not found, skipping DB check."
    fi
else
    echo "## No c5.db found (System idle)."
fi

# 3. Check for validation blocks
if [ -f ".gemini/memory/scratchpad.md" ]; then
    echo "## Recent Scratchpad Activity:"
    tail -n 5 .gemini/memory/scratchpad.md
fi

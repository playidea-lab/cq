#!/bin/bash
# PI Lab Stop Guard Hook
# Blocks session exit if C4 tasks are in progress.
#
# Exit codes:
#   0 - Allow exit
#   2 - Block exit (pending work)

[[ ! -f ".c4/state.json" ]] && exit 0

# Check for in-progress tasks
if command -v sqlite3 &>/dev/null && [[ -f ".c4/tasks.db" ]]; then
    in_progress=$(sqlite3 .c4/tasks.db "SELECT COUNT(*) FROM c4_tasks WHERE status='in_progress'" 2>/dev/null)
    if [[ "$in_progress" -gt 0 ]]; then
        cat << EOF
{
    "decision": "block",
    "reason": "$in_progress task(s) still in progress",
    "instructions": "Complete or submit pending tasks before exiting."
}
EOF
        exit 2
    fi
fi

exit 0

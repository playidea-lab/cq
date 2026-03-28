#!/bin/bash
# PI Lab Session Close Hook (SessionEnd)
# Triggers session close pipeline: status→done + summarize + knowledge + persona.
# Runs in background to avoid blocking session exit. Always exits 0.
#
# Reads JSON from stdin (Claude Code SessionEnd hook protocol):
#   { "session_id": "...", "cwd": "..." }

# --- Require jq ---
if ! command -v jq &>/dev/null; then
    exit 0
fi

# --- Read SessionEnd hook JSON from stdin ---
INPUT=$(cat)

SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // empty' 2>/dev/null)
CWD=$(echo "$INPUT" | jq -r '.cwd // empty' 2>/dev/null)

# Fallback to env vars
if [[ -z "$SESSION_ID" ]]; then
    SESSION_ID="${CLAUDE_SESSION_ID:-}"
fi

# Nothing to close if no session ID
if [[ -z "$SESSION_ID" ]]; then
    exit 0
fi

# --- Locate cq binary ---
CQ_BIN=""
for candidate in \
    "$HOME/.local/bin/cq" \
    "/usr/local/bin/cq" \
    "$(command -v cq 2>/dev/null)"; do
    if [[ -n "$candidate" && -x "$candidate" ]]; then
        CQ_BIN="$candidate"
        break
    fi
done

if [[ -z "$CQ_BIN" ]]; then
    exit 0
fi

# --- Determine project directory ---
PROJECT_DIR="${CWD:-${CLAUDE_PROJECT_DIR:-$PWD}}"

# --- Run session close in background ---
(
    "$CQ_BIN" session close \
        --session-id "$SESSION_ID" \
        --dir "$PROJECT_DIR" \
        >/dev/null 2>&1
) &

# Disown so the subprocess survives session exit
disown $! 2>/dev/null

exit 0

#!/usr/bin/env python3
"""C4 Stop Hook - Check if work remains and block exit if needed.

This script is called by Claude Code's stop hook mechanism.
Exit codes:
  0 = Allow exit (no pending work)
  2 = Block exit (work remains, continue working)
"""

import json
import sqlite3
import sys
from pathlib import Path


def load_state_from_sqlite(db_path: Path) -> dict | None:
    """Load state from SQLite database."""
    try:
        conn = sqlite3.connect(db_path)
        cursor = conn.execute("SELECT state_json FROM c4_state LIMIT 1")
        row = cursor.fetchone()
        conn.close()
        if row:
            return json.loads(row[0])
    except (sqlite3.Error, json.JSONDecodeError):
        pass
    return None


def load_state_from_json(json_path: Path) -> dict | None:
    """Load state from JSON file (legacy)."""
    try:
        return json.loads(json_path.read_text())
    except (json.JSONDecodeError, OSError):
        pass
    return None


def main() -> None:
    """Check C4 state and determine if exit should be blocked."""
    db_file = Path(".c4/c4.db")
    json_file = Path(".c4/state.json")

    # Try SQLite first (new default), then JSON (legacy)
    state = None
    if db_file.exists():
        state = load_state_from_sqlite(db_file)
    elif json_file.exists():
        state = load_state_from_json(json_file)

    # C4 not initialized or can't read state - allow exit
    if state is None:
        sys.exit(0)

    status = state.get("status", "")

    # EXECUTE state: block if tasks remain
    if status == "EXECUTE":
        tasks = state.get("tasks", [])
        pending = [t for t in tasks if t.get("status") == "pending"]
        in_progress = [t for t in tasks if t.get("status") == "in_progress"]

        if pending or in_progress:
            msg = f"{len(pending)} pending, {len(in_progress)} in_progress tasks remain"
            print(msg)
            sys.exit(2)

    # CHECKPOINT state: block if queue has items (supervisor processing)
    if status == "CHECKPOINT":
        cp_queue = state.get("checkpoint_queue", [])
        if cp_queue:
            cp_id = cp_queue[0].get("checkpoint_id", "unknown")
            print(f"Checkpoint {cp_id} awaiting supervisor review")
            sys.exit(2)

    # All other states (COMPLETE, BLOCKED, PLAN, HALTED, INIT) - allow exit
    sys.exit(0)


if __name__ == "__main__":
    main()

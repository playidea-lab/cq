#!/usr/bin/env python3
"""C4 Stop Hook - Check if work remains and block exit if needed.

This script is called by Claude Code's stop hook mechanism.
Exit codes:
  0 = Allow exit (no pending work)
  2 = Block exit (work remains, continue working)
"""

import json
import sys
from pathlib import Path


def main() -> None:
    """Check C4 state and determine if exit should be blocked."""
    state_file = Path(".c4/state.json")

    # C4 not initialized - allow exit
    if not state_file.exists():
        sys.exit(0)

    try:
        state = json.loads(state_file.read_text())
    except (json.JSONDecodeError, OSError):
        # Can't read state - allow exit
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

---
description: |
  Spawn C4 workers to execute implementation tasks in parallel. Analyzes task
  dependency graph, calculates optimal worker count, and spawns background workers
  with fresh context isolation (one task per worker). Supports auto mode (smart
  parallelism), manual worker count, continuous mode (auto-respawn), and max limits.
  Use when ready to execute C4 tasks. Triggers: "run tasks", "execute plan", "spawn
  workers", "start implementation", "/c4-run".
---

# C4 Run — Smart Auto Worker Spawner

Execute C4 tasks with dependency-aware parallel workers.

## Usage

```
/c4-run             # Auto: analyze graph, spawn optimal workers (1-round)
/c4-run 1           # Spawn 1 worker (background)
/c4-run 3           # Spawn 3 workers (background)
/c4-run --max 4     # Auto mode, capped at 4 workers
/c4-run --continuous  # 🔄 Continuous: auto-respawn until queue empty
```

## 🔄 Single-Task Worker Model

**Each Worker processes ONE task and exits** (context isolation principle).

```
┌──────────────────────────────────────────────────┐
│  Orchestrator (/c4-run)                          │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐          │
│  │Worker 1 │  │Worker 2 │  │Worker 3 │  ...     │
│  │ Task A  │  │ Task B  │  │ Task C  │          │
│  │  EXIT   │  │  EXIT   │  │  EXIT   │          │
│  └─────────┘  └─────────┘  └─────────┘          │
│       ↓            ↓            ↓               │
│  [Fresh Context] [Fresh Context] [Fresh Context]│
│       ↓            ↓            ↓               │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐          │
│  │Worker 4 │  │Worker 5 │  │Worker 6 │  ...     │
│  │ Task D  │  │ Task E  │  │ Task F  │          │
│  └─────────┘  └─────────┘  └─────────┘          │
└──────────────────────────────────────────────────┘
```

**Why?**
- Prevents context accumulation (12+ tasks → worker death)
- Fresh context per task
- Task failures isolated

**All workers run in background.** Main session remains available for user interaction.

## Instructions

### ⚠️ Pre-Flight Checks

1. **MCP Tools Only** — NEVER use CLI commands:
   - `mcp__c4__c4_status()` - status + parallelism analysis
   - `mcp__c4__c4_start()` - PLAN/HALTED → EXECUTE
   - `mcp__c4__c4_get_task(worker_id)` - task assignment
   - `mcp__c4__c4_submit(task_id, commit_sha, validation_results)` - submit
   - `mcp__c4__c4_run_validation(names)` - validation

2. **Accept Edits Mode** — Verify enabled (bottom status bar or `Shift+Tab`).
   ⚠️ If off, automation breaks on every file edit!

### 0. Generate Worker ID (Required!)

**Before spawning**, generate unique worker ID:

```python
import uuid
WORKER_ID = f"worker-{uuid.uuid4().hex[:8]}"  # e.g., "worker-a1b2c3d4"
```

Use this ID for the entire session. **NEVER use hardcoded values like "claude-worker"!**

### 1. Status Check + Parallelism Analysis

```python
status = mcp__c4__c4_status()

# Parallelism info
parallelism = status["parallelism"]
# {
#   "recommended": 4,        # Recommended worker count
#   "ready_now": 6,          # Tasks ready to run now
#   "max_parallelism": 5,    # DAG max width
#   "by_model": {"opus": 3, "sonnet": 3},  # Model distribution
#   "pending_total": 10,     # Total pending
#   "blocked_count": 4,      # Dependency-blocked
#   "reason": "6 tasks ready, capped at 4 workers"
# }
```

**State-based routing**:
- **PLAN/HALTED**: → Step 2 (transition to EXECUTE)
- **EXECUTE**: → Step 3 (spawn workers)
- **CHECKPOINT**: Output "Checkpoint review pending." → exit
- **COMPLETE**: Output "Project complete." → exit
- **INIT**: Output "Run /c4-plan first." → exit

### 2. PLAN/HALTED State: Transition to EXECUTE

```python
result = mcp__c4__c4_start()
# result.success == true, result.status == "EXECUTE"
```

### 3. Worker Count Decision (Smart Auto)

```python
# Parse ARGUMENTS
args = "$ARGUMENTS".strip()
continuous_mode = "--continuous" in args

if args == "" or args == "--auto":
    # Auto mode: use recommended
    worker_count = parallelism["recommended"]
elif "--continuous" in args:
    # Continuous mode: spawn ready_now
    worker_count = parallelism["ready_now"]
elif args.startswith("--max"):
    # Max cap
    max_workers = int(args.split()[-1])
    worker_count = min(parallelism["recommended"], max_workers)
else:
    # Direct number
    worker_count = int(args)

# Cap at 7 (Claude Code subagent limit)
worker_count = min(worker_count, 7)

# Print analysis
print(f"""
📊 Parallelism Analysis:
   Total: {parallelism['pending_total']} tasks
   Ready: {parallelism['ready_now']} tasks
   Blocked: {parallelism['blocked_count']} (deps unmet)
   DAG max width: {parallelism['max_parallelism']}

💡 Recommended: {parallelism['recommended']} workers
   Reason: {parallelism['reason']}

🚀 Executing: {worker_count} workers
""")
```

### 4. Spawn Workers

**All workers spawn as background subagents** (main session remains interactive).

```python
import uuid

WORKER_PROMPT = """
You are C4 Worker {worker_id}.

## Mission
Execute **ONE** C4 task and exit. (Context isolation principle)

## MCP Tools (MUST USE)
- `mcp__c4__c4_get_task(worker_id="{worker_id}")` - request task
- `mcp__c4__c4_run_validation(names=["lint", "unit"])` - validation
- `mcp__c4__c4_submit(task_id, worker_id, commit_sha, validation_results)` - submit

## ⚠️ Single Task Protocol (Context Isolation!)

```
1. task = c4_get_task(worker_id="{worker_id}")
2. IF task is None or no task_id:
       PRINT "✅ No tasks available"
       EXIT
3. Implement the task (follow DoD)
4. Run validations, fix issues (max 3 retries)
5. git commit
6. c4_submit(task_id, ...)
7. EXIT (✅ Task complete - fresh context for next task!)
```

**CRITICAL**: Exit after ONE task completion!
Next task → new Worker → fresh context → prevents context death.

## Your Worker ID: {worker_id}

START NOW: Call `mcp__c4__c4_get_task(worker_id="{worker_id}")`, complete ONE task, then exit!
"""

workers = []
for i in range(worker_count):
    worker_id = f"worker-{uuid.uuid4().hex[:8]}"

    # Model selection (from by_model distribution or default opus)
    model = "opus"  # default

    result = Task(
        subagent_type="general-purpose",
        description=f"C4 Worker {i+1}/{worker_count}",
        prompt=WORKER_PROMPT.format(worker_id=worker_id),
        model=model,
        run_in_background=True
    )

    workers.append({"id": worker_id, "output": result.output_file})
    print(f"🚀 Worker {i+1}/{worker_count} spawned: {worker_id}")

print(f"""
🐝 C4 Run: {worker_count} workers spawned (background)

Workers:
""")
for w in workers:
    print(f"  • {w['id']}: {w['output']}")

if not continuous_mode:
    print("""
## ⚠️ Single-Task Worker Model

Each Worker processes **ONE task** and exits.
(Prevents context accumulation → prevents worker death)

Monitor:
  /c4-status - overall progress
  tail -f {output_file} - individual worker logs

Next Steps:
  • After workers complete, run `/c4-run` again if tasks remain
  • Or use `/c4-run --continuous` for auto-respawn mode
""")
else:
    # 🔄 Continuous Mode: Monitor and respawn
    print("""
## 🔄 Continuous Mode Active

Auto-respawns workers until queue exhausted.
Ctrl+C to interrupt.
""")

    import time

    while True:
        # Wait 30s (worker execution time)
        time.sleep(30)

        # Re-check status
        status = mcp__c4__c4_status()

        # Completion conditions
        if status["status"] == "COMPLETE":
            print("🎉 All tasks complete!")
            break

        if status["status"] == "CHECKPOINT":
            print("⏸️ Checkpoint review pending. Run /c4-checkpoint.")
            break

        # Check ready tasks
        ready = status["parallelism"]["ready_now"]
        if ready == 0:
            if status["queue"]["pending"] == 0:
                print("✅ All tasks processed!")
                break
            else:
                print(f"⏳ {status['queue']['pending']} tasks pending (deps unmet)...")
                continue

        # Spawn new workers (up to ready count)
        spawn_count = min(ready, 7 - len([w for w in status["workers"].values() if w["state"] == "busy"]))
        if spawn_count > 0:
            print(f"🚀 Spawning {spawn_count} more workers...")
            for i in range(spawn_count):
                worker_id = f"worker-{uuid.uuid4().hex[:8]}"
                Task(
                    subagent_type="general-purpose",
                    description=f"C4 Worker (continuous)",
                    prompt=WORKER_PROMPT.format(worker_id=worker_id),
                    model="opus",
                    run_in_background=True
                )
                print(f"  • {worker_id}")

    print("🏁 Continuous mode ended")
```

## 🌲 Worktree Isolation (Multi-Worker Requirement!)

**Prevents branch conflicts when multiple workers operate on same project.**

`c4_get_task()` response includes `worktree_path`:

```python
task = c4_get_task(WORKER_ID)
# task.worktree_path: ".c4/worktrees/worker-abc123"  ← Use this path!
# task.branch: "c4/w-T-001-0"
```

**All file operations MUST occur within worktree_path**:

```python
if task.worktree_path:
    work_dir = Path(task.worktree_path)
    file_to_edit = work_dir / "src" / "module.py"
    Read(file_to_edit)
    Edit(file_to_edit, ...)
```

## 🤖 Agent Routing

`c4_get_task()` response includes agent routing info:

```python
task = c4_get_task(WORKER_ID)
# task.recommended_agent: "frontend-developer"
# task.agent_chain: ["frontend-developer", "test-automator", "code-reviewer"]
```

Worker auto-selects appropriate agent.

## Expected Flows

### Auto Mode (Default)
```
/c4-run
→ Status: EXECUTE
→ Parallelism analysis: 5 tasks ready, DAG width 4
→ Recommended: 4 workers
→ 🚀 Spawn 4 workers
→ Workers process tasks in parallel
→ ✅ All tasks complete
```

### Single Worker Mode
```
/c4-run 1
→ Status: EXECUTE
→ Parallelism analysis: (display only)
→ 🚀 Spawn 1 worker (background)
→ Worker processes task in background
→ Main session available for other work
→ ✅ All tasks complete
```

## Constraints

| Constraint | Description |
|------------|-------------|
| Max Workers | 7 (Claude Code subagent limit) |
| Worktree | Required for multi-worker |
| Accept Edits | Required for automation |

## Related Skills

- `/c4-status` - Check status (includes parallelism analysis)
- `/c4-stop` - Stop execution
- `/c4-submit` - Manual submission

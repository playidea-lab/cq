---
name: c4-run
description: |
  Spawn C4 workers to execute implementation tasks in parallel. **Always runs in
  continuous mode** (auto-respawn until queue empty). Analyzes task dependency
  graph, spawns workers with fresh context isolation (one task per worker).
  Use when ready to execute C4 tasks. Triggers: "실행", "워커 실행", "태스크 실행",
  "run tasks", "execute plan", "spawn workers", "start implementation", "/c4-run".
---

# C4 Run — Continuous Auto Worker Spawner

Execute C4 tasks with dependency-aware parallel workers. **Defaults to continuous mode** (auto-respawn until queue empty).

## Usage

```
/c4-run             # Continuous: spawn workers, auto-respawn until queue empty (default)
/c4-run 3           # Continuous, initial batch 3 workers
/c4-run --max 4     # Continuous, cap 4 workers per respawn
/c4-run --single    # One round only (no respawn); then exit
```

## Single-Task Worker Model

Each Worker processes ONE task and exits (context isolation).
All workers run in background — main session remains interactive.

**Why?** Prevents context accumulation, fresh context per task, failures isolated.

## Instructions

### Pre-Flight Checks

1. **MCP Tools Only** — `c4_status`, `c4_start`, `c4_get_task`, `c4_submit`, `c4_run_validation`
2. **Accept Edits Mode** — Verify enabled (`Shift+Tab`). If off, automation breaks!

### 0. Generate Worker ID

```python
import uuid
WORKER_ID = f"worker-{uuid.uuid4().hex[:8]}"  # NEVER hardcode!
```

### 1. Status Check + Parallelism Analysis

```python
status = mcp__c4__c4_status()
parallelism = status["parallelism"]
# Keys: recommended, ready_now, max_parallelism, by_model, pending_total, blocked_count, reason
```

**State routing**: PLAN/HALTED→Step 2, EXECUTE→Step 3, CHECKPOINT→config분기, COMPLETE→exit, INIT→"/c4-plan first"

### 2. Transition to EXECUTE

```python
mcp__c4__c4_start()  # PLAN/HALTED → EXECUTE
```

### 3. Worker Count Decision

```python
args = "$ARGUMENTS".strip()
single_round = "--single" in args
continuous_mode = not single_round
worker_count = min(parallelism["ready_now"], 7)  # cap at 7
```

### 4. Spawn Workers

Worker prompt template: see `references/worker-prompt.md`

```python
WORKER_PROMPT = Read("references/worker-prompt.md")  # {worker_id} placeholder

# Model routing
review_model = status.get("economic_mode", {}).get("model_routing", {}).get("review", "opus")
impl_model = status.get("economic_mode", {}).get("model_routing", {}).get("implementation", "sonnet")

for i in range(worker_count):
    worker_id = f"worker-{uuid.uuid4().hex[:8]}"
    # R- tasks → review_model (opus), others → impl_model (sonnet)
    model = review_model if review_spawned < len(review_ids) else impl_model
    Task(subagent_type="general-purpose", prompt=WORKER_PROMPT.format(worker_id=worker_id),
         model=model, run_in_background=True)
```

### 5. Continuous Mode (default)

Auto-respawns until queue empty. 30s polling interval.

```python
while continuous_mode:
    time.sleep(30)
    status = mcp__c4__c4_status()

    if status["status"] == "COMPLETE":
        c4_notify(message='모든 태스크 완료', event='worker.complete')
        break
    if status["status"] == "CHECKPOINT":
        # auto mode: spawn checkpoint reviewer
        # interactive mode: pause + notify user
        break

    ready = status["parallelism"]["ready_now"]
    if ready == 0 and status["queue"]["pending"] == 0:
        break
    # Spawn new workers for ready tasks (review first, then impl)

# Auto-finish
Skill("c4-finish")
```

## Worktree Isolation (Multi-Worker)

`c4_get_task()` returns `worktree_path` — all file ops MUST occur within it.

```python
task = c4_get_task(WORKER_ID)
work_dir = Path(task.worktree_path)  # e.g. ".c4/worktrees/worker-abc123"
```

## Agent Routing

`c4_get_task()` returns `recommended_agent` and `agent_chain`. Worker auto-selects.

## Configuration (.c4/config.yaml)

```yaml
run:
  checkpoint_mode: interactive  # "auto" | "interactive"
```

## Constraints

| Constraint | Value |
|------------|-------|
| Max Workers | 7 (Claude Code subagent limit) |
| Worktree | Required for multi-worker |
| Accept Edits | Required for automation |

## Related Skills

- `/c4-status` — Check status
- `/c4-stop` — Stop execution
- `/c4-submit` — Manual submission
- `/c4-checkpoint` — Checkpoint review

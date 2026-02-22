# /c4-run

Trigger: `/c4-run` or keywords: `실행`, `run`, `ㄱㄱ`

## What it does

Spawns one worker per ready task. Workers run in parallel, each in an isolated git worktree.

```
/c4-run
→ finds pending tasks (dependencies resolved)
→ spawns Worker 1 for T-001 (worktree: c4/w-T-001-0)
→ spawns Worker 2 for T-002 (worktree: c4/w-T-002-0)
→ each worker: reads task → implements → runs tests → submits
→ review tasks (R-001, R-002) created automatically
→ review workers spawned for each
→ respawn continues until queue is empty
→ /c4-polish: build-test-review-fix loop until zero changes
→ /c4-finish: build · test · docs · commit
```

## Worker lifecycle

Each worker:
1. Calls `c4_get_task` → receives task with DoD and knowledge context
2. Implements in the assigned worktree
3. Runs validations (lint + tests)
4. Calls `c4_submit` with commit SHA and handoff summary
5. Auto-generates a review task

## Monitoring

```
/c4-status    → visual progress with dependency graph
```

## If a review requests changes

A revision task (T-001-1) is created automatically. `/c4-run` will pick it up on the next cycle.

## Continuous mode

`/c4-run` keeps respawning until all tasks are done — you don't need to re-run it manually.

## After execution

When all tasks are done, `/c4-run` automatically calls `/c4-polish` (build-test-review-fix loop until zero changes) and then `/c4-finish` (build · test · docs · commit). No extra steps needed.

If you make additional manual changes, run `/c4-finish` to wrap up.

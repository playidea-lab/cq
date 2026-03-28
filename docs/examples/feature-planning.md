# Feature Planning: /pi → /c4-plan → /c4-run

A complete example of building a large feature — real-time webhook delivery with retry logic — using CQ's full planning workflow.

---

## When to Use This Workflow

Use `/pi` → `/c4-plan` → `/c4-run` when:

- The feature touches 10+ files
- Architecture decisions need to be made (DB schema, retry strategy, queue design)
- Multiple engineers (or workers) will work in parallel
- You need a spec document before writing any code

For smaller tasks, use `/c4-quick` instead.

---

## The Feature: Webhook Delivery System

**Goal**: When an event is published (e.g., `job.completed`), the system shall deliver it to all registered webhook URLs with exponential backoff retry (max 5 attempts, 2^n seconds between retries).

---

## Step 1: Idea Exploration with /pi

Start with `/pi` to brainstorm and surface design options before committing to a plan:

```
/pi "webhook delivery system with retry"
```

CQ enters Play Idea mode — a low-commitment exploration phase. It asks clarifying questions and surfaces options:

```
[PI] Exploring: webhook delivery system with retry

Questions to clarify scope:
  1. Delivery ordering: strict (FIFO) or best-effort?
  2. Retry storage: in-memory queue or persistent (SQLite/Postgres)?
  3. Failure notification: should callers be notified on final failure?
  4. Concurrency: max parallel deliveries per endpoint?

Design options:
  Option A: In-process goroutine pool + SQLite retry table
    Pros: no infra dependencies, simple ops
    Cons: lost on crash if queue not flushed

  Option B: Separate webhook-worker process + Redis queue
    Pros: durable, scalable
    Cons: requires Redis, more moving parts

  Option C: Embedded retry loop in event dispatcher
    Pros: minimal code, no new tables
    Cons: blocks dispatcher goroutine

Recommendation: Option A for v1 (fits current SQLite architecture)
```

Answer the questions:

```
1. Best-effort is fine for v1
2. Persistent — SQLite, consistent with rest of system
3. Yes, emit a webhook.failed event
4. 5 concurrent per endpoint
```

CQ summarizes the agreed design and offers to move to planning:

```
[PI] Ready to plan. Run /c4-plan to generate tasks.
```

---

## Step 2: Generate Tasks with /c4-plan

```
/c4-plan "webhook delivery: SQLite-backed retry, 5 attempts, exponential backoff, webhook.failed event on exhaustion"
```

CQ runs Discovery → Design → Task generation:

```
[DISCOVERY] Domain: go backend
Requirements collected (EARS format):
  - When job.completed fires, system shall deliver to all registered hooks
  - If delivery fails, system shall retry with 2^n second delay (max 5 attempts)
  - When all retries exhausted, system shall emit webhook.failed event
  - System shall support max 5 concurrent deliveries per endpoint

[DESIGN] Architecture decision recorded (ADR-014):
  Storage: webhooks table (id, url, event_type, created_at)
           webhook_deliveries table (id, webhook_id, payload, attempt, status, next_retry)
  Worker: WebhookWorker goroutine pool (pool size = 5 per endpoint)
  Retry: exponential backoff 1s, 2s, 4s, 8s, 16s

[TASKS] Generated 6 tasks:
  T-WH-01  DB schema: webhooks + webhook_deliveries tables
  T-WH-02  WebhookStore: CRUD + pending-deliveries query
  T-WH-03  WebhookWorker: goroutine pool + HTTP delivery
  T-WH-04  RetryScheduler: backoff + requeue logic
  T-WH-05  EventBus integration: subscribe job.completed → enqueue
  T-WH-06  Tests: unit + integration
  CP-WH    Checkpoint: all tasks + integration tests passing

Dependencies:
  T-WH-02 requires T-WH-01
  T-WH-03 requires T-WH-02
  T-WH-04 requires T-WH-03
  T-WH-05 requires T-WH-03
  T-WH-06 requires T-WH-03, T-WH-04, T-WH-05
  CP-WH   requires T-WH-06
```

Review the task list:

```
/c4-status
```

```
State: PLAN → EXECUTE
Queue: 6 pending | 0 in_progress | 0 done
Ready now: T-WH-01 (no dependencies)
Blocked: T-WH-02..06 (waiting on dependencies)
```

---

## Step 3: Run Workers with /c4-run

Start the workers. T-WH-01 has no dependencies, so one worker can start immediately:

```
/c4-run 3
```

```
Spawning 3 workers...

Worker-1 claimed: T-WH-01  DB schema: webhooks + webhook_deliveries tables
Worker-2: no ready tasks, waiting...
Worker-3: no ready tasks, waiting...
```

Worker-1 produces the migration:

```sql
-- infra/supabase/migrations/00060_webhooks.sql
CREATE TABLE webhooks (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    url         TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    secret      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id  TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    payload     JSONB NOT NULL,
    attempt     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | delivered | failed
    next_retry  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_deliveries_pending
    ON webhook_deliveries(status, next_retry)
    WHERE status = 'pending';
```

Worker-1 submits T-WH-01. T-WH-02 becomes ready.

```
Worker-1 submitted T-WH-01
Worker-2 claimed: T-WH-02  WebhookStore: CRUD + pending-deliveries query
```

Workers continue picking up tasks as dependencies are resolved. Watch progress:

```
/c4-status
```

```
State: EXECUTE
Queue: 2 pending | 3 in_progress | 1 done
  T-WH-01  [done]         DB schema
  T-WH-02  [in_progress]  WebhookStore
  T-WH-03  [in_progress]  WebhookWorker
  T-WH-04  [pending]      RetryScheduler    (blocked by T-WH-03)
  T-WH-05  [pending]      EventBus integration (blocked by T-WH-03)
  T-WH-06  [in_progress]  Tests
  CP-WH    [pending]      Checkpoint
```

---

## Step 4: Checkpoint Review

When all tasks complete, CQ automatically enters CHECKPOINT state:

```
/c4-status
```

```
State: CHECKPOINT
Checkpoint: CP-WH — all 6 tasks done, awaiting review
```

Trigger the review:

```
/c4-checkpoint
```

```
[CHECKPOINT] Reviewing CP-WH...

Changes (6 files, +842 lines, -12 lines):
  infra/supabase/migrations/00060_webhooks.sql  (+38)
  internal/store/webhook_store.go               (+180)
  internal/worker/webhook_worker.go             (+210)
  internal/scheduler/retry_scheduler.go         (+95)
  internal/eventbus/dispatcher.go               (+45, -12)
  internal/worker/webhook_worker_test.go        (+274)

Validations:
  go-build:       PASS
  go-vet:         PASS
  go-test:        PASS  (87 tests, 1.4s)
  integration:    PASS  (3 tests, 2.1s)

Review result: APPROVE

Notes:
  - WebhookWorker correctly uses semaphore for per-endpoint concurrency (good)
  - RetryScheduler uses time.AfterFunc — consider ticker for large queues in future
  - webhook.failed event correctly emitted on attempt == maxAttempts
```

---

## Step 5: Finish

```
/c4-finish
```

```
[FINISH] Polishing...
  - Generating changelog entry
  - Running final validation
  - All checks passed

State: COMPLETE

Summary: Webhook delivery system
  - 6 tasks completed
  - 842 lines added
  - 87 unit + 3 integration tests passing
  - Ready for release
```

---

## What Each Step Does

| Step | Command | Purpose |
|------|---------|---------|
| Explore | `/pi "idea"` | Brainstorm options, clarify constraints before committing |
| Plan | `/c4-plan "spec"` | Generate spec, design decisions (ADR), task queue |
| Execute | `/c4-run N` | Spawn N workers; they pick up tasks as deps resolve |
| Review | `/c4-checkpoint` | Supervisor reviews all changes; approve or request changes |
| Finish | `/c4-finish` | Polish, changelog, final validation |

---

## Tips

**Sizing workers**: `/c4-run 3` is a good default. More workers help when many tasks are independent. For a linear dependency chain (A → B → C), extra workers idle.

**If checkpoint requests changes**: CQ creates new tasks and returns to EXECUTE. Workers automatically pick them up — just wait, or run `/c4-run` again.

**Spec is preserved**: The spec and design from `/c4-plan` are stored in `.c4/specs/` and `.c4/designs/`. Refer to them anytime with `c4_get_spec` or `c4_get_design`.

---

## Next Steps

- **GPU workloads**: [Distributed Experiments](distributed-experiments.md)
- **Researcher workflow**: [Researcher E2E](researcher-workflow.md)
- **Full workflow reference**: [Usage Guide](../usage-guide.md)

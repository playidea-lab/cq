# Example: Feature Planning

Building a new feature end-to-end with parallel workers.

## Setup

```sh
cd your-project
cq claude          # initialize CQ
# restart Claude Code
```

## Step 1 — Describe what you want

> **You:** "JWT 인증 추가해줘. Google이랑 GitHub 로그인"

CQ's `/c4-plan` triggers automatically and runs a structured discovery process:

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery
    Q: Session store — Redis or DB?          → DB (stateless preferred)
    Q: Token expiry — access/refresh split?  → Yes, 15min / 7day
    Q: Existing user model?                  → users table exists

  ● Design
    Provider abstraction → Google → GitHub → Session middleware
    JWT: RS256, stored in httpOnly cookie

  ● Tasks created
    T-001  OAuth provider interface
    T-002  Google provider
    T-003  GitHub provider
    T-004  JWT middleware + session store
    T-005  Integration tests
```

Each task has a **Definition of Done** — a checklist that must pass before the task is considered complete.

## Step 2 — Run the workers

> **You:** "ㄱㄱ"

```
/c4-run

  ◆ T-001  [worker-a] worktree: c4/w-T-001-0  ████████░░  implementing...
  ◆ T-002  [worker-b] worktree: c4/w-T-002-0  ████░░░░░░  implementing...
  ◆ T-003  [worker-c] worktree: c4/w-T-003-0  ██░░░░░░░░  implementing...
  ◆ T-004  waiting on T-001

  ✓ T-001  submitted (sha: a3f8c21)  →  R-001 review queued
  ✓ T-002  submitted (sha: 7b2e94d)  →  R-002 review queued
  ...
  ✓ All tasks complete. Run /c4-finish to wrap up.
```

Workers run in **parallel** — each in its own isolated git worktree so they never conflict. Review tasks (`R-001`, `R-002`) are created automatically after each implementation.

## Step 3 — Check progress

> **You:** "지금 어디까지 됐어?"

```
/c4-status

  Phase: EXECUTE  ████████████░░░░  75%

  ✓ T-001  OAuth interface      [merged]
  ✓ T-002  Google provider      [merged]
  ▶ T-003  GitHub provider      [in review]
  ◷ T-004  JWT middleware        [pending T-003]
  ◷ T-005  Integration tests    [pending T-004]

  Workers: 1 active  |  Queue: 2 pending  |  Knowledge: 8 records
```

## Step 4 — Wrap up

> **You:** "마무리해줘"

```
/c4-finish

  ● Polish: 2 rounds → 0 changes (CONVERGED)
  ● Build:  go build ./... ✓
  ● Tests:  247 passed, 0 failed
  ● Docs:   CHANGELOG updated
  ● Commit: feat(auth): JWT OAuth with Google and GitHub (sha: f4a9c31)
```

## What happened behind the scenes

- Each worker got the task DoD + relevant past knowledge injected into its context
- Reviews caught 3 issues across T-002 and T-003, triggering revision tasks automatically
- All discoveries from this session were recorded to the knowledge base for future tasks

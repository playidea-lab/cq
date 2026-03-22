# Example: Idea to Ship

From a vague idea to committed code — without touching a single command between steps.

::: info solo tier
This example works with the `solo` tier — no cloud setup required.
:::

## Scenario

You have a rough idea: "add a health check endpoint." You don't want to manually run `/c4-plan`, then `/c4-run`, then `/c4-finish`. You want to describe the idea and let CQ handle the rest.

## Step 1 — Describe your idea

```sh
cq claude
```

> **You:** "헬스체크 엔드포인트 추가하고 싶어. DB 연결이랑 디스크 용량도 체크하면 좋겠는데"

CQ's `/pi` activates. It asks a few clarifying questions, then converges:

```
💡 Ideation mode

Got it — health check endpoint. Let me explore:
- Should it return detailed status per component, or just OK/FAIL?
- Public endpoint or behind auth?
- Any specific response format? (JSON, plain text)

> JSON, public, detailed per component

📋 idea.md generated: .c4/ideas/health-check-endpoint.md
```

## Step 2 — Choose "Auto"

CQ asks how to proceed:

```
Next step:
  (1) Auto — plan → run → finish, fully automatic
  (2) Plan only — create tasks, review before running

> 1
```

That's it. You're done talking.

## What happens next (no human input)

```
🔗 Pipeline: plan → run → finish (auto)

━━ /c4-plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ● Discovery: EARS requirements parsed from idea.md
  ● Design: single handler + checker interface
  ● Tasks:
    T-001  Health check handler (/healthz)
    T-002  DB connectivity checker
    T-003  Disk space checker

━━ /c4-run ━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ◆ T-001  [worker-a]  implementing...  ✓ done
  ◆ T-002  [worker-b]  implementing...  ✓ done
  ◆ T-003  [worker-c]  implementing...  ✓ done
  ✓ R-001  review passed
  ✓ R-002  review passed
  ✓ R-003  review passed

━━ /c4-finish ━━━━━━━━━━━━━━━━━━━━━━━━━
  ● Polish: 1 round → 0 changes (CONVERGED)
  ● Build:  go build ./... ✓
  ● Tests:  312 passed, 0 failed
  ● Commit: feat(health): add /healthz with DB and disk checks
```

You come back to a committed feature in your repo. No intermediate prompts, no "shall I proceed?" questions.

## The pipeline in detail

```
You said "auto"
  ↓
/pi writes .c4/pipeline-state.json (auto=true)
  ↓
/c4-plan skips action selection (FROM_PI), skips editor (AUTO_RUN)
  ↓
/c4-run spawns workers in parallel worktrees
  ↓
/c4-finish runs polish loop, builds, tests, commits
  ↓
pipeline-state.json deleted — pipeline complete
```

The chain is driven by a simple state file. Each skill checks `auto=true` and proceeds without asking. If anything fails, the agent fixes it — the pipeline doesn't intervene.

## When to use this

- You have a clear idea but don't want to babysit the process
- Small-to-medium features (1–5 tasks)
- You trust CQ's planning and want to review the result, not the process

## When NOT to use this

- Large features where you want to review the plan before execution
- Exploratory work where the design isn't clear yet
- Changes to critical systems where you want to approve each step

For those cases, use `/c4-plan` manually and review before running.

## Next steps

- **Review manually first**: → [Feature Planning](/examples/feature-planning)
- **Fix something small**: → [Quick Bug Fix](/examples/quick-fix)

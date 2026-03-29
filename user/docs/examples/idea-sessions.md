# Idea Session Management

Use `/pi` to brainstorm ideas, save them as `idea.md` files, and pick them up days or weeks later. No idea gets lost.

---

## The Problem

You have three ideas brewing:
1. A CLI dashboard for monitoring workers
2. A Slack integration for task notifications
3. A caching layer for the knowledge search

Each needs research and discussion before committing to implementation. You can't do them all at once.

---

## Start an Idea Session

```
/pi "CLI dashboard for monitoring CQ workers in real-time"
```

CQ enters brainstorming mode:
- Searches existing knowledge for related work
- Runs web searches for prior art (blessed, k9s, lazydocker)
- Asks probing questions: "Who's the primary user? What metrics matter?"

After 5-10 minutes of discussion, the idea crystallizes:

```
✓ Saved: .c4/ideas/worker-dashboard.md
  - Problem: No visibility into worker status without checking logs
  - Target: Solo developers running 3+ workers
  - Core: Real-time task progress, worker health, queue depth
  - Risk: TUI library choice (bubbletea vs tview)
```

---

## Come Back Later

Next week, you want to pick up the Slack integration idea:

```
/pi "Slack notifications for CQ tasks"
```

CQ finds your existing ideas:

```
Found 1 existing idea:
  - worker-dashboard.md (7 days ago) — CLI dashboard for worker monitoring

This is a new idea. Starting fresh brainstorm...
```

After discussion:

```
✓ Saved: .c4/ideas/slack-notifications.md
```

---

## List All Ideas

```sh
ls .c4/ideas/
```

```
worker-dashboard.md      (2026-03-22)
slack-notifications.md   (2026-03-29)
```

Each file contains the full context: problem, target user, scope, risks, and research findings.

---

## Resume an Idea

Pick up where you left off:

```
/pi "let's continue the worker dashboard idea"
```

CQ loads `worker-dashboard.md` and resumes the conversation with full context — no re-explaining.

---

## Promote to Implementation

When an idea is ready:

```
Choose next step:
  [1] Auto-implement — plan → run → finish (fully automatic)
  [2] Plan only     — create tasks, you decide when to run
```

Option 1 triggers the full pipeline:

```
/pi → idea.md → /plan → /run → /finish
```

No manual handoff. The idea flows straight into code.

---

## Idea File Format

Each `idea.md` captures:

```markdown
# Worker Dashboard

## Problem
No real-time visibility into CQ worker status...

## Target User
Solo developers running 3+ parallel workers...

## Core Concept
TUI dashboard showing task progress, worker health, queue depth...

## Research
- bubbletea: Go TUI framework, used by charm.sh tools
- k9s: Kubernetes TUI — good UX reference
...

## Risks
- TUI library lock-in
- Terminal compatibility across OS
...

## Requirements (EARS)
- WHEN user runs `cq dashboard` THEN shows live worker grid
- WHEN a worker fails THEN row turns red within 2 seconds
...
```

---

## Next Steps

- [Feature Planning](feature-planning.md) — the full /pi → /plan → /run workflow
- [Growth Loop](growth-loop-in-action.md) — how CQ learns your preferences over time

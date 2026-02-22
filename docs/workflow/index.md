# Workflow Overview

CQ follows a structured loop:

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE → CHECKPOINT → POLISH → COMPLETE
```

## Commands

| Command | Phase | When to use |
|---------|-------|-------------|
| [`/c4-plan`](/workflow/plan) | Plan | New feature or significant change |
| [`/c4-run`](/workflow/run) | Execute | Spawn workers, implement, auto-polish, and finish |
| [`/c4-finish`](/workflow/finish) | Complete | Final build, tests, docs, and commit (also called automatically by `/c4-run`) |
| `/c4-status` | Any | Check progress at any time |
| `/c4-quick` | — | Single small task, skip the planning phase |

## Task lifecycle

Every task goes through:

```
pending → in_progress → (review) → completed
                      ↘ request_changes → revision → ...
```

- **T-001-0** — implementation task (version 0)
- **R-001-0** — auto-generated review task after each implementation
- **T-001-1** — revision task if a review requests changes
- **CP-001** — checkpoint, auto-generated when a phase completes

Checkpoints are created automatically — you don't trigger them manually. When a checkpoint appears, CQ pauses and asks you to review the phase output before continuing to the next phase.

## Worker isolation

Each worker runs in its own git worktree (`c4/w-T-XXX-N`), so parallel workers never conflict. Worktrees are cleaned up automatically after `c4-finish`.

## Knowledge loop

Every completed task records discoveries and rationale. Future tasks receive relevant past knowledge injected into their context automatically.

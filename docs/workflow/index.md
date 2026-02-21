# Workflow Overview

CQ follows a structured loop:

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → REFINE → POLISH → COMPLETE
```

## Commands

| Command | When to use |
|---------|-------------|
| [`/c4-plan`](/workflow/plan) | New feature or significant change |
| [`/c4-run`](/workflow/run) | Execute the task queue |
| [`/c4-refine`](/workflow/refine) | Quality gate — fix CRITICAL and HIGH issues |
| [`/c4-polish`](/workflow/polish) | Full convergence — repeat until zero modifications |
| [`/c4-finish`](/workflow/finish) | Wrap up after all tasks complete |
| `/c4-status` | Check progress at any time |
| `/c4-quick` | Single small task, skip the planning phase |

## Task lifecycle

Every task goes through:

```
pending → in_progress → (review) → completed
                      ↘ request_changes → revision → ...
```

- **T-001-0** — implementation task (version 0)
- **R-001-0** — auto-generated review task
- **T-001-1** — revision if review requests changes
- **CP-001** — checkpoint after a phase completes

## Worker isolation

Each worker runs in its own git worktree (`c4/w-T-XXX-N`), so parallel workers never conflict. Worktrees are cleaned up automatically after `c4-finish`.

## Knowledge loop

Every completed task records discoveries and rationale. Future tasks receive relevant past knowledge injected into their context automatically.

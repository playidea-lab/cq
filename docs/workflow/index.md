# Workflow Overview

CQ operates in **cloud-primary mode** when connected: tasks, knowledge, and LLM calls are routed through the CQ cloud (Supabase SSOT). Local execution happens at the edge — the cloud is the brain, your machine is the hands. For `solo` tier, everything runs locally.

CQ follows a structured loop with **gates at every transition**:

```
/pi → PLAN → EXECUTE → COMPLETE
       ↑        ↑         ↑
    refine    polish    review
     gate      gate      gate
```

## Commands

| Command | Phase | What happens |
|---------|-------|-------------|
| [`/pi`](/workflow/plan#pi) | Ideation | Brainstorm, research, debate. Produces `idea.md`, auto-transitions to `/c4-plan`. |
| [`/c4-plan`](/workflow/plan) | Plan | Discovery → Design → Task breakdown. **Refine gate**: batch ≥ 4 tasks requires critique loop. |
| [`/c4-run`](/workflow/run) | Execute | Spawn parallel workers. **Polish gate**: each worker self-reviews before submit. |
| [`/c4-finish`](/workflow/finish) | Complete | Build → test → install → commit. Polish loop built-in. |
| `/c4-status` | Any | Check progress, dependency graph, worker status. |
| `/c4-quick` | — | Single small task, skip planning. |

## Task lifecycle

```
pending → in_progress → polish → submit → review → done
                                            ↘ request_changes → T-XXX-1 (revision)
```

- **T-001-0** — implementation (version 0)
- **R-001-0** — auto-generated 6-axis review
- **T-001-1** — revision if review requests changes (max 3 revisions)
- **CP-001** — checkpoint at phase boundaries

## Quality gates

| Gate | Trigger | Enforcement |
|------|---------|-------------|
| **Refine** | `c4_add_todo` with 4+ tasks in 10 min | Go rejects unless `c4_record_gate("refine", "done")` |
| **Polish** | `c4_submit` with diff ≥ 5 lines | Go rejects unless `c4_record_gate("polish", "done")` |
| **Review** | Every T- task completion | Auto-creates R- task, 6-axis evaluation |
| **Max revision** | 3 rejection cycles | Go blocks further revisions |

These are **not suggestions** — they are Go-level checks compiled into the binary.

## Worker isolation

Each worker runs in its own git worktree (`c4/w-T-XXX-N`). Parallel workers never conflict. Worktrees merge automatically on submit.

## Knowledge & Persona loop

```
Task completed → discoveries recorded → knowledge base updated
                                              ↓
Next task ← knowledge auto-injected ← persona learns your style
```

The **3-layer ontology** builds up over time:
- **L1**: Your local patterns (naming, review preferences)
- **L2**: Project-level cross-position patterns
- **L3**: Collective patterns shared via Hub

After 100 tasks, CQ adapts to your style. After 500, it anticipates your feedback.

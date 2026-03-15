---
name: c4-swarm
description: |
  Agent Teams-based parallel collaboration for C4 tasks. Spawns coordinator-led
  teams with direct member communication. Supports standard (implementation),
  review (read-only audit), and investigate (hypothesis competition) modes.
  Auto-maps C4 tasks to team tasks, domain-based agent selection, handoff tracking,
  auto-judge review spawning. Triggers: "팀 협업", "스웜", "병렬 팀 실행",
  "swarm mode", "spawn team", "parallel workers with communication",
  "team collaboration", "coordinate agents".
---

# C4 Swarm — Agent Teams Parallel Collaboration

**Team-based parallel execution with direct inter-member communication.** Maps C4 tasks to Agent Teams, with you as the coordinator.

## Usage

```
/c4-swarm                  # Auto: C4 task-based team composition
/c4-swarm 3                # Spawn 3 members
/c4-swarm --review         # Review-only team (read-only)
/c4-swarm --investigate    # Hypothesis competition mode
```

## vs /c4-run

| Item | `/c4-run` | `/c4-swarm` |
|------|-----------|-------------|
| Communication | None (independent) | `SendMessage` (direct) |
| Lifetime | 1 task → exit | Until team disbands |
| Coordinator | None | Main session = team lead |

**Simple parallel → `/c4-run`, team collaboration → `/c4-swarm`.**

## Instructions

### 0. Parse Arguments

```python
review_mode = "--review" in args
investigate_mode = "--investigate" in args
member_count = min(int(args), 5) if args.isdigit() else None  # Auto in Step 1
```

### 1. Check C4 Status + Team Size

```python
status = mcp__c4__c4_status()
```

- **INIT/CHECKPOINT/COMPLETE**: exit with guidance
- **PLAN/HALTED**: `c4_start()` → EXECUTE
- **EXECUTE**: proceed

Auto-size: `min(status["parallelism"]["recommended"], 5)`.

### 2. Create Team + Map Tasks

```python
team_name = f"c4-{int(time.time())}"
TeamCreate(team_name=team_name, description=f"C4 Swarm — {member_count} members")
```

Map C4 pending tasks to Agent Teams TaskCreate (standard mode), or create review/investigate tasks per mode.

### 3. Spawn Members

Assign role and domain to each member. See `references/member-prompts.md` for prompt templates (standard/review/investigate).

**Standard mode**: auto-select agent type by task domain (DOMAIN_AGENT_MAP).
**Review mode**: 3 reviewers (Security / Performance / Test Coverage), `mode="plan"` (read-only).
**Investigate mode**: 1 investigator per hypothesis.

Spawn all members **simultaneously (in parallel)**.

### 4. Coordinator Role (You = Team Lead)

React to auto-received member messages:
- Task completion → check handoff + guide next task
- Block report → provide solution or delegate
- Questions → answer
- Review results → synthesize

#### Auto-Judge: Automatic Review Spawning (CRITICAL)

When `c4_submit` returns `pending_review`, immediately spawn a `code-reviewer` agent for that review task. See `references/member-prompts.md` for reviewer prompt.

#### Recursive Sub-planners

If a task has 3+ files or 5+ DoD checkboxes, spawn a sub-planner instead of worker. See `references/member-prompts.md`.

### 5. Disband Team

When all tasks complete: `SendMessage(type="shutdown_request")` to each member → `TeamDelete()`.

## Constraints

| Constraint | Value |
|------------|-------|
| Max Members | 5 |
| Review Mode | plan mode (read-only) |
| Standard Mode | bypassPermissions |

## Related Skills

- `/c4-run` — Independent Worker parallel (no communication)
- `/c4-checkpoint` — Checkpoint review

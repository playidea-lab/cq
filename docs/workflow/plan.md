# /c4-plan

Trigger: `/c4-plan "feature description"` or keywords: `계획`, `plan`, `설계`

## What it does

Runs a structured planning process:

1. **Discovery** — Clarifies requirements using EARS format. Asks questions until the scope is unambiguous.
2. **Design** — Proposes architecture and key decisions with tradeoffs.
3. **Lighthouse** — Registers tool contracts (DoD checklist per task). Lighthouse is C4's contract-first TDD layer: each task gets a verifiable definition of done before any code is written.
4. **Tasks** — Drafts the task queue with Definition of Done per task.
5. **Plan Critique Loop** — Spawns a fresh reviewer each round to stress-test specs, DoDs, and task design. Stops when no CRITICAL or HIGH issues remain. This is why no separate `/c4-refine` step is needed.
6. **Commit** — Saves the validated tasks to `.c4/tasks.db`.

## Example

```
/c4-plan "add OAuth2 login with Google and GitHub"
```

CQ will ask clarifying questions, then create tasks like:
- T-001: implement OAuth2 provider abstraction
- T-002: add Google provider
- T-003: add GitHub provider
- T-004: integrate with session store

## Task Definition of Done

Each task gets a DoD checklist:

```
- [ ] Unit tests pass
- [ ] Integration test with mock provider
- [ ] Error handling for token expiry
- [ ] Rationale documented in handoff
```

## After planning

Run `/c4-run` to start workers. `/c4-run` handles the rest — implementation, polish, and finish.

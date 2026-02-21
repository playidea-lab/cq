# /c4-plan

Trigger: `/c4-plan "feature description"` or keywords: `계획`, `plan`, `설계`

## What it does

Runs a 4-phase structured planning process:

1. **Discovery** — Clarifies requirements using EARS format. Asks questions until the scope is unambiguous.
2. **Design** — Proposes architecture and key decisions with tradeoffs.
3. **Lighthouse** — Registers tool contracts (DoD checklist per task).
4. **Tasks** — Creates the task queue in `.c4/tasks.db` with Definition of Done per task.

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

Run `/c4-run` to start workers.

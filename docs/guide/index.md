# What is CQ?

**CQ** is a project management engine that runs alongside Claude Code.

It adds structure to AI-assisted development:

- **Tasks** have a Definition of Done and are tracked in a local SQLite database
- **Workers** are Claude Code instances that each handle one task in an isolated git worktree
- **Reviews** are automatic — every implementation task gets a review task
- **Checkpoints** act as phase gates before moving forward
- **Knowledge** is recorded from every completed task and used to improve future ones

## How it works

```
You describe a feature → /c4-plan creates tasks with DoD
                       → /c4-run spawns workers (one per task)
                       → each worker: implement → test → submit
                       → reviewer worker checks the output
                       → build, test, commit
```

CQ uses the **Model Context Protocol (MCP)** — Claude Code talks to the CQ binary via 100+ tools (`c4_*` prefix).

## What CQ is not

- Not an AI model — it orchestrates Claude Code
- Not a code generator — workers write code, CQ manages the process
- Not opinionated about your stack — works with any language or framework

## Next

- [Install CQ →](/guide/install)

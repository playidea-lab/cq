# What is CQ?

**CQ** is a project management engine that works with Claude Code, Cursor, Codex CLI, and Gemini CLI.

It adds structure to AI-assisted development:

- **Tasks** have a Definition of Done and are tracked in a local SQLite database
- **Workers** are AI agent instances (Claude Code, Cursor, Codex CLI, or Gemini CLI) that each handle one task in an isolated git worktree
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

CQ uses the **Model Context Protocol (MCP)** — Claude Code talks to the CQ binary via 169+ tools (`c4_*` prefix). Only ~40 core tools are shown by default; set `CQ_TOOL_TIER=full` to expose all of them.

CQ auto-routes requests by size: **Small** (direct edit), **Medium** (`/c4-quick`), **Large** (`/pi` → plan → run → finish).

## What CQ is not

- Not an AI model — it orchestrates AI coding tools
- Not a code generator — workers write code, CQ manages the process
- Not opinionated about your stack — works with any language or framework

## Next

- [Install CQ →](/guide/install)

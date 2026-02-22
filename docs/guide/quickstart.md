# Quick Start

This guide takes you from zero to your first CQ-managed task in under 5 minutes.

## Step 1: Initialize your project

Open a terminal in your project directory and run:

```sh
cd your-project
cq claude
```

This creates:
- `.mcp.json` — tells Claude Code where the CQ binary is
- `CLAUDE.md` — injects CQ rules into Claude Code's context
- `.c4/` — local database and config directory

Then **restart Claude Code** so it picks up the new MCP server.

## Step 2: Verify the connection

In Claude Code, run:

```
/c4-status
```

You should see the project state and an empty task queue.

## Step 3: Plan a feature

Describe what you want to build:

```
/c4-plan "add user authentication with JWT"
```

CQ will:
1. Ask clarifying questions (Discovery phase)
2. Design the approach (Design phase)
3. Break it into tasks with Definition of Done
4. Create the task queue

## Step 4: Run

```
/c4-run
```

Workers start automatically — one per task, each in an isolated git worktree. When the queue empties, `/c4-run` automatically runs polish (fix until zero changes) then finish (build · tests · docs · commit).

You can watch progress with:

```
/c4-status
```

That's it. `/c4-run` handles implementation, review, polish, and finish end-to-end.

---

If you need to make manual changes afterward, wrap up with:

```
/c4-finish
```

---

## Minimal example (single task)

For small tasks, skip the full plan flow:

```
/c4-quick "fix the login button not responding on mobile"
```

This creates one task, assigns it to a worker, and runs immediately.

## Next

- [Understand tiers →](/guide/tiers)
- [Learn the full workflow →](/workflow/)

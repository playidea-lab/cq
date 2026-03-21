# Quick Start

This guide takes you from zero to your first CQ-managed task in under 5 minutes.

## Step 1: Initialize your project

Open a terminal in your project directory and run the command for your AI tool:

```sh
cd your-project
cq claude   # Claude Code
cq cursor   # Cursor
cq codex    # OpenAI Codex CLI
cq gemini   # Gemini CLI
```

Each command creates `.CLAUDE.md`, `.c4/`, and the MCP config for your tool:

| Command | MCP config | Agent instructions |
|---------|-----------|-------------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI config) | `CLAUDE.md` |

Then **restart your AI tool** so it picks up the new MCP server.

::: tip Other AI tools
Any tool that supports the [AGENTS.md standard](https://agents.md) can read `CLAUDE.md` directly — no `cq init` required.
:::

## Step 1.5: Log in (connected / full tier)

If you're using the `connected` or `full` tier, authenticate once:

```sh
cq auth login
```

This opens GitHub OAuth in your browser and automatically patches `.c4/config.yaml` with `cloud.enabled`, `url`, and `anon_key`. After login, startup prints:

```
✓ Cloud: user@example.com (expires in 47h)
```

Skip this step for the `solo` tier — no login required.

## Step 1.6: Set up Telegram bot (optional)

To connect a Telegram bot for remote access:

```sh
cq --bot
```

Select "새 봇 만들기" from the menu and follow the wizard (BotFather token + your Telegram ID). After setup, use `cq --bot` or `cq --bot <botname>` to launch with Telegram.

## Step 1.7: Explore ideas first (optional)

Before planning, use `/pi` to brainstorm and refine your idea:

```
/pi
```

`/pi` enters ideation mode — diverge, converge, research, debate. When ready, it automatically launches `/c4-plan`.

## Step 1.8: Auto-configure with `cq doctor --fix`

After initializing, run the doctor to verify and auto-fix common setup issues:

```sh
cq doctor --fix
```

This checks and automatically fixes:
- `CLAUDE.md` / `AGENTS.md` presence and content
- Hook installation (`.claude/hooks/c4-gate.sh`)
- Python sidecar (`c4-bridge`) installation
- MCP server configuration

If everything is healthy, you'll see all green checks:

```
✓ CLAUDE.md         present
✓ hooks             c4-gate.sh installed
✓ sidecar           c4-bridge ready
✓ mcp               cq registered
```

::: tip
Run `cq doctor` (without `--fix`) to inspect without making changes.
:::

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
3. Generate **behavior spec** with WHEN-THEN-VERIFY scenarios → opens in editor for review
4. Break it into tasks with Definition of Done
5. Create the task queue

::: tip Behavior Spec (v1.3.1+)
For features with 4+ tasks, CQ auto-generates a behavior spec in `.c4/specs/`.
Each scenario defines **what the feature does** in human-readable format,
with machine-checkable VERIFY conditions mapped to tests.
Review it before implementation starts — it becomes the completion criteria.
:::

## Step 4: Run

```
/c4-run
```

Workers start automatically — one per task, each in an isolated git worktree. When the queue empties, `/c4-run` automatically runs polish (fix until zero changes) then finish (build · tests · docs · commit).

During finish, CQ validates your behavior spec against actual test results:

```
✅ S1: Happy path     → TestAuth_S1_LoginSuccess     PASS
✅ S2: Invalid token   → TestAuth_S2_InvalidToken     PASS
⚠️ S3: Token expiry    → (not implemented)            NO TEST
```

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

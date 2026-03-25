# Quick Start

This guide takes you from zero to your first CQ-managed task in under 5 minutes.

## Step 1: Initialize your project

Open a terminal in your project directory and run:

```sh
cd your-project
cq
```

This single command handles everything:
1. **Login** — opens GitHub OAuth if not logged in (no API key needed)
2. **Service install** — registers `cq serve` as an OS service (LaunchAgent / systemd with `Restart=always`)
3. **Project setup** — creates `CLAUDE.md`, `.c4/`, and MCP config
4. **Launch** — starts your AI tool (auto-detects Claude Code, Cursor, etc.)

```
CQ ready.
```

That's it. CQ runs in the background permanently — relay, journal sync, and token refresh all active. Workers survive reboots via systemd `Restart=always` (Linux) or LaunchAgent (macOS).

You can also specify the AI tool explicitly:

```sh
cq claude   # Claude Code
cq cursor   # Cursor
cq codex    # OpenAI Codex CLI
cq gemini   # Gemini CLI
```

::: tip Other AI tools
Any tool that supports the [AGENTS.md standard](https://agents.md) can read `CLAUDE.md` directly — no `cq init` required.
:::

::: tip Token-free MCP config
Worker entries in `.mcp.json` use `http://localhost:4140/w/{worker}/mcp` with no Bearer token. `cq serve` acts as a local proxy and auto-injects JWT on every request.
:::

## Step 1.5: Set up Telegram bot (optional)

To connect a Telegram bot for remote access:

```sh
cq --bot
```

Select "새 봇 만들기" from the menu and follow the wizard (BotFather token + your Telegram ID). After setup, use `cq --bot` or `cq --bot <botname>` to launch with Telegram.

## Step 1.6: Explore ideas first (optional)

Before planning, use `/pi` to brainstorm and refine your idea:

```
/pi
```

`/pi` enters ideation mode — diverge, converge, research, debate. When ready, it automatically launches `/c4-plan`.

## Step 1.7: Auto-configure with `cq doctor --fix`

After initializing, run the doctor to verify and auto-fix common setup issues:

```sh
cq doctor --fix
```

This checks and automatically fixes:
- `CLAUDE.md` / `AGENTS.md` presence and content
- Hook installation (`.claude/hooks/c4-gate.sh`)
- Python sidecar (`c4-bridge`) installation
- MCP server configuration
- **Relay** connectivity
- **OS service** status (`cq serve`)

If everything is healthy, you'll see all green checks:

```
✓ CLAUDE.md         present
✓ hooks             c4-gate.sh installed
✓ sidecar           c4-bridge ready
✓ relay             connected (2 workers)
✓ os-service        running (Restart=always)
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

Workers start automatically — one per task, each in an isolated git worktree. Each worker runs a built-in **polish loop** (review → fix → converge) before submitting. The Go-level polish gate rejects unreviewed code (diff ≥ 5 lines), so quality is enforced by the system, not by trust.

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

## Auto-routing

CQ auto-routes requests by size (configured in CLAUDE.md):

| Size | Route | When |
|------|-------|------|
| **Small** | Direct edit | Typo, 1-line fix, config tweak |
| **Medium** | `/c4-quick` | Single-task bug fix or small feature |
| **Large** | `/pi` → plan → run → finish | Multi-file feature, architecture change |

You don't need to pick — just describe what you need and CQ chooses the right path.

## What happens behind the scenes

Every step is gated:

| Gate | When | What it checks |
|------|------|----------------|
| **Refine** | `/c4-plan` adds 4+ tasks | Critique loop must run first |
| **Polish** | Worker submits code (diff ≥ 5 lines) | Self-review must converge |
| **Review** | After every implementation | 6-axis evaluation (correctness, security, reliability, observability, tests, readability) |

These are Go-level checks that cannot be skipped. The more you use CQ, the sharper reviews get — the [persona ontology](/guide/ecosystem) learns your preferences across tasks.

## Next

- [What's included →](/guide/tiers)
- [Learn the full workflow →](/workflow/)

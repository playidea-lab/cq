# Quick Start

Build something with CQ in 5 minutes.

## Before You Begin

Complete [installation](install.md) first, then run:

```sh
cq doctor    # Verify everything is working
```

## The Core Workflow

```
cq doctor → cq auth login → cq claude
                                 │
                                 ▼
                           /c4-plan "goal"
                                 │
                                 ▼
                            /c4-run
                           (repeats)
                                 │
                                 ▼
                           /c4-finish
```

## Step 1: Check Health

```sh
cq doctor
```

```
[✓] cq binary: v1.37
[✓] Claude Code: installed
[✓] MCP server: connected
[✓] .c4/ directory: initialized
```

Fix any failures before continuing.

## Step 2: Log In

```sh
cq auth login    # GitHub OAuth — opens browser
```

One-time setup. CQ uses the token for cloud sync, the Growth Loop, and the Knowledge base.

## Step 3: Open Claude Code

```sh
cq claude    # Launches Claude Code with CQ MCP connected
```

CQ auto-detects your AI tool. You can also use:

```sh
cq cursor     # Cursor
cq codex      # OpenAI Codex CLI
cq gemini     # Google Gemini
```

## Step 4: Plan

In Claude Code, describe what you want to build:

```
/c4-plan "add user authentication with JWT"
```

CQ walks through three phases:

1. **Discovery** — collects requirements (EARS format)
2. **Design** — proposes architecture and ADRs
3. **Plan** — breaks work into verifiable tasks with Definition of Done

Each phase requires your approval before proceeding.

## Step 5: Run

```
/c4-run
```

Workers pick up tasks automatically:

- Each worker gets one task, one fresh context, one isolated worktree
- Runs lint and tests before submitting
- Auto-respawns until the queue is empty
- Quality gates reject submissions that skip review

Set it up and come back — the queue drains on its own.

## Step 6: Finish

```
/c4-finish
```

Polishes the code, runs the full review cycle, and creates a clean commit. Checks off the Definition of Done checklist.

## Check Status at Any Time

```
/c4-status
```

```
## CQ Project Status

State:    EXECUTE
Queue:    3 pending | 2 in-progress | 7 done
Workers:  2 active
```

## Scenario: New Feature

```
/c4-plan "add CSV export to the reports page"
/c4-run
/c4-finish
```

## Scenario: Bug Fix (Direct Mode)

For single-file fixes that don't need the full planning pipeline:

```
Fix the null pointer in auth/token.go when refresh token is missing
```

Claude handles it directly. Or use the explicit flow:

```
c4_claim → make changes → c4_report
```

## Command Reference

| Command | What it does | When to use |
|---------|-------------|-------------|
| `cq doctor` | Health check | Before starting |
| `cq auth login` | GitHub OAuth | First time |
| `cq claude` | Launch Claude Code | Every session |
| `/c4-status` | Show project state | Any time |
| `/c4-plan "goal"` | Create plan + tasks | New feature |
| `/c4-run` | Start workers | After planning |
| `/c4-finish` | Polish + commit | After implementation |
| `/pi "idea"` | Brainstorm + research | Before planning |

## Auto-Routing

CQ routes requests automatically based on scope:

| Size | Criteria | Workflow |
|------|----------|----------|
| Small | Typo, 1–2 line change | Direct edit |
| Medium | 1–3 files, function change | `/c4-quick` → 1 worker |
| Large | New feature, design needed | `/pi` → `/c4-plan` → `/c4-run` → `/c4-finish` |

When in doubt, CQ picks the smaller option — faster is better than over-engineered.

## What Happens to My Sessions?

When you close a session, CQ automatically:

1. Summarizes decisions and discoveries
2. Extracts preferences from how you worked
3. Stores them in your Knowledge base
4. Feeds them into the [Growth Loop](growth-loop.md)

Next session, your preferences are already there.

## Next Steps

- [Tiers](tiers.md) — understand solo / connected / full
- [Growth Loop](growth-loop.md) — how CQ learns your preferences
- [Worker Setup](worker-setup.md) — add GPU workers for training jobs
- [Remote Brain](remote-brain.md) — access CQ from ChatGPT or Claude Desktop

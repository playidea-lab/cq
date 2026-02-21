# CQ — AI Project Orchestration Engine

**CQ** is a project management engine for Claude Code.
It automates the full development lifecycle — planning, implementation, review, and delivery — through a structured workflow powered by C4 Engine.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Opens a new terminal and you're ready:

```sh
cq --help
```

## Tiers

Choose the tier that fits your setup:

| Tier | Description | Use when |
|------|-------------|----------|
| `solo` | Local only, no external deps | Personal / offline |
| `connected` | + Supabase, LLM Gateway, EventBus | Team / cloud sync |
| `full` | + Hub, Drive, CDP, GPU, C1 Messenger | Full production |

```sh
# Install a specific tier (default: solo)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

## Quick Start

```sh
# 1. Check environment
cq doctor

# 2. Initialize C4 in your project (generates .mcp.json + CLAUDE.md)
cd your-project
cq claude   # for Claude Code
cq cursor   # for Cursor

# 3. Open Claude Code — C4 MCP tools are now available
```

## How it looks

### Scenario 1 — Building a new feature

> **You:** "JWT 인증 추가해줘. Google이랑 GitHub 로그인"

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery
    Q: Session store — Redis or DB?          → DB (stateless preferred)
    Q: Token expiry — access/refresh split?  → Yes, 15min / 7day
    Q: Existing user model?                  → users table exists

  ● Design
    Provider abstraction → Google → GitHub → Session middleware
    JWT: RS256, stored in httpOnly cookie

  ● Tasks created
    T-001  OAuth provider interface
    T-002  Google provider
    T-003  GitHub provider
    T-004  JWT middleware + session store
    T-005  Integration tests
```

> **You:** "ㄱㄱ"

```
/c4-run

  ◆ T-001  [worker-a] worktree: c4/w-T-001-0  ████████░░  implementing...
  ◆ T-002  [worker-b] worktree: c4/w-T-002-0  ████░░░░░░  implementing...
  ◆ T-003  [worker-c] worktree: c4/w-T-003-0  ██░░░░░░░░  implementing...
  ◆ T-004  waiting on T-001

  ✓ T-001  submitted (sha: a3f8c21)  →  R-001 review queued
  ✓ T-002  submitted (sha: 7b2e94d)  →  R-002 review queued
  ...
  ✓ All tasks complete. Run /c4-finish to wrap up.
```

---

### Scenario 2 — Quick bug fix

> **You:** "모바일에서 로그인 버튼 클릭이 안 돼"

```
/c4-quick "fix login button not responding on mobile"

  ● Task T-011-0 created
    DoD: touch event handler added, tested on viewport <768px

  ◆ [worker] implementing fix...
  ✓ submitted  →  review passed  →  done

  Changed: src/components/LoginButton.tsx (+3 -1)
```

---

### Scenario 3 — Checking status mid-flight

> **You:** "지금 어디까지 됐어?"

```
/c4-status

  Phase: EXECUTE  ████████████░░░░  75%

  ✓ T-001  OAuth interface      [merged]
  ✓ T-002  Google provider      [merged]
  ▶ T-003  GitHub provider      [in review]
  ◷ T-004  JWT middleware        [pending T-003]
  ◷ T-005  Integration tests    [pending T-004]

  Workers: 1 active  |  Queue: 2 pending  |  Knowledge: 8 records
```

---

## Workflow

```
/c4-plan "feature description"   → discovery + design + tasks
/c4-run                          → spawn workers, implement in parallel
/c4-finish                       → build · test · docs · commit
/c4-status                       → check progress at any time
```

## Config

`solo` tier works out of the box — no config needed.

For `connected` / `full` tiers, place the config provided by your team at `~/.c4/config.yaml`.

## Update

Re-run the install command to update to the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Requirements

- macOS Apple Silicon (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) installed
- `curl` available

## License

[MIT + Commons Clause](LICENSE) — free to use and modify, commercial resale prohibited.

# CQ — AI Project Orchestration Engine

[![Latest Release](https://img.shields.io/github/v/release/PlayIdea-Lab/cq)](https://github.com/PlayIdea-Lab/cq/releases/latest)
[![License](https://img.shields.io/badge/license-MIT%20%2B%20Commons%20Clause-blue)](LICENSE)

[한국어](README.ko.md) | **English**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

**CQ** is a local-first AI orchestration platform.
It automates the full development lifecycle — planning, implementation, review, and delivery — while distributing work across multiple machines via Supabase.

## How It Works

```
Your Laptop                          Remote Workers (GPU servers, etc.)
┌──────────────────────┐             ┌──────────────────────┐
│  cq                  │             │  cq hub worker start │
│  ├── Claude Code     │   Supabase  │  ├── LISTEN/NOTIFY   │
│  ├── 133 MCP tools   │◄──────────►│  ├── claim job       │
│  ├── Telegram bot    │   (jobs,    │  ├── execute         │
│  └── Knowledge DB    │  knowledge) │  └── report result   │
└──────────────────────┘             └──────────────────────┘
```

- **No server required** — Supabase handles job queue, knowledge sync, auth, and storage
- **Works with any AI CLI** — Claude Code, Gemini CLI, Codex, Cursor
- **Telegram control** — pair a bot and control from your phone

## Quick Start

```sh
# Install
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh

# Log in
cq auth login

# Start Claude Code
cq claude

# (Optional) Pair a Telegram bot for phone control
cq setup
```

## Distributed Workers

Run experiments across multiple machines — no server to manage:

```sh
# On your GPU server:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login --device
cq hub worker start     # connects to Supabase, waits for jobs

# On your laptop:
cq hub submit --run "python train.py --lr 1e-4"
# → Worker picks it up and executes
```

## Workflow

```
/pi "idea..."                    → ideation → idea.md
/c4-plan "feature description"   → discovery + design + tasks
/c4-run                          → spawn workers, implement in parallel
/c4-finish                       → build · test · docs · commit
/c4-status                       → check progress at any time
```

## How It Looks

### Building a new feature

> **You:** "JWT 인증 추가해줘"

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery → Design → Tasks
    T-001  OAuth provider interface
    T-002  Google provider
    T-003  GitHub provider
    T-004  JWT middleware
    T-005  Integration tests

/c4-run

  ◆ T-001  [worker-a]  ████████░░  implementing...
  ◆ T-002  [worker-b]  ████░░░░░░  implementing...
  ...
  ✓ All tasks complete → auto-polish → done
```

### Distributed experiments

> **You:** "backbone 3개 비교 돌려"

```
cq hub submit --run "python train.py --backbone resnet50"
cq hub submit --run "python train.py --backbone efficientnet"
cq hub submit --run "python train.py --backbone vit"

# Workers on different machines pick up and run in parallel
# Results accumulate in Supabase knowledge base
```

### Quick bug fix

> **You:** "모바일에서 버튼 안 돼"

```
/c4-quick "fix login button on mobile"
  ✓ Fixed → reviewed → merged
  Changed: src/components/LoginButton.tsx (+3 -1)
```

## Telegram

Pair a Telegram bot and control CQ from your phone:

```sh
cq setup    # BotFather → token → pairing (one-time)
cq          # select bot → Claude Code + Telegram session
```

Then from Telegram:
- Send messages → Claude Code processes and responds
- Receive notifications when tasks/experiments complete
- Works as a memo pad — messages queue while laptop is off

## Knowledge System

CQ accumulates knowledge from every task and experiment:

```
Task complete → auto-record (discoveries, patterns, concerns)
                    ↓
              Knowledge DB (FTS + vector search)
                    ↓
Next task assigned → auto-inject relevant past knowledge
                    ↓
              Better implementation → record → ...
```

Self-reinforcing loop. Knowledge syncs across devices via Supabase.

## Soul & Learning

CQ learns your coding style and evolves over time:

- **Persona** — extracts patterns from your git diffs
- **POP** — Personal Ontology Pipeline (conversation → knowledge)
- **Soul** — review priorities, quality philosophy

## Sessions

```sh
cq claude -t myproject    # start or resume a named session
cq ls                     # list bots and sessions
```

## Config

```sh
cq auth login              # GitHub OAuth → auto-configures cloud
cq auth login --device     # headless/SSH: device code flow
cq doctor                  # check installation health
```

No manual config editing required. `cq auth login` sets up everything.

## Update

Re-run the install command:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Requirements

- macOS Apple Silicon (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) installed
- `curl` available

## License

[MIT + Commons Clause](LICENSE) — free to use and modify, commercial resale prohibited.

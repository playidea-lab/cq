# What's included

CQ ships as a **single binary** with all features included. No tier selection needed.

::: tip Single tier since v1.34.0
Previously CQ shipped in three tiers (solo, connected, full). The binary size difference was only 295KB, so everything is now bundled together.
:::

## All features

Every CQ install includes:

- **169+ MCP tools** (~40 shown by default, `CQ_TOOL_TIER=full` for all)
- **39 skills** with ★ (core) and [internal] markers
- **Task management** — plan → run → review → finish
- **Polish & Refine gates** — Go-level quality enforcement
- **Local SQLite** database + **Supabase** cloud sync
- **Git worktree** isolation per worker
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama (cloud-managed keys)
- **C3 EventBus** — gRPC event bus for real-time notifications
- **C0 Drive** — Supabase Storage file storage
- **C9 Knowledge** — semantic search + pgvector for cross-project knowledge
- **Persona/Soul Evolution** — coding style pattern learning
- **C6 Secret Central** — encrypted secret sync (Supabase-backed, cache-first)
- **Relay** — WSS-based NAT traversal for remote MCP access. Token-free `.mcp.json` — `cq serve` auto-injects JWT
- **Telegram bot** — job completion notifications + slash commands
- **Distributed workers** — Supabase LISTEN/NOTIFY queue (NAT-safe)
- **3-Layer Ontology** — L1 local → L2 project → L3 collective pattern learning
- **Worker survivability** — systemd `Restart=always`, WSL2 Task Scheduler, auto-linger
- **Learn Loop** — 4 wires: submit→learn, reject→warning, get_task→inject, hook_deny→inject
- **Auto-routing** — Small (direct edit), Medium (/c4-quick), Large (/pi → plan → run → finish)

## Tool tiering

CQ has 169+ tools but shows only ~40 by default to keep the tool list manageable. To see all tools:

```sh
export CQ_TOOL_TIER=full
```

## Cloud features

Cloud features (Supabase sync, LLM Gateway, Knowledge, Relay) activate automatically after login:

```sh
cq    # handles login + service install automatically
```

No API keys needed. No manual config. The cloud becomes your SSOT for tasks, knowledge, and LLM calls.

## Config file location

CQ looks for config at `~/.c4/config.yaml`. For most users, no config is required — `cq` handles everything automatically after login.

For advanced customization:

```yaml
# ~/.c4/config.yaml

# Task storage
task_store:
  type: supabase  # or sqlite for local-only

# Background daemon (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30   # reset in_progress tasks stuck longer than this
    interval_seconds: 60

# Hub — distributed worker queue (uses Supabase LISTEN/NOTIFY)
hub:
  enabled: true

# Permission reviewer (bash hook)
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true
```

## Build tags (advanced)

For contributors building from source, features are controlled by build tags:

| Tag | Feature |
|-----|---------|
| `c0_drive` | File storage |
| `c3_eventbus` | gRPC event bus |
| `hub` | Distributed worker queue |
| `llm_gateway` | LLM proxy |
| `cdp` | Chrome DevTools Protocol |
| `gpu` | GPU job scheduler |
| `c1_messenger` | Telegram bot |
| `research` | ML experiment loop |
| `skills_embed` | Embedded skills |
| `c7_observe` | Observe trace system |

Default build includes all tags. See `Makefile` for details.

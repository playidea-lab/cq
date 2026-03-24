<div align="center">

# CQ

**External Brain for AI**

AI codes fast but forgets, skips planning, and doesn't learn.
CQ is the brain it's missing.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![MCP Tools](https://img.shields.io/badge/MCP_Tools-148-blueviolet)
![License](https://img.shields.io/badge/License-Personal_Study-orange)

</div>

---

## Quick Start

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq               # Login + start service (one-time)
cq claude        # Start building
```

Works with **Claude Code, Cursor, Codex CLI, Gemini CLI** — any MCP-compatible tool.

Update anytime: `cq update`

## What CQ Does

**Plans before coding** — Requirements analysis, architecture decisions, and task breakdown with Definition of Done — before a single line is written.

**Gates, not trust** — 6-axis review (correctness, security, reliability, observability, testing, readability). Compiled into the binary — not optional.

**Learns from experience** — Every review rejection becomes a warning for the next task. Past decisions and patterns are auto-extracted into reusable knowledge.
## Architecture

```
┌──────────────────┐          ┌────────────────────────────┐
│ Local (Thin Agent)│  JWT    │ Cloud (Supabase)            │
│                   │◄───────►│                             │
│ Hands:            │         │ Brain:                      │
│  ├ Files / Git    │         │  ├ Tasks (Postgres)         │
│  ├ Build / Test   │         │  ├ Knowledge (pgvector)     │
│  ├ LSP analysis   │         │  ├ LLM Proxy (Edge Fn)     │
│  └ MCP bridge     │         │  ├ Quality Gates            │
│                   │         │  └ Hub (distributed jobs)   │
│ Service (cq serve)│   WSS   │                             │
│  ├ Relay ─────────┼────────►│  Relay (Fly.io)             │
│  ├ Relay proxy    │         │  └ NAT traversal            │
│  ├ EventBus       │         │                             │
│  └ Token refresh  │         │                             │
└──────────────────┘          └────────────────────────────┘

solo:       Everything local (SQLite + your API key)
connected:  Brain in cloud + relay (cq → login + serve)
full:       Connected + GPU workers + research loop
```

## Learn More

[Installation](https://playidea-lab.github.io/cq/guide/install) | [Quick Start](https://playidea-lab.github.io/cq/guide/quickstart) | [Architecture](docs/ARCHITECTURE.md) | [Tiers](https://playidea-lab.github.io/cq/guide/tiers)

## Development

```bash
cd c4-core && go build ./... && go test -p 1 ./...   # Go
uv run pytest tests/                                   # Python
cq doctor                                              # Health check
```

## License

Personal Study & Research License (Non-Commercial). See [LICENSE.md](./LICENSE.md). Copyright (c) 2026 PlayIdeaLab.

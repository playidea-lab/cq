<div align="center">

# CQ

**External Brain for AI**

Every AI conversation becomes permanent knowledge.
CQ gives AI persistent memory, quality gates, and distributed execution.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![Version](https://img.shields.io/badge/version-v1.37-blue)
![MCP Tools](https://img.shields.io/badge/MCP_Tools-169-blueviolet)
![License](https://img.shields.io/badge/License-Personal_Study-orange)

</div>

---

## Quick Start

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq               # Login + start service (one-time)
cq claude        # Start building
```

Works with **Claude Code, ChatGPT, Cursor, Codex CLI, Gemini** — any MCP-compatible tool.

Update anytime: `cq update`

## What CQ Does

### External Brain — Memory That Persists Across AI Platforms

Connect CQ as a remote MCP server and every AI conversation contributes to your knowledge base. ChatGPT discovers a bug root cause? Claude picks it up in the next session. Decisions, patterns, and preferences follow you everywhere.

- **AI Self-Capture** — Tool descriptions engineered so AI proactively saves knowledge without being asked
- **Cross-Platform Search** — Vector + FTS + ilike fallback across all stored knowledge
- **OAuth 2.1** — Secure third-party access via Cloudflare Worker MCP proxy

### Quality Gates — Plans Before Coding, 6-Axis Review

Requirements analysis (EARS), architecture decisions (ADR), and task breakdown with Definition of Done — before a single line is written. 6-axis review (correctness, security, reliability, observability, testing, readability) is compiled into the binary — not optional.

### Growth Loop — AI Learns Your Preferences

Every review rejection, decision, and session summary feeds back as context. The AI adapts to your patterns over time, not just within a single conversation.

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
│  ├ EventBus       │         │  └ NAT traversal            │
│  └ Token refresh  │         │                             │
└──────────────────┘          │ External Brain (CF Worker)  │
                              │  ├ OAuth 2.1 MCP proxy      │
Any AI (ChatGPT,   ── MCP ──►│  ├ Knowledge record/search  │
 Claude, Gemini)              │  └ Session summary          │
                              └────────────────────────────┘

solo:       Everything local (SQLite + your API key)
connected:  Brain in cloud + relay (login + serve)
full:       Connected + GPU workers + research loop
```

## Key Components

| Component | Description |
|-----------|-------------|
| **Go MCP Server** | 169 tools (118 base + 26 Hub + 25 conditional), Registry-based |
| **Knowledge** | FTS5 + pgvector (OpenAI 1536d) + 3-way RRF + auto-distill |
| **Hub** | Distributed job queue, DAG engine, artifact store, cron |
| **Session** | Auto-summarize via LLM, context injection on startup |
| **Research Loop** | Autonomous ML experiment cycle (plan→train→evaluate→iterate) |
| **36 Skills** | Claude Code slash commands (/plan, /run, /finish, /pi, etc.) |

## Learn More

[Documentation](https://cq.pilab.kr) | [Installation](https://cq.pilab.kr/guide/install) | [Quick Start](https://cq.pilab.kr/guide/quickstart) | [Architecture](https://cq.pilab.kr/reference/architecture)

📖 **Full documentation at [cq.pilab.kr](https://cq.pilab.kr)**

## Development

```bash
cd c4-core && make install                             # Build + install
cd c4-core && go build ./... && go test -p 1 ./...    # Go tests
uv run pytest tests/                                   # Python tests
cq doctor                                              # Health check
```

## License

Personal Study & Research License (Non-Commercial). See [LICENSE.md](./LICENSE.md). Copyright (c) 2026 PlayIdeaLab.

# The C-Series Ecosystem

CQ is the CLI and distribution layer for **C4 Engine**, which is part of a broader ecosystem of interconnected components called the **C-series**.

## Philosophy

> "The only truth is data. Implement minimally, let results speak."

The C-series is built around three principles:

1. **Data as single source of truth** — every decision is validated by data, not opinion
2. **Minimal implementation first** — start with what works, add complexity only when needed
3. **Everything is versioned and reproducible** — data, code, experiments are all tracked

Human or agent, the quality bar is the same. Work is always reviewed.

---

## The Components

```
C0 Drive      — Cloud file storage (Supabase Storage)
C1 Messenger  — Tauri 2.x unified dashboard (4-tab: Messenger, Docs, Settings, Team)
C2 Docs       — Document lifecycle (parsing, workspace, profile)
C3 EventBus   — gRPC event bus (UDS + WebSocket + DLQ)
C4 Engine     — MCP orchestration engine  ← you are here
C5 Hub        — Distributed job queue (worker pull model, lease-based)
C9 Knowledge  — Knowledge management (FTS5 + pgvector + embedding + ingestion)
```

Each component can run standalone or together. CQ's tiers reflect this:

| Tier | Components active |
|------|------------------|
| solo | C4 only |
| connected | C4 + C0 + C3 + C9 + LLM Gateway |
| full | All components |

---

## C4 Engine (this project)

C4 is the orchestration core. It exposes **100+ MCP tools** (`c4_*`) to Claude Code and manages:

- **Task lifecycle** — create, assign, review, checkpoint, complete
- **Worker isolation** — each worker gets a fresh git worktree
- **Knowledge accumulation** — discoveries recorded automatically, injected into future tasks
- **Secret store** — AES-256-GCM, never in config files
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama
- **Skills** — 22 slash commands embedded in the binary

---

## C1 Messenger

Tauri 2.x desktop app with four views:

- **Messenger** — real-time team chat via Supabase Realtime, agent presence
- **Documents** — local file parsing via C2
- **Settings** — `.claude/` and `.c4/` config viewer/editor
- **Team** — project dashboard, Supabase-backed

Members are unified: humans, agents, and systems are all equal participants.

---

## C2 Docs

Document lifecycle management:

- Parses PDF, EPUB, HTML, Markdown into structured workspace
- Profile/persona system — learns from user edits
- Powers `/c2-paper-review` and `/c4-review` skills

---

## C3 EventBus

gRPC event bus connecting all components:

- **16 event types**: `task.completed`, `knowledge.recorded`, `hub.job.completed`, etc.
- **DLQ** (dead letter queue) for failed deliveries
- **Filter v2**: `$eq`, `$ne`, `$gt`, `$in`, `$regex`, `$exists`
- **HMAC-SHA256 webhooks** for external integrations

---

## C5 Hub

Distributed job queue for running workers at scale:

- **Pull model** — workers poll for jobs, no push dependencies
- **Lease-based** — jobs are leased with timeout, auto-requeued on failure
- **Artifact pipeline** — workers download inputs, upload outputs via signed URLs
- REST + WebSocket API

---

## C9 Knowledge

Multi-layer knowledge store:

- **FTS5** full-text search (SQLite)
- **pgvector** semantic search (Supabase)
- **Embedding** pipeline with usage tracking
- **Ingestion** from docs, web pages, experiments
- **Publish/pull** for cross-project knowledge sharing

---

## How they connect

```
Claude Code
    │
    ▼ MCP (stdio)
C4 Engine ──────────────── C9 Knowledge (search + record)
    │                              ▲
    ├── C3 EventBus ───────────────┘ (task.completed → auto-record)
    │       │
    │       └── C1 Messenger (real-time notifications)
    │
    ├── C5 Hub (distributed workers)
    │       └── Artifact storage via C0 Drive
    │
    └── LLM Gateway (Anthropic / OpenAI / Gemini / Ollama)
```

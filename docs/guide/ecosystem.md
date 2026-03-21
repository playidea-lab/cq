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
C5 Hub        — Supabase-native worker queue (pgx LISTEN/NOTIFY, lease-based)
C6 Guard      — RBAC access control (policy, audit, role assignment)
C7 Observe    — Observability layer (metrics, logs, tracing middleware)
C8 Gate       — External integrations (webhooks, scheduler, connectors)
C9 Knowledge  — Knowledge management (FTS5 + pgvector + embedding + ingestion)
```

Each component can run standalone or together. CQ's tiers reflect this:

| Tier | Components active |
|------|------------------|
| solo | C4 only |
| connected | C4 + C0 + C3 + C9 + LLM Gateway |
| full | All components (+ C1, C5 worker queue, C6, C7, C8) |

C6/C7/C8 are activated via build tags (`c6_guard`, `c7_observe`, `c8_gate`) — they are always compiled into the `full` tier binary.

---

## C4 Engine (this project)

C4 is the orchestration core. It exposes **100+ MCP tools (varies by tier)** (`c4_*`) to Claude Code and manages:

- **Task lifecycle** — create, assign, review, checkpoint, complete
- **Worker isolation** — each worker gets a fresh git worktree
- **Knowledge accumulation** — discoveries recorded automatically, injected into future tasks
- **Secret store** — AES-256-GCM, never in config files
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama
- **Skills** — 36 slash commands embedded in the binary (/pi, c9-* research loop, and more)

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

- **19+ event types (28+ with worker queue enabled)**: `task.created/started/completed/blocked/stale`, `checkpoint.approved/rejected`, `review.changes_requested`, `validation.passed/failed`, `knowledge.recorded/searched`, `lighthouse.promoted`, `llm.cache_miss_alert`, `persona.evolved`, `soul.updated`, `research.recorded/started`; worker queue adds `hub.job.completed/failed/submitted/cancelled/retried`, `hub.dag.executed`, `hub.worker.registered`
- **DLQ** (dead letter queue) for failed deliveries
- **Filter v2**: `$eq`, `$ne`, `$gt`, `$in`, `$regex`, `$exists`
- **HMAC-SHA256 webhooks** for external integrations

---

## C5 Hub (Supabase Worker Queue)

Supabase-native distributed job queue for running workers at scale:

- **pgx LISTEN/NOTIFY** — workers subscribe to Supabase Postgres for real-time job delivery
- **Lease-based** — jobs are leased with timeout, auto-requeued on failure
- **VRAM-aware scheduling** — GPU workers matched by free VRAM; CPU fallback configurable
- **Artifact pipeline** — workers download inputs, upload outputs via signed URLs
- **Log retention** — automatic rotation (50k rows) + 7-day cleanup
- Supabase PostgREST + RPC API

Workers connect directly to Supabase — no Hub server process to start or manage.

---

## C6 Guard

Role-based access control for C4 tools:

- **Policy engine** — allow/deny rules per tool, per role
- **Audit log** — every tool call recorded with actor and decision
- **Role assignment** — assign roles to agents or users
- Activated with `c6_guard` build tag

---

## C7 Observe

Observability layer for the C4 engine:

- **Metrics** — request counts, latency, error rates per tool
- **Structured logs** — `slog`-based with configurable level and format
- **Middleware** — automatically instruments every MCP tool call
- Activated with `c7_observe` build tag

---

## C8 Gate

External integration hub:

- **Webhooks** — register endpoints, test payloads, HMAC-SHA256 signed
- **Scheduler** — cron-style jobs that trigger C4 tasks
- **Connectors** — Telegram and GitHub out of the box
- Activated with `c8_gate` build tag

---

## Persona & Soul Evolution

CQ learns from your coding patterns and evolves its behavior over time:

- **Pattern extraction** — analyzes diffs between AI drafts and your final edits
- **Soul persistence** — patterns stored in `.c4/souls/{user}/raw_patterns.json`
- **Evolution** — `scripts/soul-evolve.sh` synthesizes accumulated patterns into `soul-developer.md`
- `c4_persona_learn` / `c4_soul_get` / `c4_soul_set` MCP tools

---

## POP (Personal Ontology Pipeline)

Automatically extracts knowledge proposals from conversation and crystallizes them into Soul:

- **5-stage pipeline** — Extract → Consolidate → Propose → Validate → Crystallize
- **Confidence gating** — only HIGH confidence (≥0.8) proposals reach Soul
- **Gauge tracking** — merge_ambiguity / avg_fan_out / contradictions / temporal_queries
- **Atomic writes** — soul_backup/ maintained (10 snapshots)
- `c4_pop_extract` / `c4_pop_status` / `c4_pop_reflect` MCP tools
- `cq pop status` CLI command

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
    ├── Supabase (worker queue via pgx LISTEN/NOTIFY)
    │       └── Artifact storage via C0 Drive
    │
    └── LLM Gateway (Anthropic / OpenAI / Gemini / Ollama)
```

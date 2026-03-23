# The CQ Ecosystem

CQ is the CLI and distribution layer for the **Engine**, which is part of a broader ecosystem of interconnected components.

## Philosophy

> "The only truth is data. Implement minimally, let results speak."

The ecosystem is built around three principles:

1. **Data as single source of truth** — every decision is validated by data, not opinion
2. **Minimal implementation first** — start with what works, add complexity only when needed
3. **Everything is versioned and reproducible** — data, code, experiments are all tracked

Human or agent, the quality bar is the same. Work is always reviewed.

---

## The Components

```
Drive      — Cloud file storage (Supabase Storage)
Docs       — Document lifecycle (parsing, workspace, profile)
EventBus   — gRPC event bus (UDS + WebSocket + DLQ)
Engine     — MCP orchestration engine  ← you are here
Hub        — Supabase-native worker queue (pgx LISTEN/NOTIFY, lease-based)
Guard      — RBAC access control (policy, audit, role assignment)
Observe    — Observability layer (metrics, logs, tracing middleware)
Gate       — External integrations (webhooks, scheduler, connectors)
Knowledge  — Knowledge management (FTS5 + pgvector + embedding + ingestion)
```

Each component can run standalone or together. CQ's tiers reflect this:

| Tier | Components active |
|------|------------------|
| solo | Engine only |
| connected | Engine + Drive + EventBus + Knowledge + LLM Gateway |
| full | All components (+ Hub worker queue, Guard, Observe, Gate) |

Guard, Observe, and Gate are activated via build tags — they are always compiled into the `full` tier binary.

---

## Engine (this project)

The Engine is the orchestration core. It exposes **144 MCP tools (varies by tier)** (`c4_*`) to Claude Code and manages:

- **Task lifecycle** — create, assign, review, checkpoint, complete
- **Worker isolation** — each worker gets a fresh git worktree
- **Knowledge accumulation** — discoveries recorded automatically, injected into future tasks
- **Secret store** — AES-256-GCM, never in config files
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama
- **Skills** — 36 slash commands embedded in the binary (/pi, research loop, and more)

---

## Docs

Document lifecycle management:

- Parses PDF, EPUB, HTML, Markdown into structured workspace
- Profile/persona system — learns from user edits
- Powers `/c4-review` and paper review skills

---

## EventBus

gRPC event bus connecting all components:

- **19+ event types (28+ with worker queue enabled)**: `task.created/started/completed/blocked/stale`, `checkpoint.approved/rejected`, `review.changes_requested`, `validation.passed/failed`, `knowledge.recorded/searched`, `lighthouse.promoted`, `llm.cache_miss_alert`, `persona.evolved`, `soul.updated`, `research.recorded/started`; worker queue adds `hub.job.completed/failed/submitted/cancelled/retried`, `hub.dag.executed`, `hub.worker.registered`
- **DLQ** (dead letter queue) for failed deliveries
- **Filter v2**: `$eq`, `$ne`, `$gt`, `$in`, `$regex`, `$exists`
- **HMAC-SHA256 webhooks** for external integrations

---

## Hub (Supabase Worker Queue)

Supabase-native distributed job queue for running workers at scale:

- **pgx LISTEN/NOTIFY** — workers subscribe to Supabase Postgres for real-time job delivery
- **Lease-based** — jobs are leased with timeout, auto-requeued on failure
- **VRAM-aware scheduling** — GPU workers matched by free VRAM; CPU fallback configurable
- **Artifact pipeline** — workers download inputs, upload outputs via signed URLs
- **Log retention** — automatic rotation (50k rows) + 7-day cleanup
- Supabase PostgREST + RPC API

Workers connect directly to Supabase — no Hub server process to start or manage.

---

## Guard

Role-based access control for Engine tools:

- **Policy engine** — allow/deny rules per tool, per role
- **Audit log** — every tool call recorded with actor and decision
- **Role assignment** — assign roles to agents or users
- Activated with `c6_guard` build tag

---

## Observe

Observability layer for the Engine:

- **Metrics** — request counts, latency, error rates per tool
- **Structured logs** — `slog`-based with configurable level and format
- **Middleware** — automatically instruments every MCP tool call
- Activated with `c7_observe` build tag

---

## Gate

External integration hub:

- **Webhooks** — register endpoints, test payloads, HMAC-SHA256 signed
- **Scheduler** — cron-style jobs that trigger Engine tasks
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

## Knowledge

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
Engine ──────────────── Knowledge (search + record)
    │                              ▲
    ├── EventBus ──────────────────┘ (task.completed → auto-record)
    │
    ├── Supabase (worker queue via pgx LISTEN/NOTIFY)
    │       └── Artifact storage via Drive
    │
    └── LLM Gateway (Anthropic / OpenAI / Gemini / Ollama)
```

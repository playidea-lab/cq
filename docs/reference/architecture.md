# Architecture Reference

CQ is an **External Brain for AI** -- a system where every AI conversation becomes permanent knowledge, quality gates ensure code integrity, and distributed execution enables remote GPU training. This document describes the core components.

---

## System Overview

```
+------------------+          +----------------------------+
| Local (Thin Agent)|  JWT    | Cloud (Supabase)            |
|                   |<------->|                             |
| Hands:            |         | Brain:                      |
|  +- Files / Git  |         |  +- Tasks (Postgres)        |
|  +- Build / Test |         |  +- Knowledge (pgvector)    |
|  +- LSP analysis |         |  +- LLM Proxy (Edge Fn)    |
|  +- MCP bridge   |         |  +- Quality Gates           |
|                   |         |  +- Hub (distributed jobs)  |
| Service (cq serve)|   WSS  |                             |
|  +- Relay --------+-------->|  Relay (Fly.io)             |
|  +- EventBus     |         |  +- NAT traversal            |
|  +- Token refresh|         |                             |
+------------------+          | External Brain (CF Worker)  |
                              |  +- OAuth 2.1 MCP proxy     |
Any AI (ChatGPT,   --- MCP -->|  +- Knowledge record/search |
 Claude, Gemini)              |  +- Session summary         |
                              +----------------------------+

solo:       Everything local (SQLite + your API key)
connected:  Brain in cloud + relay (login + serve)
full:       Connected + GPU workers + research loop
```

---

## Deployment Tiers

| Tier | Data SSOT | LLM | Setup |
|------|-----------|-----|-------|
| **solo** | Local SQLite | User's API key | `config.yaml` required |
| **connected** | Supabase (cloud-primary) | PI Lab LLM Proxy | `cq auth login` + `cq serve` |
| **full** | Supabase (cloud-primary) | PI Lab LLM Proxy | Connected + GPU workers |

- Cloud failure falls back to SQLite (read-only)
- ~70 tools cloud-primary, ~48 tools require local (files/git/build)
- External Brain: ChatGPT/Claude/Gemini connect via OAuth MCP (no local install needed)

---

## Go MCP Server (c4-core/)

The primary MCP server. Serves 169 tools via stdio transport.

```
Claude Code -> Go MCP Server (stdio, 169 tools)
                +-> Go native (28): state, tasks, files, git, validation, config
                +-> Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                +-> Soul/Persona/Twin (10): soul_evolve, persona_learn, twin_record, ...
                +-> LLM Gateway (3): llm_call, llm_providers, llm_costs
                +-> CDP + WebMCP (5): cdp_run, webmcp_discover, web_fetch, ...
                +-> Drive (6): upload, download, list, delete, info, mkdir
                +-> File Index (2): fileindex_search, fileindex_status
                +-> Session (3): session_index, session_summarize, session_snapshot
                +-> Memory (1): memory_import
                +-> Relay (2): cq_workers, cq_relay_call
                +-> Knowledge (13): record, search, distill, ingest, sync, publish, ...
                +-> Hub Client (19, conditional): job, worker, DAG, artifact, cron
                +-> Worker Standby (3, Hub): standby, complete, shutdown
                +-> C7 Observe (4, build tag): metrics, logs, trace, status
                +-> C6 Guard (5, build tag): check, audit, policy, deny
                +-> C8 Gate (6, build tag): webhook, schedule, slack, github, ...
                +-> EventSink (1) + HubPoller (1)
                +-> JSON-RPC proxy (10) -> Python Sidecar
```

### Tool Tiering

- **Core** (40 tools): Always loaded, immediate availability
- **Extended** (129 tools): Loaded on demand, available after initialization
- **Conditional**: Hub tools require `serve.hub.enabled: true`; C7/C6/C8 require build tags

### Package Structure

```
c4-core/
+-- cmd/c4/           # CLI (cobra) + MCP server entry point
+-- internal/
    +-- mcp/          # Registry + stdio transport
    |   +-- apps/     # MCP Apps ResourceStore + embedded widget HTML
    |   +-- handlers/ # Per-tool handlers
    +-- bridge/       # Python sidecar manager (JSON-RPC/TCP, lazy start)
    +-- task/         # TaskStore (SQLite, Memory, Supabase)
    +-- state/        # State machine (INIT -> COMPLETE)
    +-- worker/       # Worker manager
    +-- validation/   # Validation runner (go test, pytest, cargo test auto-detect)
    +-- config/       # Config manager (YAML, env, economic presets)
    +-- cloud/        # Auth (OAuth), CloudStore, HybridStore, TokenProvider
    +-- hub/          # Hub REST+WS client (26 tools)
    +-- daemon/       # Local job scheduler (GPU-aware)
    +-- eventbus/     # C3 EventBus v4 (gRPC, WS bridge, DLQ, filter v2)
    +-- knowledge/    # Knowledge (FTS5 + Vector + Embedding + Sync)
    +-- research/     # Research iteration store
    +-- c2/           # Workspace/Profile/Persona + webcontent
    +-- drive/        # Drive client (TUS resumable upload)
    +-- fileindex/    # Cross-device file search
    +-- session/      # Session tracking + LLM summarizer
    +-- memory/       # ChatGPT/Claude session import pipeline
    +-- relay/        # WebSocket relay client
    +-- llm/          # LLM Gateway (Anthropic, OpenAI, Gemini, Ollama)
    +-- cdp/          # Chrome DevTools Protocol + WebMCP
    +-- observe/      # C7 Observe (c7_observe build tag)
    +-- guard/        # C6 Guard (c6_guard build tag)
    +-- gate/         # C8 Gate (c8_gate build tag)
```

### Build and Install

```bash
# Build + install (CRITICAL -- always use make install)
cd c4-core && make install

# Tests
cd c4-core && go test ./...

# Environment diagnostics
cq doctor
```

---

## MCP Apps (Widget System)

When a tool is called with `format=widget`, the response includes `_meta.ui.resourceUri`. The MCP client fetches the HTML via `resources/read` and renders it in a sandboxed iframe.

```
Tool call (format=widget)
  -> handler returns {data: {...}, _meta: {ui: {resourceUri: "ui://cq/..."}}}
  -> client calls resources/read("ui://cq/...")
  -> ResourceStore returns embedded HTML
  -> client renders in sandboxed iframe
```

| Widget URI | Tool | Description |
|-----------|------|-------------|
| `ui://cq/dashboard` | `c4_dashboard` | Project status summary |
| `ui://cq/job-progress` | `c4_job_status` | Job progress |
| `ui://cq/job-result` | `c4_job_summary` | Job results |
| `ui://cq/experiment-compare` | `c4_experiment_search` | Experiment comparison |
| `ui://cq/task-graph` | `c4_task_graph` | Task dependency graph |
| `ui://cq/nodes-map` | `c4_nodes_map` | Connected nodes map |
| `ui://cq/knowledge-feed` | `c4_knowledge_search` | Knowledge feed |
| `ui://cq/cost-tracker` | `c4_llm_costs` | LLM cost tracker |
| `ui://cq/test-results` | `c4_run_validation` | Test results |
| `ui://cq/git-diff` | `c4_diff_summary` | Git diff viewer |
| `ui://cq/error-trace` | `c4_error_trace` | Error trace viewer |

---

## Knowledge System

4-layer pipeline: every task decision becomes searchable knowledge for future tasks.

```
Plan (knowledge_search) -> Task DoD (Rationale) -> Worker (knowledge_context injected)
     ^                                                       |
pattern_suggest <- distill <- autoRecordKnowledge <- Worker complete (handoff)
```

- **FTS5**: Full-text search on all knowledge records
- **pgvector**: OpenAI 1536-dim embeddings (or Ollama 768-dim nomic-embed-text)
- **3-way RRF**: Ranked fusion of FTS + vector + popularity scores
- **Auto-distill**: Triggered by `/c4-finish` when knowledge count >= 5
- **Cloud sync**: Local SQLite <-> Supabase pgvector sync
- **Cross-project**: `c4_knowledge_publish` / `c4_knowledge_pull` for sharing

### Knowledge handoff (c4_submit)

Workers submit structured handoff with their task:

```json
{
  "summary": "What was implemented",
  "files_changed": ["src/feature.go"],
  "discoveries": ["pattern X works better than Y"],
  "concerns": ["edge case Z not handled"],
  "rationale": "Why approach A was chosen"
}
```

This is auto-parsed and recorded as knowledge for future workers.

---

## Hub (Distributed Execution)

The Hub is a distributed job queue backed by Supabase PostgreSQL. Workers pull jobs via a lease model.

```
Developer (laptop)
  +-- c4_job_submit(spec, routing={tags: ["gpu"]}) -->+
                                                      |
                                    Supabase: hub_jobs INSERT
                                              | pg_notify('new_job')
                                              v
                                    Worker (remote GPU server)
                                      +- ClaimJob (lease)
                                      +- Execute
                                      +- Upload artifacts
                                      +- CompleteJob
```

### DAG Pipelines

```
c4_hub_dag_create (nodes + edges)
    |
    v (topological sort -> root nodes auto-submitted)
    v
Worker completes node -> advance_dag -> next layer released
    |
    v
All nodes complete -> DAG complete event
```

### Worker Affinity

Workers are automatically routed based on affinity scores:

```
affinity_score = project_match * 10 + tag_match * 3 + recency * 2 + success_rate * 5
```

View affinity scores: `cq hub workers` (shows `AFFINITY` column).

---

## Relay (NAT Traversal)

The relay enables external MCP clients to reach local workers through NAT.

```
External MCP client (Cursor / Codex / Gemini CLI)
    | HTTPS (MCP over HTTP)
    v
cq-relay.fly.dev  [Go relay server]
    ^ WSS (outbound, worker connects first)
cq serve  [local / cloud worker]
    |
    v
Go MCP Server (stdio) + Python Sidecar
```

Authentication flow:
1. `cq auth login` -> Supabase Auth -> JWT issued + relay URL auto-configured
2. `cq serve` starts -> relay WSS connection (Authorization: Bearer JWT)
3. Relay verifies token, registers worker tunnel
4. External client -> `https://cq-relay.fly.dev/<worker-id>` -> relay -> WSS -> worker

---

## EventBus (C3)

gRPC UDS daemon with WebSocket bridge. 18 event types. 78 tests.

```
EventBus (gRPC UDS)
    |-- Rules engine (YAML routing)
    |-- DLQ (dead letter queue)
    |-- WebSocket bridge (external subscribers)
    +-- HMAC-SHA256 webhook delivery
```

Event types:

| Category | Events |
|----------|--------|
| Tasks | `task.completed`, `task.updated`, `task.blocked`, `task.created` |
| Checkpoints | `checkpoint.approved`, `checkpoint.rejected` |
| Reviews | `review.changes_requested` |
| Validation | `validation.passed`, `validation.failed` |
| Knowledge | `knowledge.recorded`, `knowledge.searched` |
| Hub | `hub.job.completed`, `hub.job.failed`, `hub.worker.started`, `hub.worker.offline` |
| Observability | `tool.called` (C7), `guard.denied` (C6) |

---

## External Brain (Cloudflare Worker)

OAuth 2.1 MCP proxy. Any AI (ChatGPT, Claude web, Gemini) can access CQ knowledge without local install.

Tools exposed via External Brain:

| Tool | Description |
|------|-------------|
| `c4_knowledge_record` | AI proactively saves knowledge (5-condition trigger in tool description) |
| `c4_knowledge_search` | Vector + FTS + ilike 3-stage fallback search |
| `c4_session_summary` | Capture complete session summary on conversation end |
| `c4_status` | Read current project state |

---

## Python Sidecar (c4/)

Go MCP server delegates 10 tools to Python via JSON-RPC/TCP (lazy-started).

```
Go MCP Server -- JSON-RPC/TCP --> Python Sidecar (10 tools)
                                    +-> LSP (7): find_symbol, get_overview, replace_body,
                                    |          insert_before/after, rename, find_refs
                                    |          (Python/JS/TS only -- Go/Rust: use c4_search_for_pattern)
                                    +-> Doc (2): parse_document, extract_text
                                    +-> Onboard (1): c4_onboard
```

- **Lazy Start**: Sidecar starts only on first proxy tool call
- **Graceful fallback**: If Python/uv unavailable, LSP/Doc tools are disabled (not a crash)

---

## State Machine

```
INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> REFINE -> POLISH -> COMPLETE
                                          |
                                          +-> HALTED (resumable)
```

| State | Meaning |
|-------|---------|
| INIT | Project created, no tasks yet |
| DISCOVERY | Gathering requirements (c4-plan Phase 1) |
| DESIGN | Architecture decisions (c4-plan Phase 2) |
| PLAN | Tasks created, ready to execute |
| EXECUTE | Workers active, tasks being claimed |
| CHECKPOINT | Phase gate reached, review in progress |
| HALTED | Execution paused, resumable with `/c4-run` |
| COMPLETE | All tasks done, ready for `/c4-finish` |

---

## Security: Permission Hook

Two-layer gate on all tool use and shell execution:

```
PreToolUse event
    |
    v
c4-gate.sh (pattern match)
    |-- allow_patterns -> immediate allow
    |-- model mode -> Haiku API decision
    |-- block_patterns -> block (with audit log)
    +-- fallback -> built-in safe patterns

PermissionRequest event
    |
    v
c4-permission-reviewer.sh (Haiku classification)
```

Configuration (`.c4/config.yaml`):

```yaml
permission_reviewer:
  enabled: true
  mode: hook        # "hook" (regex only) or "model" (Haiku API)
  auto_approve: true
  allow_patterns: []
  block_patterns: []
```

---

## Supabase Schema (Key Tables)

| Table | Purpose |
|-------|---------|
| `c4_tasks` | Task queue (state, assignments, commit SHAs) |
| `c4_documents` | Knowledge records (content, embeddings, FTS) |
| `c4_projects` | Project registry (owners, settings) |
| `hub_jobs` | Distributed job queue (spec, status, lease) |
| `hub_workers` | Registered workers (capabilities, affinity) |
| `hub_dags` | DAG pipeline definitions |
| `hub_cron_schedules` | Cron job definitions |
| `c4_drive_files` | Drive file metadata (hash, URLs, versions) |
| `c4_datasets` | Dataset registry with content-addressable versioning |
| `c1_messages` | Inter-session and messaging channel messages |
| `notification_channels` | Telegram/Dooray notification configs |

52 migrations, RLS policies on all user-facing tables, pgvector extension for embeddings.

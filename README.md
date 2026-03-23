<div align="center">

# CQ

**AI-Powered Project Orchestration System**

Plan, execute, review, and learn — automated end-to-end.

*CQ*: C-series (C0–C1–C2–C3–C4–C5–C9) as one organically connected ecosystem. CLI: `cq`.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![Python](https://img.shields.io/badge/Python-3.11+-3776AB?logo=python&logoColor=white)
![Tools](https://img.shields.io/badge/MCP_Tools-133-blueviolet)
![Tests](https://img.shields.io/badge/Tests-3%2C628+-brightgreen)
![License](https://img.shields.io/badge/License-Personal_Study-orange)

</div>

---

CQ turns Claude Code into a full project management system. It provides **133 MCP tools** (107 base + 26 Hub), plus 15 optional tiered tools (C6/C7/C8), a structured workflow engine, multi-lens code review, knowledge feedback loops, distributed job scheduling, and GPU-aware task management — all through natural language. Run the CLI with `cq`.

```
You: /c4-plan "Add user authentication with JWT"
CQ:  Creates 5 tasks with DoD, spawns workers, reviews each PR, learns from decisions.
```

## C Series Ecosystem (CQ)

C1·C2·C3·C4·C5·C9가 유기적으로 연결된 생태계.

```
C0 Drive    — Cloud file storage (Supabase Storage)
C1 Desktop  — Tauri 2.x project explorer (6-tab view)
C2 Docs     — Document lifecycle (parsing/workspace/profile)
C3 EventBus — gRPC event bus (UDS + WebSocket + DLQ)
C4 Engine   — MCP orchestration engine (this repo)
C5 Hub      — Distributed job queue server (Worker Pull, Lease-based)
C9 Knowledge — Knowledge management (FTS5 + pgvector + Embedding + Usage)
```

## How It Works

```
INIT ─▶ DISCOVERY ─▶ DESIGN ─▶ PLAN ─▶ EXECUTE ⇄ CHECKPOINT ─▶ REFINE ─▶ POLISH ─▶ COMPLETE
                                         │              │            │            │
                                    Worker mode     Multi-lens    Iterative    Build·Test·Review
                                    Direct mode     code review   quality loop until 0 changes
```

CQ breaks features into tasks, assigns them to workers (parallel) or claims them directly (sequential), auto-generates review tasks, and accumulates decisions as organizational knowledge.

## Architecture

```
Claude Code ──stdio──▶ Go MCP Server (107 base + 26 Hub = 133 tools, +15 optional tiered)
                        │
                        ├── Go Native ──────── State, Tasks, Files, Git, Validation
                        ├── SQLite Store ───── Specs, Designs, Checkpoints, Artifacts
                        ├── Knowledge (Go) ─── FTS5 + pgvector + Embedding + 3-way RRF
                        │                      ├── Usage Tracking (view/cite/search_hit)
                        │                      ├── Document Ingestion (chunker + RAG)
                        │                      └── Visibility (private/team/public)
                        ├── Research (Go) ──── Research Loop, C2 Workspace, GPU
                        ├── Soul Engine ────── Persona evolution (Analysis-Persistence-Evolution loop), Digital Twin, Reflection
                        ├── LLM Gateway ────── Claude / GPT / Gemini / Ollama + Embeddings
                        ├── CDP Runner ─────── Browser automation (DevTools Protocol)
                        ├── EventBus ──gRPC──▶ Event daemon (UDS + WebSocket + DLQ)
                        ├── Cloud Layer ────── Supabase (Auth, CloudStore, HybridStore)
                        ├── Drive ─────────── Supabase Storage (upload/download/mkdir)
                        ├── C1 Context Hub ─── Search, Mentions, Briefing
                        ├── Hub Client ─HTTP─▶ C5 Hub Server (distributed job queue)
                        │                      ├── Multi-tenant isolation (project_id)
                        │                      ├── DAG Orchestration
                        │                      └── Edge Deployment
                        │
                        └── JSON-RPC ──TCP──▶ Python Sidecar (10 tools)
                                              ├── LSP (Multilspy/Jedi — Python/JS/TS)
                                              └── C2 Document Parsing
```

| Component | Directory | Stack |
|-----------|-----------|-------|
| Go MCP Server | `c4-core/` | Go 1.22+, SQLite, Cobra CLI |
| C5 Hub Server | `c5/` | Go, SQLite, REST API, WebSocket |
| Python Sidecar | `c4/` | Python 3.11+, multilspy, sqlite-vec |
| C1 Desktop App | `c1/` | Tauri 2.x, React, Rust |
| Cloud Infra | `infra/supabase/` | PostgreSQL, pgvector, RLS |

## Quick Start

**Prerequisites:** Go 1.22+, Python 3.11+, [uv](https://docs.astral.sh/uv/)

```bash
# One-line remote install
curl -sSL https://git.pilab.co.kr/pi/cq/raw/main/install.sh | bash

# Or clone + install
git clone https://git.pilab.co.kr/pi/cq.git && cd cq
./install.sh

# Optional: install C5 Hub Server
./install.sh --with-hub
```

Restart Claude Code, then:

```bash
cq doctor           # Verify installation (8 health checks)
/c4-status          # Verify connection (133 tools registered)
/c4-plan "feature"  # Start planning
/c4-run             # Execute tasks
```

## Features

### Task Orchestration
- **Dual execution mode** — Worker (parallel, isolated worktrees) or Direct (sequential, shared workspace)
- **Review cascade** — Every task `T-001` auto-generates review `R-001`; rejections create `T-001-1 → R-001-1`
- **Checkpoint system** — APPROVE / REQUEST_CHANGES / REPLAN / REDESIGN decision points
- **Smart Auto Mode** — Automatically picks worker vs direct based on task dependencies

### Lighthouse (Spec-as-MCP TDD)

Define API contracts first, implement later — the lighthouse pattern brings TDD to MCP tool development.

```
Register spec ──▶ Stub + Task created ──▶ Worker gets spec context ──▶ Promote with schema check
```

```bash
# 1. Define the contract — stub tool is immediately available
c4_lighthouse(action="register", name="export_api",
  description="Batch export project data",
  input_schema='{"type":"object","properties":{"format":{"type":"string"}},"required":["format"]}',
  spec="## Export API\nReturns data in JSON or CSV format.")
# → Creates stub tool [LIGHTHOUSE] export_api
# → Auto-creates task T-LH-export_api-0

# 2. Worker receives full spec context on assignment
c4_get_task  # → TaskAssignment includes lighthouse_spec: {name, spec, input_schema}

# 3. After implementation, promote validates and removes stub
c4_lighthouse(action="promote", name="export_api")
# → Compares real tool schema against lighthouse spec (warnings on mismatch)
# → Marks T-LH-export_api-0 as done
# → Removes stub from registry
```

| Action | Description |
|--------|-------------|
| `register` | Create stub + auto-task (disable with `auto_task: false`) |
| `list` | All lighthouses with status counts (stub/implemented/deprecated) |
| `get` | Full spec, schema, version, linked task ID |
| `promote` | Validate schema → mark implemented → remove stub → complete task |
| `update` | Modify spec/schema/description (stub only, bumps version) |
| `remove` | Deprecate and unregister from MCP |

### Code Intelligence
- **Native LSP** — Go (go/ast), Dart (regex), Python/JS/TS (Jedi + multilspy via sidecar)
- **Symbol operations** — Find, rename, replace body, insert before/after across the project
- **Multi-lens review** — Security, performance, architecture, testing perspectives per review

### Knowledge & Learning (C9)
- **Knowledge feedback loop** — Plan→Execute→Record→Distill→Reuse 자동 순환
- **Auto-record on completion** — Task handoff (discoveries/concerns/rationale) → knowledge DB 자동 기록
- **Worker knowledge injection** — AssignTask 시 관련 knowledge context 자동 주입
- **Real embeddings** — OpenAI text-embedding-3-small (1536d) for semantic search
- **3-way hybrid search** — FTS5 + Vector similarity + Popularity ranking via Reciprocal Rank Fusion
- **Auto-distill** — Experiment 클러스터에서 Pattern 자동 추출 (finish 시)
- **Usage tracking** — Automatic view/cite/search_hit tracking with popularity boost
- **Cloud sync** — Bidirectional sync with Supabase (pgvector + RLS)
- **Persona & Soul Evolution** — 사용자의 코딩 스타일, 어조, 선호도를 학습하여 에이전트 행동 양식을 진화시키는 루프
  - **Analysis**: AI 초안 vs 사용자 수정본 Diff 분석 (tone softening, structured logging, error wrapping 패턴 추출)
  - **Persistence**: 추출된 패턴을 `.c4/souls/{user}/raw_patterns.json`에 누적
  - **Evolution**: Gemini 3.0 기반 `soul-evolve.sh`를 통해 기존 소울과 합성하여 진화된 `soul-developer.md` 생성
- **Digital Twin** — `c4_reflect` for pattern analysis, growth tracking, challenge identification

### Infrastructure
- **LLM Gateway** — Route to Claude, GPT, Gemini, or Ollama with cost tracking + embeddings
- **C5 Hub Server** — Distributed job queue with multi-tenant isolation, lease-based scheduling
- **Daemon Scheduler** — Local job queue with GPU allocation, duration estimation, and retry
- **DAG Orchestration** — Multi-step pipelines with dependency resolution
- **Edge Deployment** — Push artifacts to edge devices with auto-trigger rules
- **C3 EventBus** — gRPC event daemon with WebSocket bridge, DLQ, correlation tracking
- **C0 Drive** — Supabase Storage integration (upload, download, mkdir, list)
- **CDP Runner** — Browser automation via Chrome DevTools Protocol
- **cq serve** — Long-running service manager (StaleChecker, EventBus, C5 Hub subprocess); install as OS service via `cq serve install`

### Developer Experience
- **24 slash commands** — `/c4-plan`, `/c4-run`, `/c4-status`, `/c4-checkpoint`, `/c4-swarm`, `/c4-polish`, `/c4-finish`, `/c4-quick`, `/c4-submit`, `/c4-release`, `/c4-attach`, `/c4-reboot`, ...
- **37 specialized agents** — `code-reviewer`, `ml-engineer`, `security-auditor`, `debugger`, ...
- **Shell completion** — `cq completion bash/zsh/fish`; auto-installed by `install.sh`
- **Workflow gates** — Hook-based: `git commit` blocked until `/c4-polish` done; `/c4-finish` requires polish gate
- **Headless auth** — `cq auth login --device` (user_code) / `--link` (URL) for SSH/container environments
- **7 hooks** — Secret scanning, force-push prevention, auto-lint (Python/TypeScript)
- **Economic mode** — Model routing presets (standard / economic / ultra-economic / quality)

## MCP Tools (133)

| Category | Count | Examples |
|----------|-------|---------|
| Core | 12 | `c4_status`, `c4_add_todo`, `c4_claim`, `c4_report`, `c4_checkpoint` |
| Files & Git | 11 | `c4_find_file`, `c4_search_for_pattern`, `c4_read_file`, `c4_search_commits` |
| Discovery | 12 | `c4_save_spec`, `c4_save_design`, `c4_artifact_save`, `c4_lighthouse` (TDD loop) |
| Code Intelligence | 7 | `c4_find_symbol`, `c4_get_symbols_overview`, `c4_replace_symbol_body` |
| Knowledge | 12 | `c4_knowledge_search`, `c4_knowledge_discover`, `c4_knowledge_ingest`, `c4_knowledge_stats` |
| Research | 5 | `c4_research_start`, `c4_research_next`, `c4_research_record` |
| Soul & Team | 10 | `c4_soul_resolve`, `c4_persona_evolve`, `c4_reflect`, `c4_whoami` |
| LLM & CDP | 5 | `c4_llm_call`, `c4_llm_providers`, `c4_cdp_run` |
| C1 & C2 | 11 | `c1_search`, `c4_parse_document`, `c4_workspace_create`, `c4_persona_learn` |
| Drive | 6 | `c4_drive_upload`, `c4_drive_download`, `c4_drive_list` |
| EventBus | 6 | `c4_event_publish`, `c4_rule_add`, `c4_rule_list`, `c4_rule_toggle` |
| Lighthouse | 6 | `c4_lighthouse` (register/list/get/promote/update/remove) |
| Hub | 26 | `c4_hub_submit`, `c4_hub_dag_create`, `c4_hub_edge_register`, `c4_hub_deploy` |

## Codebase

| Language | Source | Tests | Total |
|----------|--------|-------|-------|
| Go (`c4-core/`) | ~38.9K | ~36.8K | ~75.7K |
| Go (`c5/`) | ~6.9K | ~4.8K | ~11.7K |
| Python (`c4/`) | ~22.9K | ~9.5K | ~32.4K |
| Rust (`c1/src-tauri/`) | ~9.5K | (built-in) | ~9.5K |
| TS+CSS (`c1/src/`) | ~11.8K | — | ~11.8K |
| SQL (`infra/`) | ~1.1K | — | ~1.1K |
| **Total** | **~90.9K** | **~50.8K** | **~179K LOC** |

**Tests:** ~3,628 total

## Configuration

### Project Config (`.c4/config.yaml`)

```yaml
project_id: my-project
default_branch: main
review_as_task: true            # Auto-generate R-tasks for each T-task
checkpoint_as_task: true
max_revision: 3                 # Max REQUEST_CHANGES before blocking

economic_mode:
  preset: standard              # standard | economic | ultra-economic | quality

validation:
  lint: "uv run ruff check ."
  unit: "uv run pytest tests/ -v"

worktree:
  enabled: true                 # Isolated git worktrees per worker
```

### Economic Mode Presets

| Preset | Implementation | Review | Checkpoint | Scout |
|--------|:-:|:-:|:-:|:-:|
| **standard** | Sonnet | Opus | Opus | Haiku |
| **economic** | Sonnet | Sonnet | Sonnet | Haiku |
| **ultra-economic** | Haiku | Sonnet | Sonnet | Haiku |
| **quality** | Opus | Opus | Opus | Sonnet |

## C1 Desktop App

Multi-LLM project explorer with 6 views: Sessions, Dashboard, Config, Documents, Channels, Events.

Integrates with Claude Code, Codex CLI, Cursor, and Gemini CLI.

```bash
cd c1 && pnpm install && pnpm tauri dev
```

See [c1/README.md](c1/README.md) for details.

## Development

```bash
# Go MCP Server
cd c4-core && go build ./... && go test -p 1 ./...

# C5 Hub Server
cd c5 && go build ./... && go test ./...

# Python Sidecar
uv run pytest tests/

# C1 Desktop
cd c1 && pnpm test
cd c1/src-tauri && cargo test

# Environment diagnosis
cq doctor              # 8-item health check (binary, .c4/, .mcp.json, hooks, hub, ...)
cq doctor --json       # Machine-readable output for CI
cq doctor --fix        # Auto-fix simple issues (broken symlinks, etc.)
```

## Documentation

| Guide | Description |
|-------|-------------|
| [Usage Guide](docs/usage-guide.md) | When to use what — decision tree & workflows |
| [Installation](docs/getting-started/설치-가이드.md) | Step-by-step setup |
| [Quick Start](docs/getting-started/빠른-시작.md) | First project walkthrough |
| [Architecture](docs/developer-guide/아키텍처.md) | System design overview |
| [Workflow](docs/user-guide/워크플로우-개요.md) | Plan → Execute → Review lifecycle |
| [Commands](docs/user-guide/명령어-레퍼런스.md) | Slash command reference |
| [Smart Auto Mode](docs/user-guide/Smart-Auto-Mode.md) | Automatic execution mode |
| [LLM Config](docs/user-guide/LLM-설정.md) | Model routing & economic mode |
| [Cursor Guide](docs/user-guide/Cursor-가이드.md) | Use CQ MCP in Cursor IDE |
| [Troubleshooting](docs/user-guide/문제-해결.md) | Common issues & fixes |
| [Roadmap](docs/ROADMAP.md) | Future plans |

## License

Personal Study & Research License (Non-Commercial). See [LICENSE.md](./LICENSE.md).

Copyright (c) 2026 PlayIdeaLab. All rights reserved.

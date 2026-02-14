<div align="center">

# C4

**AI-Powered Project Orchestration System**

Plan, execute, review, and learn — automated end-to-end.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![Python](https://img.shields.io/badge/Python-3.11+-3776AB?logo=python&logoColor=white)
![Tools](https://img.shields.io/badge/MCP_Tools-103-blueviolet)
![Tests](https://img.shields.io/badge/Tests-1%2C100+-brightgreen)
![License](https://img.shields.io/badge/License-Personal_Study-orange)

</div>

---

C4 turns Claude Code into a full project management system. It provides **103 MCP tools**, a structured workflow engine, multi-lens code review, knowledge persistence, and GPU-aware task scheduling — all through natural language.

```
You: /c4-plan "Add user authentication with JWT"
C4:  Creates 5 tasks with DoD, spawns workers, reviews each PR, learns from decisions.
```

## How It Works

```
INIT ─▶ DISCOVERY ─▶ DESIGN ─▶ PLAN ─▶ EXECUTE ⇄ CHECKPOINT ─▶ COMPLETE
                                         │              │
                                    Worker mode     Multi-lens
                                    Direct mode     code review
```

C4 breaks features into tasks, assigns them to workers (parallel) or claims them directly (sequential), auto-generates review tasks, and accumulates decisions as organizational knowledge.

## Architecture

```
Claude Code ──stdio──▶ Go MCP Server (103 tools)
                        │
                        ├── Go Native ──────── State, Tasks, Files, Git, Validation
                        ├── SQLite Store ───── Specs, Designs, Checkpoints, Artifacts
                        ├── Soul Engine ────── Persona evolution, Digital Twin, Reflection
                        ├── LLM Gateway ────── Claude / GPT / Gemini / Ollama
                        ├── CDP Runner ─────── Browser automation (DevTools Protocol)
                        ├── Hub Client ─HTTP─▶ Daemon Scheduler
                        │                      ├── Process Manager (max-jobs, GPU alloc)
                        │                      ├── DAG Orchestration
                        │                      └── Edge Deployment
                        │
                        └── JSON-RPC ──TCP──▶ Python Sidecar
                                              ├── Code Intelligence (Multilspy/Jedi/Tree-sitter)
                                              ├── Knowledge Store (FTS5 + Vector, RRF)
                                              ├── C2 Document Lifecycle
                                              └── Research Loop
```

| Component | Directory | Stack |
|-----------|-----------|-------|
| Go MCP Server | `c4-core/` | Go, SQLite, Cobra CLI |
| Daemon Scheduler | `c4-core/internal/daemon/` | Go, REST API, GPU monitoring |
| Python Sidecar | `c4/` | Python, multilspy, sqlite-vec |
| C1 Desktop App | `c1/` | Tauri 2.x, React, Rust |

## Quick Start

**Prerequisites:** Go 1.22+, Python 3.11+, [uv](https://docs.astral.sh/uv/)

```bash
git clone https://git.pilab.co.kr/pi/c4.git && cd c4
./install.sh        # Builds Go binary, installs Python deps, configures .mcp.json
```

Restart Claude Code, then:

```bash
/c4-status          # Verify connection (103 tools registered)
/c4-plan "feature"  # Start planning
/c4-run             # Execute tasks
```

## Features

### Task Orchestration
- **Dual execution mode** — Worker (parallel, isolated worktrees) or Direct (sequential, shared workspace)
- **Review cascade** — Every task `T-001` auto-generates review `R-001`; rejections create `T-001-1 → R-001-1`
- **Checkpoint system** — APPROVE / REQUEST_CHANGES / REPLAN / REDESIGN decision points
- **Smart Auto Mode** — Automatically picks worker vs direct based on task dependencies

### Code Intelligence
- **3-tier LSP fallback** — Multilspy → Jedi → Tree-sitter for symbol resolution
- **Symbol operations** — Find, rename, replace body, insert before/after across the project
- **Multi-lens review** — Security, performance, architecture, testing perspectives per review

### Knowledge & Learning
- **Persistent knowledge store** — Experiments, patterns, insights, hypotheses with hybrid search (FTS5 + Vector)
- **Soul system** — Per-user judgment profiles that evolve from task outcomes
- **Digital Twin** — `c4_reflect` for pattern analysis, growth tracking, challenge identification
- **Persona evolution** — 27 role-specific personas with automatic behavioral learning

### Infrastructure
- **LLM Gateway** — Route to Claude, GPT, Gemini, or Ollama with cost tracking
- **Daemon Scheduler** — Local job queue with GPU allocation, duration estimation, and retry
- **DAG Orchestration** — Multi-step pipelines with dependency resolution
- **Edge Deployment** — Push artifacts to edge devices with auto-trigger rules
- **CDP Runner** — Browser automation via Chrome DevTools Protocol

### Developer Experience
- **14 slash commands** — `/c4-plan`, `/c4-run`, `/c4-status`, `/c4-checkpoint`, `/c4-swarm`, ...
- **37 specialized agents** — `code-reviewer`, `ml-engineer`, `security-auditor`, `debugger`, ...
- **7 hooks** — Secret scanning, force-push prevention, auto-lint (Python/TypeScript)
- **Economic mode** — Model routing presets (standard / economic / ultra-economic / quality)

## MCP Tools (103)

| Category | Count | Examples |
|----------|-------|---------|
| Core | 12 | `c4_status`, `c4_add_todo`, `c4_claim`, `c4_report`, `c4_checkpoint` |
| Files & Git | 11 | `c4_find_file`, `c4_search_for_pattern`, `c4_read_file`, `c4_search_commits` |
| Discovery | 12 | `c4_save_spec`, `c4_save_design`, `c4_artifact_save`, `c4_lighthouse` |
| Code Intelligence | 7 | `c4_find_symbol`, `c4_get_symbols_overview`, `c4_replace_symbol_body` |
| Knowledge | 12 | `c4_knowledge_search`, `c4_experiment_record`, `c4_research_start` |
| Soul & Team | 10 | `c4_soul_resolve`, `c4_persona_evolve`, `c4_reflect`, `c4_whoami` |
| LLM & CDP | 5 | `c4_llm_call`, `c4_llm_providers`, `c4_cdp_run` |
| C2 Documents | 8 | `c4_parse_document`, `c4_workspace_create`, `c4_persona_learn` |
| Hub | 26 | `c4_hub_submit`, `c4_hub_dag_create`, `c4_hub_edge_register`, `c4_hub_deploy` |

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

Multi-LLM project explorer with 4 views: Sessions, Dashboard, Config, Team.

Integrates with Claude Code, Codex CLI, Cursor, and Gemini CLI.

```bash
cd c1 && pnpm install && pnpm tauri dev
```

See [c1/README.md](c1/README.md) for details.

## Development

```bash
# Go MCP Server
cd c4-core && go build ./... && go test ./...

# Python Sidecar
uv run pytest tests/

# C1 Desktop
cd c1 && pnpm test
cd c1/src-tauri && cargo test
```

## Documentation

| Guide | Description |
|-------|-------------|
| [Installation](docs/getting-started/설치-가이드.md) | Step-by-step setup |
| [Quick Start](docs/getting-started/빠른-시작.md) | First project walkthrough |
| [Architecture](docs/developer-guide/아키텍처.md) | System design overview |
| [Workflow](docs/user-guide/워크플로우-개요.md) | Plan → Execute → Review lifecycle |
| [Commands](docs/user-guide/명령어-레퍼런스.md) | Slash command reference |
| [Smart Auto Mode](docs/user-guide/Smart-Auto-Mode.md) | Automatic execution mode |
| [LLM Config](docs/user-guide/LLM-설정.md) | Model routing & economic mode |
| [Troubleshooting](docs/user-guide/문제-해결.md) | Common issues & fixes |
| [Roadmap](docs/ROADMAP.md) | Future plans |

## License

Personal Study & Research License (Non-Commercial). See [LICENSE.md](./LICENSE.md).

Copyright (c) 2026 PlayIdeaLab. All rights reserved.

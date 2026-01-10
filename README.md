# C4 - AI Project Orchestration System

C4 (Codex-Claude-Completion Control) is an AI project orchestration system that enables AI agents to execute projects from planning through completion with minimal human intervention.

## Key Features

- **State Machine**: Structured workflow INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ↔ CHECKPOINT → COMPLETE
- **MCP Server**: Native integration with Claude Code via Model Context Protocol (19 tools)
- **Multi-Worker**: Parallel task execution with SQLite WAL mode (race-condition free, 30-min stale recovery)
- **Agent Routing**: Domain-based agent selection with chaining (Phase 4)
- **EARS Requirements**: 5-pattern requirements gathering (Ubiquitous, State/Event-driven, Optional, Unwanted)
- **ADR (Architecture Decision Records)**: Structured design decision management
- **Verification System**: 6 verifiers (HTTP, CLI, Browser, Visual, Metrics, Dryrun)
- **Checkpoint Gates**: Human/supervisor review points between phases
- **Auto-Validation**: Built-in lint and test runners
- **Pluggable Architecture**: Extensible StateStore and SupervisorBackend
- **Stop Hook**: Prevents Claude exit while tasks remain (continuous execution)
- **Auto Supervisor**: Headless supervisor loop starts automatically on execution
- **SQLite Storage**: Default storage with automatic migration from legacy JSON format
- **Default Checkpoints**: Auto-generated CP-REVIEW and CP-FINAL on init
- **Review Parser**: Automatic review-report.md → task conversion

## Documentation

### Getting Started

| 문서 | 설명 |
|------|------|
| [설치 가이드](docs/getting-started/설치-가이드.md) | 설치 및 Claude Code 설정 |
| [빠른 시작](docs/getting-started/빠른-시작.md) | 5분 퀵스타트 가이드 |
| [예제: C4 셀프호스팅](docs/getting-started/예제-C4-셀프호스팅.md) | C4로 C4 개발하기 튜토리얼 |

### User Guide

| 문서 | 설명 |
|------|------|
| [워크플로우 개요](docs/user-guide/워크플로우-개요.md) | Plan → Execute → Checkpoint 흐름 |
| [명령어 레퍼런스](docs/user-guide/명령어-레퍼런스.md) | 슬래시 명령어 상세 |
| [문제 해결](docs/user-guide/문제-해결.md) | FAQ 및 트러블슈팅 |

### Developer Guide

| 문서 | 설명 |
|------|------|
| [아키텍처](docs/developer-guide/아키텍처.md) | 시스템 구조 및 컴포넌트 |
| [StateStore 확장](docs/developer-guide/StateStore-확장.md) | 커스텀 저장소 구현 (Redis, Supabase 등) |
| [SupervisorBackend 확장](docs/developer-guide/SupervisorBackend-확장.md) | 다른 LLM 연동 (OpenAI, Copilot 등) |
| [커스텀 Validator](docs/developer-guide/커스텀-Validator.md) | 검증 명령 추가 |

### API Reference

| 문서 | 설명 |
|------|------|
| [MCP 도구 레퍼런스](docs/api/MCP-도구-레퍼런스.md) | 19개 MCP 도구 상세 스펙 |

---

## Quick Start

### 1. Installation (One-liner)

```bash
rm -rf ~/.c4 && git clone https://git.pilab.co.kr/pi/c4.git ~/.c4 && ~/.c4/install.sh
```

설치 완료! 스크립트가 자동으로:
- C4를 `~/.c4`에 클론
- 의존성 설치 (`uv sync`)
- 글로벌 `c4` 명령어 생성 (`~/.local/bin/c4`)
- 슬래시 명령어 복사 (`~/.claude/commands/`)
- Hooks 설정 (Stop Hook, Security Hook)

### 2. Start a Project

```bash
cd /path/to/your/project
c4
```

이 명령어 하나로:
- C4 자동 초기화
- MCP 서버와 함께 Claude Code 시작

### 3. Start Working

Claude Code에서:

```
/c4-plan       # 문서 분석 → 태스크 생성 (대화형)
/c4-run        # 자동 실행 시작
/c4-status     # 진행 상황 확인
```

<details>
<summary>설치 옵션</summary>

```bash
# 신규 설치
git clone https://git.pilab.co.kr/pi/c4.git ~/.c4 && ~/.c4/install.sh

# 재설치 (기존 삭제 후)
rm -rf ~/.c4 && git clone https://git.pilab.co.kr/pi/c4.git ~/.c4 && ~/.c4/install.sh

# 업데이트만 (git pull)
cd ~/.c4 && git pull && ./install.sh

# 경로 지정 설치
git clone https://git.pilab.co.kr/pi/c4.git ~/tools/c4 && ~/tools/c4/install.sh
```

</details>

<details>
<summary>Claude Code 내에서 초기화</summary>

이미 Claude Code 세션에 있다면:

```
/c4-init
```

Note: MCP 서버 로드를 위해 Claude Code 재시작 필요.

</details>

---

## Claude Code Slash Commands

| Command | Description |
|---------|-------------|
| `/c4-init` | Initialize C4 in current directory (includes Stop Hook setup) |
| `/c4-status` | Show project status and queue |
| `/c4-plan` | Scan docs, interview preferences, generate tasks |
| `/c4-run` | Start worker loop (PLAN→EXECUTE or join existing) |
| `/c4-stop` | Halt execution |
| `/c4-validate` | Run validations (lint, unit) |
| `/c4-submit` | Submit completed task |
| `/c4-checkpoint` | Handle checkpoint review |
| `/c4-add-task` | Add new task to queue |

---

## MCP Tools

| Tool | Description |
|------|-------------|
| `c4_status` | Get project status, queue, workers |
| `c4_start` | Transition to EXECUTE and auto-start supervisor loop |
| `c4_get_task` | Get next task assignment |
| `c4_submit` | Submit completed task |
| `c4_run_validation` | Run validations |
| `c4_checkpoint` | Record supervisor decision |
| `c4_add_todo` | Add new task |
| `c4_mark_blocked` | Mark task as blocked |
| `c4_clear` | Reset C4 state (delete .c4 directory) |

---

## Workflow

```text
┌──────┐   ┌───────────┐   ┌────────┐   ┌──────┐   ┌─────────┐   ┌────────────┐   ┌──────────┐
│ INIT │──▶│ DISCOVERY │──▶│ DESIGN │──▶│ PLAN │──▶│ EXECUTE │──▶│ CHECKPOINT │──▶│ COMPLETE │
└──────┘   └───────────┘   └────────┘   └──────┘   └─────────┘   └────────────┘   └──────────┘
                                            │           │               │
                                            │           ▼               │
                                            │      ┌─────────┐          │
                                            └──────│ HALTED  │◀─────────┘
                                                   └─────────┘
```

### States

| State | Description |
|-------|-------------|
| **INIT** | Project created, awaiting initialization |
| **DISCOVERY** | Domain detection + EARS requirements gathering |
| **DESIGN** | Architecture design and decisions |
| **PLAN** | Task creation and planning |
| **EXECUTE** | Workers processing tasks (with Agent Routing) |
| **CHECKPOINT** | Awaiting supervisor review |
| **HALTED** | Execution paused |
| **COMPLETE** | All tasks done, project finished |

### Checkpoint Decisions

| Decision | Effect |
|----------|--------|
| `APPROVE` | Proceed to next phase or complete |
| `REQUEST_CHANGES` | Create fix tasks, continue execution |
| `REPLAN` | Return to planning phase |
| `REDESIGN` | Return to design phase |

### Agent Routing (Phase 4)

When a worker requests a task via `c4_get_task()`, the system automatically provides agent routing information:

| Domain | Primary Agent | Chain |
|--------|--------------|-------|
| `web-frontend` | frontend-developer | frontend → test → reviewer |
| `web-backend` | backend-architect | architect → python → test → reviewer |
| `fullstack` | backend-architect | backend → frontend → test → reviewer |
| `ml-dl` | ml-engineer | ml → python → test |
| `mobile-app` | mobile-developer | mobile → test → reviewer |
| `infra` | cloud-architect | cloud → deployment |
| `library` | python-pro | python → docs → test → reviewer |
| `unknown` | general-purpose | general → reviewer |

**Response includes:**
```json
{
  "task_id": "T-001",
  "recommended_agent": "frontend-developer",
  "agent_chain": ["frontend-developer", "test-automator", "code-reviewer"],
  "domain": "web-frontend",
  "handoff_instructions": "Pass component specs and test requirements..."
}
```

---

## Architecture

```text
┌─────────────────────────────────────────────────────────────────┐
│                         MCP Server                               │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                       C4Daemon                             │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐    │  │
│  │  │ StateMachine │  │ WorkerManager│  │ SQLiteLockStore│   │  │
│  │  └──────┬───────┘  └──────────────┘  └───────────────┘    │  │
│  │         │                                                  │  │
│  │         v          ┌───────────────────────────────────┐  │  │
│  │  ┌──────────────┐  │         AgentRouter (Phase 4)     │  │  │
│  │  │  StateStore  │  │  ┌─────────────────────────────┐  │  │  │
│  │  │  (Protocol)  │  │  │ Domain → Agent Mapping      │  │  │  │
│  │  └──────┬───────┘  │  │ web-frontend → frontend-dev │  │  │  │
│  │         │          │  │ web-backend → backend-arch  │  │  │  │
│  │  ┌──────┴──────┐   │  │ ml-dl → ml-engineer         │  │  │  │
│  │  │ SQLite(기본)│   │  │ + Agent Chaining            │  │  │  │
│  │  │ (Extensible)│   │  └─────────────────────────────┘  │  │  │
│  │  └─────────────┘   └───────────────────────────────────┘  │  │
│  │                                                            │  │
│  │  ┌────────────────────────┐                               │  │
│  │  │   SupervisorBackend    │                               │  │
│  │  │   ClaudeCLI / Mock     │                               │  │
│  │  └────────────────────────┘                               │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
              │                               │
   ┌──────────┴──────────┐         ┌──────────┴──────────┐
   │   Worker Agents     │         │     Supervisor      │
   │   (Claude Code)     │         │   (Human/Claude)    │
   │   + Agent Routing   │         └─────────────────────┘
   └─────────────────────┘
```

### Pluggable Components

**StateStore**: 상태 저장소 백엔드
- `SQLiteStateStore`: SQLite 데이터베이스 (기본, WAL 모드로 멀티워커 동시성 지원)
- `LocalFileStateStore`: 파일 기반 (레거시)
- 확장: Redis, Supabase, PostgreSQL 등

**SupervisorBackend**: Supervisor 리뷰 백엔드
- `ClaudeCliBackend`: Claude CLI 사용 (기본)
- `MockBackend`: 테스트용
- 확장: OpenAI, GitHub Copilot, Human Review 등

---

## Configuration

Edit `.c4/config.yaml`:

```yaml
project_id: my-project
default_branch: main
work_branch_prefix: "c4/w-"

validations:
  lint:
    command: "uv run ruff check"
    description: "Code style check"
  unit:
    command: "uv run pytest tests/unit"
    description: "Unit tests"
  integration:
    command: "uv run pytest tests/integration"
    description: "Integration tests"

checkpoints:
  - id: CP-001
    name: "Phase 1 Review"
    required_tasks: ["T-001", "T-002"]
    required_validations: ["lint", "unit"]
```

### Default Checkpoints

`c4 init` 시 기본 체크포인트가 자동 생성됩니다:

| ID | 설명 | 필수 Validation |
|----|------|----------------|
| `CP-REVIEW` | 코드 리뷰 완료 후 Supervisor 검토 | lint |
| `CP-FINAL` | 모든 작업 완료 후 최종 검토 | lint, unit |

기본 체크포인트 없이 시작하려면 `with_default_checkpoints=False` 옵션 사용.

---

## Development

```bash
# Run tests
uv run pytest tests/ -v

# Run linter
uv run ruff check c4/ tests/

# Type check
uv run mypy c4/

# Run specific test category
uv run pytest tests/unit -v
uv run pytest tests/integration -v
```

## Project Structure

```text
c4/
├── c4/                    # Main package
│   ├── mcp_server.py      # MCP server (C4Daemon)
│   ├── state_machine.py   # State transitions
│   ├── models/            # Pydantic schemas
│   │   ├── task.py        # Task with domain field
│   │   └── responses.py   # TaskAssignment with agent routing
│   ├── store/             # StateStore implementations (SQLite default)
│   ├── supervisor/        # SupervisorBackend + AgentRouter
│   │   ├── agent_router.py    # Domain → Agent mapping (Phase 4)
│   │   ├── claude_backend.py  # Claude CLI backend
│   │   └── prompt.py          # Prompt renderer
│   ├── daemon/            # Manager classes
│   │   ├── workers.py     # WorkerManager (stale recovery)
│   │   └── supervisor_loop.py # Checkpoint/repair processing
│   └── validation.py      # Validation runner
├── tests/
│   ├── unit/              # Unit tests (incl. test_agent_router.py)
│   ├── integration/       # Integration tests (incl. test_agent_routing.py)
│   └── e2e/               # End-to-end tests
├── docs/                  # Documentation (한국어)
│   ├── getting-started/   # 시작 가이드
│   ├── user-guide/        # 사용자 가이드
│   ├── developer-guide/   # 개발자 가이드
│   └── api/               # API 레퍼런스
└── .claude/commands/      # Slash commands (10 commands)

# Per-project storage (.c4/ directory)
your-project/
└── .c4/
    ├── c4.db              # SQLite database (state, locks, tasks)
    ├── config.yaml        # Project configuration
    └── events/            # Event logs
```

---

## License

**Business Source License 1.1** (BSL)

- **Free for**: Personal use, evaluation, non-commercial projects
- **Requires license for**: Commercial use, production deployment

See [LICENSE](./LICENSE) for full terms.

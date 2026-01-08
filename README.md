# C4 - AI Project Orchestration System

C4 (Codex-Claude-Completion Control) is an AI project orchestration system that enables AI agents to execute projects from planning through completion with minimal human intervention.

## Key Features

- **State Machine**: Structured workflow INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE
- **MCP Server**: Native integration with Claude Code via Model Context Protocol
- **Multi-Worker**: Parallel task execution with scope-based locking
- **Checkpoint Gates**: Human/supervisor review points between phases
- **Auto-Validation**: Built-in lint and test runners
- **Pluggable Architecture**: Extensible StateStore and SupervisorBackend

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
| [MCP 도구 레퍼런스](docs/api/MCP-도구-레퍼런스.md) | 7개 MCP 도구 상세 스펙 |

---

## Quick Start

### 1. Installation (One-liner)

```bash
curl -LsSf https://git.pilab.co.kr/pi/c4/-/raw/main/install-remote.sh | sh
```

That's it! The script will:
- Install dependencies (`uv sync`)
- Copy slash commands to `~/.claude/commands/`
- Configure MCP server in `~/.claude.json`

### 2. Restart Claude Code

Close and reopen Claude Code to load the new configuration.

### 3. Initialize Your Project

```bash
cd /path/to/your/project
claude
```

In Claude Code:
```
/c4-init
```

### 4. Start Working

```
/c4-plan       # Interpret docs and create tasks
/c4-run        # Start automated execution
/c4-status     # Check progress anytime
```

<details>
<summary>Alternative: Clone & Install</summary>

```bash
git clone https://git.pilab.co.kr/pi/c4.git
cd c4
./install.sh
```

</details>

<details>
<summary>Alternative: Manual Setup</summary>

```bash
# 1. Clone and install dependencies
git clone https://git.pilab.co.kr/pi/c4.git
cd c4
uv sync

# 2. Copy commands
cp .claude/commands/c4-*.md ~/.claude/commands/

# 3. Add to ~/.claude.json mcpServers:
"c4": {
  "command": "uv",
  "args": ["--directory", "/path/to/c4", "run", "python", "-m", "c4.mcp_server"]
}
```

</details>

---

## Claude Code Slash Commands

| Command | Description |
|---------|-------------|
| `/c4-init` | Initialize C4 in current directory |
| `/c4-status` | Show project status and queue |
| `/c4-plan` | Enter planning mode |
| `/c4-run` | Start execution phase |
| `/c4-stop` | Halt execution |
| `/c4-worker` | Get task assignment |
| `/c4-validate` | Run validations (lint, unit) |
| `/c4-submit` | Submit completed task |
| `/c4-checkpoint` | Handle checkpoint review |
| `/c4-add-task` | Add new task to queue |

---

## MCP Tools

| Tool | Description |
|------|-------------|
| `c4_status` | Get project status, queue, workers |
| `c4_get_task` | Get next task assignment |
| `c4_submit` | Submit completed task |
| `c4_run_validation` | Run validations |
| `c4_checkpoint` | Record supervisor decision |
| `c4_add_todo` | Add new task |
| `c4_mark_blocked` | Mark task as blocked |

---

## Workflow

```text
┌─────────┐    ┌─────────┐    ┌──────────┐    ┌────────────┐    ┌──────────┐
│  INIT   │───▶│  PLAN   │───▶│ EXECUTE  │───▶│ CHECKPOINT │───▶│ COMPLETE │
└─────────┘    └─────────┘    └──────────┘    └────────────┘    └──────────┘
                   │               │               │
                   │               ▼               │
                   │          ┌─────────┐          │
                   └──────────│ HALTED  │◀─────────┘
                              └─────────┘
```

### States

| State | Description |
|-------|-------------|
| **INIT** | Project created, awaiting plan |
| **PLAN** | Planning tasks and checkpoints |
| **EXECUTE** | Workers processing tasks |
| **CHECKPOINT** | Awaiting supervisor review |
| **HALTED** | Execution paused |
| **COMPLETE** | All tasks done, project finished |

### Checkpoint Decisions

| Decision | Effect |
|----------|--------|
| `APPROVE` | Proceed to next phase or complete |
| `REQUEST_CHANGES` | Create fix tasks, continue execution |
| `REPLAN` | Return to planning phase |

---

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                       MCP Server                             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │                     C4Daemon                             ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐  ││
│  │  │ StateMachine │  │ TaskManager  │  │ LockManager   │  ││
│  │  └──────┬───────┘  └──────────────┘  └───────────────┘  ││
│  │         │                                                ││
│  │         v                                                ││
│  │  ┌──────────────┐         ┌────────────────────────┐    ││
│  │  │  StateStore  │◄───────▶│  SupervisorBackend    │    ││
│  │  │  (Protocol)  │         │     (Protocol)         │    ││
│  │  └──────┬───────┘         └────────┬───────────────┘    ││
│  │         │                          │                     ││
│  │  ┌──────┴──────┐           ┌───────┴───────┐            ││
│  │  │ LocalFile   │           │  ClaudeCLI    │            ││
│  │  │ SQLite      │           │  Mock         │            ││
│  │  │ (Extensible)│           │  (Extensible) │            ││
│  │  └─────────────┘           └───────────────┘            ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
              │                               │
   ┌──────────┴──────────┐         ┌──────────┴──────────┐
   │   Worker Agents     │         │     Supervisor      │
   │   (Claude Code)     │         │   (Human/Claude)    │
   └─────────────────────┘         └─────────────────────┘
```

### Pluggable Components

**StateStore**: 상태 저장소 백엔드
- `LocalFileStateStore`: 파일 기반 (기본)
- `SQLiteStateStore`: SQLite 데이터베이스
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
│   ├── store/             # StateStore implementations
│   ├── supervisor/        # SupervisorBackend implementations
│   ├── daemon/            # Manager classes
│   └── validation.py      # Validation runner
├── tests/
│   ├── unit/              # Unit tests
│   ├── integration/       # Integration tests
│   └── e2e/               # End-to-end tests
├── docs/                  # Documentation (한국어)
│   ├── getting-started/   # 시작 가이드
│   ├── user-guide/        # 사용자 가이드
│   ├── developer-guide/   # 개발자 가이드
│   └── api/               # API 레퍼런스
└── .claude/commands/      # Slash commands
```

---

## License

**Business Source License 1.1** (BSL)

- **Free for**: Personal use, evaluation, non-commercial projects
- **Requires license for**: Commercial use, production deployment

See [LICENSE](./LICENSE) for full terms.

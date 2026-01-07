# C4 - AI Project Orchestration System

C4 (Codex-Claude-Completion Control) is an AI project orchestration system that enables AI agents to execute projects from planning through completion with minimal human intervention.

## Key Features

- **State Machine**: Structured workflow INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE
- **MCP Server**: Native integration with Claude Code via Model Context Protocol
- **Multi-Worker**: Parallel task execution with scope-based locking
- **Checkpoint Gates**: Human/supervisor review points between phases
- **Auto-Validation**: Built-in lint and test runners

## Installation

```bash
git clone https://github.com/your-org/c4.git
cd c4
uv sync
```

## Quick Start

### 1. Initialize Project

```bash
uv run c4 init --project-id "my-project"
```

Creates `.c4/` directory with config and state files.

### 2. Configure MCP Server

Add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "c4": {
      "command": "uv",
      "args": ["run", "python", "-m", "c4.mcp_server"],
      "cwd": "/path/to/your/project"
    }
  }
}
```

### 3. Start Working

In Claude Code, use the slash commands:

```bash
/c4-status     # Check project state
/c4-worker     # Get a task and start working
/c4-validate   # Run lint/test validations
/c4-submit     # Submit completed work
```

## Claude Code Slash Commands

C4 provides slash commands for seamless Claude Code integration:

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

## MCP Tools

When connected via MCP, these tools are available:

| Tool | Description |
|------|-------------|
| `c4_status()` | Get project status, queue, workers |
| `c4_get_task(worker_id)` | Get next task assignment |
| `c4_submit(task_id, commit_sha, results)` | Submit completed task |
| `c4_run_validation(names, timeout)` | Run validations |
| `c4_checkpoint(id, decision, notes)` | Record supervisor decision |
| `c4_add_todo(task_id, title, dod)` | Add new task |

## Workflow

```text
┌─────────┐    ┌─────────┐    ┌──────────┐    ┌────────────┐    ┌──────────┐
│  INIT   │───▶│  PLAN   │───▶│ EXECUTE  │───▶│ CHECKPOINT │───▶│ COMPLETE │
└─────────┘    └─────────┘    └──────────┘    └────────────┘    └──────────┘
                   │               │               │
                   │               ▼               │
                   │          ┌─────────┐          │
                   └──────────│ REPLAN  │◀─────────┘
                              └─────────┘
```

### States

| State | Description |
|-------|-------------|
| **INIT** | Project created, awaiting plan |
| **PLAN** | Planning tasks and checkpoints |
| **EXECUTE** | Workers processing tasks |
| **CHECKPOINT** | Awaiting supervisor review |
| **COMPLETE** | All tasks done, project finished |

### Checkpoint Decisions

| Decision | Effect |
|----------|--------|
| `APPROVE` | Proceed to next phase or complete |
| `REQUEST_CHANGES` | Create fix tasks, continue execution |
| `REPLAN` | Return to planning phase |

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                     C4 MCP Server                           │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ State Machine │ Event Log │ Task Queue │ Validations   │ │
│  │     (.c4/)    │  events/  │ state.json │   runner      │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
   ┌─────────────────────┐         ┌─────────────────────┐
   │   Worker Agents     │         │     Supervisor      │
   │   (Claude Code)     │         │   (Human/Claude)    │
   │                     │         │                     │
   │ • /c4-worker        │         │ • /c4-checkpoint    │
   │ • /c4-submit        │         │ • Review bundles    │
   │ • /c4-validate      │         │ • APPROVE/REJECT    │
   └─────────────────────┘         └─────────────────────┘
```

## Configuration

Edit `.c4/config.yaml`:

```yaml
project_id: my-project
default_branch: main
work_branch_prefix: "c4/w-"

validation:
  commands:
    lint: uv run ruff check src/
    unit: uv run pytest tests/ -v
  required:
    - lint
    - unit

checkpoints:
  - id: CP1
    name: "Phase 1 Review"
    required_tasks: ["T-001", "T-002"]
    required_validations: ["lint", "unit"]
```

## Example Session

```bash
# 1. Initialize
$ uv run c4 init --project-id "feature-auth"

# 2. In Claude Code with MCP connected:
> /c4-status
Project: feature-auth
Status: PLAN
Queue: 0 pending, 0 in_progress, 0 done

> /c4-add-task T-001 "Implement login API"
Added task T-001

> /c4-run
Status changed: PLAN → EXECUTE

> /c4-worker
Assigned: T-001 "Implement login API"
Branch: c4/w-T-001
DoD: Login endpoint with JWT

# ... implement feature ...

> /c4-validate
Running: lint, unit
Results: lint=pass, unit=pass

> /c4-submit
Submitted T-001 (commit: abc123)
Checkpoint CP1 reached - awaiting review

> /c4-checkpoint APPROVE "Code looks good"
CP1 approved - project COMPLETE
```

## Development

```bash
# Run tests
uv run pytest tests/ -v

# Run linter
uv run ruff check c4/ tests/

# Type check
uv run mypy c4/
```

## Project Structure

```text
c4/
├── c4/                    # Main package
│   ├── mcp_server.py      # MCP server (C4Daemon)
│   ├── state_machine.py   # State transitions
│   ├── models/            # Pydantic schemas
│   ├── daemon/            # Manager classes
│   └── bundle.py          # Checkpoint bundles
├── tests/
│   ├── unit/              # Unit tests
│   ├── integration/       # Integration tests
│   └── e2e/               # End-to-end tests
└── .claude/commands/      # Slash commands
```

## License

**Business Source License 1.1** (BSL)

- **Free for**: Personal use, evaluation, non-commercial projects
- **Requires license for**: Commercial use, production deployment

See [LICENSE](./LICENSE) for full terms.

# C4D - AI Project Orchestration Daemon

C4 (Codex-Claude-Completion Control) is an AI project orchestration system that enables AI agents to execute projects from planning through completion without interruption.

## Features

- **State Machine**: INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE
- **MCP Server**: Model Context Protocol integration for Claude Code
- **Multi-Worker Support**: Parallel task execution with scope locking
- **Checkpoint System**: Supervisor review gates between phases
- **Validation Runner**: Automated lint and test execution

## Installation

```bash
# Clone the repository
git clone https://github.com/your-org/c4.git
cd c4

# Install dependencies with uv
uv sync
```

## Quick Start

### 1. Initialize a project

```bash
uv run c4 init --project-id "my-project"
```

This creates:

- `.c4/` - C4 data directory
- `docs/` - Plan documents (PLAN.md, CHECKPOINTS.md, DONE.md)

### 2. Create your task list

Edit `todo.md` with your tasks:

```markdown
### T-001: Implement feature X
- **Scope**: src/feature
- **DoD**: Feature X working with tests
- **Validations**: lint, unit
```

### 3. Add tasks to queue

```bash
uv run c4 add-task T-001 --title "Implement feature X" --dod "Feature X working" --scope "src/feature"
```

### 4. Start execution

```bash
uv run c4 run
```

### 5. Check status

```bash
uv run c4 status
```

## MCP Server Setup

Add to your `.mcp.json`:

```json
{
  "mcpServers": {
    "c4d": {
      "command": "uv",
      "args": ["run", "python", "-m", "c4d.mcp_server"]
    }
  }
}
```

## CLI Commands

### c4 (Project Management)

| Command | Description |
|---------|-------------|
| `c4 init` | Initialize C4 in current directory |
| `c4 status` | Show current project status |
| `c4 run` | Start execution (PLAN → EXECUTE) |
| `c4 stop` | Stop execution (→ HALTED) |
| `c4 plan` | Enter planning mode |
| `c4 add-task` | Add task to queue |
| `c4 worker join` | Join as worker |
| `c4 worker submit` | Submit completed task |

### c4d (Daemon Management)

| Command | Description |
|---------|-------------|
| `c4d start` | Start daemon (background) |
| `c4d stop` | Stop daemon |
| `c4d status` | Check daemon status |

## MCP Tools

When connected via MCP, the following tools are available:

| Tool | Description |
|------|-------------|
| `c4_status()` | Get project status |
| `c4_get_task(worker_id)` | Get next task assignment |
| `c4_submit(task_id, commit, results)` | Submit completed task |
| `c4_run_validation(names)` | Run lint/test validations |
| `c4_checkpoint(id, decision, notes)` | Record supervisor decision |
| `c4_add_todo(task_id, title, dod)` | Add new task |

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                    c4d (MCP Server)                          │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │ State Machine: INIT → PLAN → EXECUTE → CHECKPOINT → ... │ │
│  │ Event Log: .c4/events/                                   │ │
│  │ Task Queue: .c4/state.json                              │ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
   ┌─────────────────────┐         ┌─────────────────────┐
   │      Worker         │         │     Supervisor      │
   │   (Claude Code)     │         │  (Claude headless)  │
   │                     │         │                     │
   │ • MCP Client        │         │ • claude -p         │
   │ • Task execution    │         │ • JSON decision     │
   └─────────────────────┘         └─────────────────────┘
```

## Configuration

Edit `.c4/config.yaml`:

```yaml
project_id: my-project
default_branch: main

validation:
  commands:
    lint: uv run ruff check src/
    unit: uv run pytest tests/ -v
  required:
    - lint
    - unit

checkpoints:
  - id: CP1
    required_tasks: [T-001, T-002]
    required_validations: [lint, unit]
    auto_approve: false
```

## Documentation

- [docs/PLAN.md](./docs/PLAN.md) - Project plan
- [docs/CHECKPOINTS.md](./docs/CHECKPOINTS.md) - Checkpoint definitions
- [docs/DONE.md](./docs/DONE.md) - Completion criteria
- [docs/specs/](./docs/specs/) - Technical specifications

## Development

```bash
# Run tests
uv run pytest tests/ -v

# Run linter
uv run ruff check c4d/ tests/

# Run type checker
uv run mypy c4d/
```

---

## C4 Cloud (Coming Soon)

C4 Cloud is the hosted SaaS version of C4.

### Why Cloud?

| Feature | C4 Local (CLI) | C4 Cloud |
|---------|---------------|----------|
| Setup | `pip install c4d` + API key | Sign up only |
| Interface | Terminal | Web dashboard |
| Workers | Manual terminal tabs | Slider control |
| Results | Local files | Auto GitHub push/PR |
| Cost | Your API keys | Subscription |

See [docs/cloud/](./docs/cloud/) for detailed Cloud architecture and PRD.

---

## License

This project is licensed under the **Business Source License 1.1** (BSL).

- **Free for**: Personal use, evaluation, non-commercial projects
- **Requires license for**: Commercial use, production deployment in businesses

See [LICENSE](./LICENSE) for full terms.

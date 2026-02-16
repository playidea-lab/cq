# C4 Project Tools for Gemini

Gemini CLI should utilize these project-specific tools to interact with the C4/C5 ecosystem.

## Task Management (Go/Python Hybrid)
- `c4_status`: Displays the current task queue and system status. **Always check this first.**
- `c4_claim <task_id>`: Assigns a task to the current user/agent.
- `c4_get_task`: Fetches the next available task (Worker mode).
- `c4_submit <task_id>`: Submits a completed task for review.
- `c4_report <task_id>`: Reports completion in Direct mode.
- `c4_add_task "<description>"`: Creates a new task in the backlog.

## Validation & Quality
- `c4_validate`: Runs the comprehensive validation suite (tests, types, lint).
- `c4_review`: Triggers a review process.

## Development Helpers
- `c5`: The main Go-based CLI tool.
    - `c5 --help`: Show available commands.
- `./scripts/codex/`: Contains various helper scripts often used by agents.

## Standard Shell Tools
- `go test ./...`: Run Go tests.
- `pytest`: Run Python tests.
- `npm test`: Run Frontend tests.
- `uv sync`: Sync Python dependencies.
- `make`: Check `Makefile` for build targets.

## Usage Rule
Whenever an agent needs to perform an action (start task, finish task, check status), prefer using these `c4_*` commands over manual file manipulation of state files.

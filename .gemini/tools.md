# Gemini CLI Tool Handbook

This document defines the **correct usage patterns** for C4 project tools.
Gemini agents must consult this guide before executing commands to avoid syntax errors.

## 1. Task Management (`c4`)

### `c4 add-task` (Create new task)
**Syntax**: `c4 add-task --title "..." [flags]`
- **Required**: `--title "Task Title"`
- **Optional**:
    - `--scope "path/to/file"` (Highly recommended)
    - `--priority N` (1=High, 5=Normal)
    - `--dod "Definition of Done"` (Detailed requirements)
    - `--depends "T-123"` (Dependency ID)

**Example**:
```bash
c4 add-task --title "Refactor Auth" --scope "src/auth/" --priority 3 --dod "Implement JWT validation"
```

### `c4 status` (Check project state)
**Syntax**: `c4 status`
- Shows active tasks, workers, and overall progress.
- **Use this first** before starting any work.

### `c4 run` (Execute tasks)
**Syntax**: `c4 run [flags]`
- Starts the task execution loop (Worker mode).
- Usually run in a separate terminal or background process.

## 2. Validation & Quality (`.gemini/skills/`)

### `c4-validate.sh` (Run tests & checks)
**Syntax**: `./.gemini/skills/c4-validate.sh [mode]`
- **Modes**: `lint`, `unit`, `security`, `all` (default)
- **Example**: `./.gemini/skills/c4-validate.sh security`

### `c4-status.sh` (Quick status check)
**Syntax**: `./.gemini/skills/c4-status.sh`
- Lightweight alternative to `c4 status`.

## 3. Core Development (`c5` & Go)

### `c5` (New Core CLI)
**Location**: `.gemini/bin/c5`
- Note: Functionality is being migrated from `c4` to `c5`. Check `c5 --help` for available commands.

### Standard Build Commands
- **Go**: `go build ./...`, `go test ./...`
- **Frontend (c1)**: `npm install`, `npm run build`, `npm test`
- **Python**: `uv sync`, `pytest`

## 4. Best Practices
1.  **Always use flags**: Do not rely on positional arguments for `c4` commands.
2.  **Quote strings**: Always quote titles and descriptions to avoid shell parsing errors.
3.  **Check exit codes**: If a command fails, read the error message and check this handbook.
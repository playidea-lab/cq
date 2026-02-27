# Gemini CLI Tool Handbook

This document defines the **correct usage patterns** for CQ project tools.
Gemini agents must consult this guide before executing commands to avoid syntax errors.

## 1. Task Management (`c4`)

### `cq add-task` (Create new task)
**Syntax**: `cq add-task --title "..." [flags]`
- **Required**: `--title "Task Title"`
- **Optional**:
    - `--scope "path/to/file"` (Highly recommended)
    - `--priority N` (1=High, 5=Normal)
    - `--dod "Definition of Done"` (Detailed requirements)
    - `--depends "T-123"` (Dependency ID)

**Example**:
```bash
cq add-task --title "Refactor Auth" --scope "src/auth/" --priority 3 --dod "Implement JWT validation"
```

### `cq status` (Check project state)
**Syntax**: `cq status`
- Shows active tasks, workers, and overall progress.
- **Use this first** before starting any work.

### `cq run` (Execute tasks)
**Syntax**: `cq run [flags]`
- Starts the task execution loop (Worker mode).
- Usually run in a separate terminal or background process.

## 2. Validation & Quality (`.gemini/skills/`)

### `c4-validate.sh` (Run tests & checks)
**Syntax**: `./.gemini/skills/c4-validate.sh [mode]`
- **Modes**: `lint`, `unit`, `security`, `all` (default)
- **Example**: `./.gemini/skills/c4-validate.sh security`

### `c4-status.sh` (Quick status check)
**Syntax**: `./.gemini/skills/c4-status.sh`
- Lightweight alternative to `cq status`.

## 3. Core Development (`c5` & Go)

### `c5` (New Core CLI)
**Location**: `.gemini/bin/c5`
- Note: Functionality is being migrated from `c4` to `c5`. Check `c5 --help` for available commands.

### Standard Build Commands
- **Go**: `go build ./...`, `go test ./...`
- **Frontend (c1)**: `npm install`, `npm run build`, `npm test`
- **Python**: `uv sync`, `pytest`

## 4. Agent & Skill Management

### `activate_skill` (Load specialized persona)
**Syntax**: `activate_skill <agent-name>`
- Loads specialized instructions and constraints from `.gemini/agents/<agent-name>.md`.
- **Available Agents**: `c4-scout`, `ai-engineer`, `backend-architect`, `frontend-developer`, `security-auditor`, etc.

### Gemini Setup (Prerequisites)
To use Gemini-exclusive features or the "Ultimate Duo" mode:
1. **Install Gemini CLI**: Ensure the `gemini` command is available in your PATH.
2. **Set API Key**: 
   ```bash
   cq secret set gemini.api_key  # Enter your Google AI API Key
   ```
   Or set the `GOOGLE_API_KEY` environment variable.

## 5. Best Practices
1.  **Always use flags**: Do not rely on positional arguments for `c4` commands.
2.  **Quote strings**: Always quote titles and descriptions to avoid shell parsing errors.
3.  **Check exit codes**: If a command fails, read the error message and check this handbook.
4.  **Specialized Context**: Use `activate_skill c4-scout` before any broad codebase exploration to save tokens and time.
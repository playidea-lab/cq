# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

1. Check if C4 is already initialized by calling `mcp__c4__c4_status`
2. If already initialized, inform the user and show current status
3. If not initialized, run: `uv run c4 init --project-id "$ARGUMENTS"`
   - If no project-id provided, use the current directory name
4. After initialization, call `mcp__c4__c4_status` to confirm success
5. Show the user what was created:
   - `.c4/` directory with config and state files
   - `docs/PLAN.md`, `docs/CHECKPOINTS.md`, `docs/DONE.md`

## Usage

```
/c4-init [project-id]
```

## Example

```
/c4-init my-awesome-project
```

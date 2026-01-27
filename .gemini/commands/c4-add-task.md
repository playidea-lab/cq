# C4 Add Task

Add a new task to the C4 queue.

## Instructions

1. Collect task information:
   - task_id: Unique identifier
   - title: Task title
   - dod: Definition of Done (must be specific and verifiable)
   - scope: Affected files/directories
2. Call `c4_add_todo()` MCP tool

## Usage

```
/c4-add-task T-001 --title "Implement login" --dod "Login returns JWT token"
```

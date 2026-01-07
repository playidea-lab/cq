# C4 Add Task

Add a new task to the project queue.

## Arguments

```
/c4-add-task <task-id> "<title>" "<dod>" [scope]
```

- `task-id`: Unique task identifier (e.g., T-001, FEAT-01)
- `title`: Brief task title
- `dod`: Definition of Done - clear completion criteria
- `scope`: (Optional) File/directory scope for the task

## Instructions

1. Parse the arguments from `$ARGUMENTS`
2. Call `mcp__c4__c4_add_todo` with:
   - task_id
   - title
   - dod
   - scope (if provided)
3. Call `mcp__c4__c4_status` to show updated queue
4. Remind user to update `docs/CHECKPOINTS.md` if this task is part of a checkpoint

## Usage Examples

```
/c4-add-task T-001 "Implement login" "Login form works with validation"
/c4-add-task T-002 "Add tests" "80% coverage" src/auth/
```

## Best Practices

- Use clear, unique task IDs
- Write specific, testable DoD
- Define scope to prevent conflicts in multi-worker setups

# C4 Submit Task

Submit a completed task with validation results.

## Arguments

```
/c4-submit <task-id> [commit-sha]
```

- `task-id`: The task ID being submitted
- `commit-sha`: (Optional) Git commit SHA of the work

## Instructions

1. Parse task-id from `$ARGUMENTS`
2. If commit-sha not provided, get it from `git rev-parse HEAD`
3. Run validations: `mcp__c4__c4_run_validation()`
4. If validations pass:
   - Call `mcp__c4__c4_submit(task_id, commit_sha, validation_results)`
   - Show submission result
   - Show next action (next task or checkpoint)
5. If validations fail:
   - Show which validations failed
   - Do NOT submit
   - Advise user to fix issues first

## Usage

```
/c4-submit T-001
/c4-submit T-001 abc123def
```

## Validation Results Format

```json
[
  {"name": "lint", "status": "pass"},
  {"name": "unit", "status": "pass"}
]
```

## After Submission

The system will automatically:
- Mark the task as done
- Release the scope lock
- Check if a checkpoint is reached
- Trigger supervisor review if checkpoint conditions met

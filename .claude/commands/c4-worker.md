# C4 Worker Operations

Get a task assignment and work on it as a C4 worker.

## Instructions

### Getting a Task

1. Generate or use provided worker ID (default: "claude-worker")
2. Call `mcp__c4__c4_get_task(worker_id)` to get assigned a task
3. If a task is assigned, show:
   - Task ID and title
   - Definition of Done
   - Scope (files to work on)
   - Branch to work on
4. If no tasks available, inform the user

### Working on the Task

Once assigned:
1. Create/checkout the work branch
2. Implement the task according to DoD
3. Run validations: `mcp__c4__c4_run_validation(["lint", "unit"])`
4. If validations pass, submit the work

### Submitting Work

After completing the task:
1. Commit your changes
2. Call `mcp__c4__c4_submit(task_id, commit_sha, validation_results)`
3. The system will:
   - Mark task as done
   - Check if checkpoint is reached
   - Assign next task or await checkpoint review

## Usage

```
/c4-worker [worker-id]
```

## Example Workflow

```
/c4-worker
> Assigned: T-001 "Implement login form"
> Scope: src/auth/
> Branch: c4/w-T-001

[... implement the feature ...]

> Validations: lint=pass, unit=pass
> Submitted: commit abc123
> Next: Awaiting checkpoint CP1 review
```

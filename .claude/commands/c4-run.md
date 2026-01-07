# C4 Start Execution

Transition from PLAN to EXECUTE state and start working on tasks.

## Instructions

1. Call `mcp__c4__c4_status` to check current state
2. If state is not PLAN, inform the user:
   - If EXECUTE: "Already in execution mode"
   - If CHECKPOINT: "Waiting for checkpoint review"
   - If COMPLETE: "Project already complete"
   - If HALTED: "Project is halted. Use /c4-resume to continue"
3. If state is PLAN:
   - Run `uv run c4 run` to transition to EXECUTE
   - Call `mcp__c4__c4_status` to confirm
4. After successful transition, show:
   - New state
   - Available tasks in queue
   - Instructions for workers to join

## Usage

```
/c4-run
```

## Next Steps After Run

After starting execution, workers can:
1. Use `/c4-worker` to get assigned a task
2. Implement the task
3. Run validations
4. Submit completed work

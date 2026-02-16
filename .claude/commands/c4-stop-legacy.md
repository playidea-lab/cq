# C4 Stop Execution

Stop project execution and transition to HALTED state.

## Instructions

1. Call `mcp__c4__c4_status` to check current state
2. If state is EXECUTE or CHECKPOINT:
   - Run `uv run c4 stop` to halt execution
   - Call `mcp__c4__c4_status` to confirm
   - Show the user:
     - Previous state
     - Current state (HALTED)
     - Any in-progress tasks that were interrupted
3. If state is already HALTED, COMPLETE, or PLAN:
   - Inform the user that stop is not applicable

## Usage

```
/c4-stop
```

## Note

Stopping does not lose progress. Use `/c4-run` to resume execution.

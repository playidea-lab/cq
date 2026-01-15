# C4 Project Status

Show the current C4 project status.

## Instructions

1. Call `mcp__c4__c4_status` to get the current project status
2. Display the status in a clear, formatted way:
   - Project ID
   - Current State (INIT/PLAN/EXECUTE/CHECKPOINT/COMPLETE/HALTED)
   - Execution Mode
   - Task Queue:
     - Pending tasks (count and IDs)
     - In-progress tasks (with worker assignments)
     - Completed tasks
   - Active Workers
   - Last Validation Results
   - Metrics (events, validations, tasks completed, checkpoints passed)

## Usage

```
/c4-status
```

## Output Format

```
C4 Status: [PROJECT_ID]
============================
State: EXECUTE (running)

Queue:
  - Pending: 2 tasks (T-003, T-004)
  - In Progress: 1 task (T-002 -> worker-1)
  - Done: 1 task (T-001)

Workers:
  - worker-1: busy (T-002, scope: src/feature)

Last Validation: lint=pass, unit=pass

Metrics:
  - Tasks completed: 1
  - Checkpoints passed: 0
```

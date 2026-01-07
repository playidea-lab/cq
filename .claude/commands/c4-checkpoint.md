# C4 Checkpoint Review

Trigger or check checkpoint status.

## Instructions

### Checking Checkpoint Status

1. Call `mcp__c4__c4_status` to check current state
2. If in CHECKPOINT state:
   - Show checkpoint ID and conditions
   - Show tasks completed for this checkpoint
   - Show validation results
3. If in EXECUTE state:
   - Check if checkpoint conditions are met
   - Show progress toward next checkpoint

### Recording Checkpoint Decision

If you are the supervisor (or simulating one):

```
mcp__c4__c4_checkpoint(
  checkpoint_id="CP1",
  decision="APPROVE|REQUEST_CHANGES|REPLAN",
  notes="Explanation",
  required_changes=["change1", "change2"]  # for REQUEST_CHANGES
)
```

## Decisions

| Decision | Effect |
|----------|--------|
| APPROVE | Move to next phase or COMPLETE |
| REQUEST_CHANGES | Create fix tasks, return to EXECUTE |
| REPLAN | Return to PLAN for re-architecture |

## Usage

```
/c4-checkpoint
```

## Example

```
Checkpoint: CP1 (Phase 1 Review)
Status: Ready for review

Conditions:
  - Tasks: T-001 (done), T-002 (done)
  - Validations: lint=pass, unit=pass

Awaiting supervisor decision...
```

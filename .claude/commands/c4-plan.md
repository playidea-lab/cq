# C4 Plan Mode

Enter or re-enter PLAN mode for project planning.

## Instructions

1. Call `mcp__c4__c4_status` to check current state
2. If state is INIT or HALTED:
   - Run `uv run c4 plan` to enter PLAN mode
3. If already in PLAN:
   - Inform user they're already in planning mode
   - Show current plan documents
4. If in EXECUTE or CHECKPOINT:
   - Warn user that entering PLAN will pause execution
   - Ask for confirmation before proceeding

## Planning Workflow

Once in PLAN mode, help the user:

1. **Define the project scope** in `docs/PLAN.md`
2. **Create tasks** with clear DoD (Definition of Done)
3. **Define checkpoints** in `docs/CHECKPOINTS.md`
4. **Set completion criteria** in `docs/DONE.md`

## Adding Tasks

Use `/c4-add-task` or the MCP tool:
```
mcp__c4__c4_add_todo(task_id, title, dod, scope)
```

## Usage

```
/c4-plan
```

## Next Steps

After planning is complete, use `/c4-run` to start execution.

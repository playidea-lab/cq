# C4 Start Execution

Start the autonomous execution loop (PLAN → EXECUTE).

## Instructions

1. **Check State**: Call `c4_status()` to verify the current state.
2. **Transition to EXECUTE**: Call `c4_run()` to start the execution phase.
3. **Monitor Progress**: 
   - Observe task assignments to workers.
   - Use `/c4-status` to track progress.
4. **Halt if needed**: Use `/c4-stop` to pause execution.

## Usage

```
/c4-run
```
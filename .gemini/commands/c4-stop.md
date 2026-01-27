# C4 Stop Execution

Stop the current C4 execution.

## Instructions

1. Call `c4_status()` to check current state
2. If in EXECUTE state, call `c4_stop()` to halt execution
3. Confirm the state changed to HALTED

## Usage

```
/c4-stop
```

# C4 Project Planning

Analyze project documentation and generate an execution plan (tasks).

## Instructions

1. **Scan Documents**: Read `PRD.md`, `README.md`, `specs/`, or any provided documentation.
2. **Identify Requirements**: Extract features, constraints, and EARS patterns.
3. **Transition to PLAN**: Call `c4_plan()` if the state is not already PLAN.
4. **Add Tasks**: Call `c4_add_todo()` for each identified task.
   - Use structured IDs (e.g., T-001, T-002).
   - Define clear **DoD** (Definition of Done).
   - Specify dependencies between tasks.
5. **Verify Plan**: Use `c4_status()` to show the generated queue.

## Usage

```
/c4-plan
```
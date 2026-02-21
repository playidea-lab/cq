# /c4-refine

Trigger: `/c4-refine` or keywords: `refine`, `리파인`, `계획 고도화`

## What it does

Iterative review loop on the **plan** — not the code. Run after `/c4-plan` to stress-test the design before any implementation starts.

Each round, a fresh reviewer examines the specs, design decisions, and task DoDs, then raises concerns. You discuss and update the plan. Repeat until the plan is solid.

```
/c4-refine
→ spawn fresh reviewer (reads specs + design + task DoDs)
→ reviewer raises concerns, gaps, risks
→ discuss and update plan
→ spawn new reviewer (clean context, no bias)
→ repeat until reviewer finds nothing to improve
→ PLAN CONVERGED ✅
```

## When to run

Between `/c4-plan` and `/c4-run`:

```
/c4-plan → /c4-refine → /c4-run → /c4-polish → /c4-finish
```

Run refine when:
- The feature is complex or risky and you want the plan stress-tested
- You're unsure about edge cases or architectural decisions
- A previous implementation failed and you want to review the plan before retrying

## What the reviewer checks

- Are the task boundaries clear and non-overlapping?
- Are the DoDs testable and specific?
- Are dependencies between tasks correct?
- Are there missing edge cases or failure modes?
- Are architecture decisions justified?
- Is scope creep hiding inside any task?

## Example

```
/c4-refine

  ● Round 1
    Reviewer: T-003 DoD doesn't cover token refresh failure path.
              T-004 depends on T-001 but dependency not declared.
    → Updated T-003 DoD + added T-001→T-004 dependency

  ● Round 2
    Reviewer: All tasks look well-defined. No further concerns.
    → CONVERGED ✅

  Plan ready. Run /c4-run to start workers.
```

## Options

```
/c4-refine                     # default (max 3 rounds)
/c4-refine --max-rounds 5      # more rounds for complex features
/c4-refine --scope "T-003"     # focus on a specific task
```

# /c4-polish (Deprecated)

> **`/c4-polish` has been merged into `/c4-finish`.**

The build-test-review-fix loop is now a built-in phase of `/c4-finish`. No separate invocation is needed.

```
/c4-plan "feature description"   → discovery + design + tasks
/c4-run                          → implement in parallel
/c4-finish                       → polish loop (built-in) → build → test → commit ✅
```

## What moved where

| Previously | Now |
|-----------|-----|
| `/c4-polish` — build-test-review-fix loop | Built into `/c4-finish` (Step 0: Polish Loop) |
| Two-phase: quality gate → full convergence | Same logic, auto-runs inside `/c4-finish` |
| Called automatically by `/c4-run` | Still auto-called, via `/c4-finish` |

The Go-level **polish gate** in `c4_submit` enforces that code with diff ≥ 5 lines has been reviewed before submission. See [Quality Gates](/workflow/#quality-gates) for details.

See [/c4-finish](/workflow/finish) for the complete post-implementation workflow.

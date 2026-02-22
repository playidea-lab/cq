# /c4-refine (Deprecated)

> **`/c4-refine` has been merged into `/c4-plan`.**

The plan critique loop — spawning a fresh reviewer to stress-test specs, DoDs, and task design — is now a built-in phase of `/c4-plan` (Phase 4.5: Plan Critique Loop).

No separate step needed. The workflow is now:

```
/c4-plan "feature description"   → discovery + design + critique loop + tasks
/c4-run                          → implement + auto-polish + finish
```

## What moved where

| Previously | Now |
|-----------|-----|
| `/c4-refine` stress-tests the plan | Built into `/c4-plan` Phase 4.5 |
| Spawns fresh plan critic each round | Same — auto-runs inside `/c4-plan` |
| Stops when CRITICAL + HIGH = 0 | Same convergence condition |

See [/c4-plan](/workflow/plan) for details on the critique loop.

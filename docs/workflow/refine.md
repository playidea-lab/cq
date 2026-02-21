# /c4-refine

Trigger: `/c4-refine` or keywords: `refine`, `리파인`, `품질 게이트`

## What it does

Iterative review-fix loop. Runs after `/c4-run` completes and keeps going until critical and high severity issues reach zero.

```
/c4-refine
→ spawn fresh reviewer worker
→ review finds CRITICAL + HIGH issues
→ fix issues
→ commit
→ spawn new reviewer (different context, no bias)
→ repeat until CRITICAL = 0 and HIGH = 0
→ CONVERGED
```

A new reviewer is spawned each round to prevent confirmation bias — the next reviewer has no memory of what the previous one approved.

## When to run

Between `/c4-run` (execution) and `/c4-polish` (final polish):

```
/c4-run → /c4-refine → /c4-polish → /c4-finish
```

Run refine when:
- A checkpoint flagged issues that need systematic fixing
- You want a quality gate before the final polish loop

## Convergence condition

Refine stops when:
- **CRITICAL = 0 and HIGH = 0** (default threshold)
- Or after max rounds (default: 5)

MEDIUM and LOW issues are left for `/c4-polish` to handle.

## Difference from /c4-polish

| | `/c4-refine` | `/c4-polish` |
|---|---|---|
| Loop unit | Review → Fix | Build → Test → Review → Fix |
| Stop condition | CRITICAL + HIGH = 0 | Zero modifications found |
| Build/test | Manual | Every round automatically |
| Typical use | After checkpoint | Before finish |
| Intensity | Quality gate | Full convergence |

## Options

```
/c4-refine                     # default (max 5 rounds, threshold=high)
/c4-refine --max-rounds 3      # stop after 3 rounds
/c4-refine --threshold medium  # also fix MEDIUM issues
/c4-refine --scope "api/ db/"  # limit to specific directories
```

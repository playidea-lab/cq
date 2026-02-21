# /c4-polish

Trigger: `/c4-polish` or keywords: `polish`, `폴리쉬`, `수렴`

## What it does

Build-test-review-fix loop that runs until a reviewer finds zero modifications. Every round includes a full build and test run before the review.

```
/c4-polish
→ build + test
→ spawn fresh reviewer
→ reviewer finds issues → fix + commit
→ build + test again
→ spawn new reviewer (clean context)
→ repeat
→ reviewer returns "no changes needed" → CONVERGED ✅
```

## When to run

After `/c4-refine` (or directly after `/c4-run`), before `/c4-finish`:

```
/c4-run → /c4-refine → /c4-polish → /c4-finish
```

`/c4-finish` runs `/c4-polish` automatically unless you pass `--no-polish`.

## Convergence condition

Polish stops when a reviewer finds **zero modifications** — not just when severity counts drop, but when there is genuinely nothing left to change.

## Options

```
/c4-polish                        # default (max 8 rounds, threshold=medium)
/c4-polish --max-rounds 5         # stop after 5 rounds
/c4-polish --threshold low        # also fix LOW severity issues
/c4-polish --scope "hub/ llm/"    # limit to specific directories
/c4-polish --no-test              # skip test phase (fast UI-only changes)
```

## Round history output

```
## Polish Summary

- Rounds: 3/8 (converged)
- Total modifications found: 13 → Fixed: 13
- Remaining: 0

| Round | CRIT | HIGH | MED | LOW | Action  |
|-------|------|------|-----|-----|---------|
| 1     | 1    | 2    | 4   | 2   | Fixed   |
| 2     | 0    | 1    | 2   | 1   | Fixed   |
| 3     | 0    | 0    | 0   | 0   | PASS ✅ |
```

## Difference from /c4-refine

| | `/c4-refine` | `/c4-polish` |
|---|---|---|
| Loop unit | Review → Fix | Build → Test → Review → Fix |
| Stop condition | CRITICAL + HIGH = 0 | Zero modifications found |
| Build/test | Manual | Every round automatically |
| Typical use | After checkpoint | Before finish |
| Intensity | Quality gate | Full convergence |

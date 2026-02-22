# /c4-polish

Trigger: `/c4-polish` or keywords: `polish`, `폴리쉬`, `수렴`

## What it does

Build-test-review-fix loop that runs in two phases:

1. **Quality gate** — fix until CRITICAL + HIGH issues reach zero
2. **Full convergence** — fix until a reviewer finds zero modifications

Every round includes a full build and test run before the review.

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

`/c4-run` calls `/c4-polish` automatically in continuous mode — you typically don't need to invoke it manually.

```
/c4-plan → /c4-run → [auto: polish → finish]
```

Run it manually when:
- You've made additional changes after `/c4-run` completed
- You want to run a targeted polish pass on specific directories

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

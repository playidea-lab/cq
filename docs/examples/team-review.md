# Example: Quality Gates in Action

See how CQ enforces quality through automated gates — not trust.

::: info solo tier
Quality gates are compiled into the CQ binary. They work in all tiers, no configuration needed.
:::

## Scenario

You're adding a payment webhook handler. This touches sensitive code — you want CQ's full quality machinery working for you.

## Step 1 — Plan with critique

> **You:** "Stripe webhook 핸들러 추가해줘. 결제 성공, 실패, 환불 이벤트 처리"

```
/c4-plan "Stripe webhook handler"

  ● Discovery
    Q: Signature verification?    → Yes, HMAC-SHA256
    Q: Idempotency handling?      → Yes, deduplicate by event ID
    Q: Retry behavior?            → Stripe retries 3x, must be idempotent

  ● Tasks (5)
    T-001  Webhook router + signature verify
    T-002  Payment success handler
    T-003  Payment failure handler
    T-004  Refund handler
    T-005  Integration tests with Stripe mock

  ● Critique loop triggered (5 tasks → refine gate)
    Round 1: "T-001 missing rate limiting" → added to DoD
    Round 2: "No replay attack prevention" → timestamp check added
    Round 3: 0 CRITICAL, 0 HIGH → CONVERGED ✅
```

The **refine gate** requires plans with 4+ tasks to pass the critique loop. This is a Go-level check — it cannot be skipped.

## Step 2 — Workers with polish

```
/c4-run

  ◆ T-001  [worker-a]  implementing...
    Polish round 1: reviewer found missing error log → fixed
    Polish round 2: 0 modifications → CONVERGED
    ✓ submitted (sha: b2c4e91)

  ◆ T-002  [worker-b]  implementing...
    Polish: diff < 5 lines → auto-skipped
    ✓ submitted (sha: 7f3a82d)
```

Each worker runs a **polish loop** before submitting:
1. Spawn a code reviewer agent (6-axis evaluation)
2. Fix issues found
3. Repeat until zero modifications

The **polish gate** in `c4_submit` rejects code with diff ≥ 5 lines that hasn't been self-reviewed. Small diffs (< 5 lines) are auto-approved.

## Step 3 — Automated review

After each task submits, CQ automatically creates a review task:

```
  ✓ T-001 submitted → R-001 created (6-axis review)

  R-001 review:
    ✅ Correctness   — signature verification correct
    ✅ Security      — HMAC comparison is constant-time
    ✅ Reliability   — idempotency key prevents duplicate processing
    ✅ Observability — structured logging on all paths
    ⚠️ Tests        — missing edge case: expired timestamp
    ✅ Readability   — clear naming, good structure

  Decision: REQUEST CHANGES → T-001-1 revision created
```

The review found a missing test case. CQ automatically creates a revision task (`T-001-1`) with the specific issue to fix.

## Step 4 — Revision cycle

```
  ◆ T-001-1  [worker-d]  fixing expired timestamp test...
    ✓ submitted

  R-001-1 review:
    ✅ All 6 axes pass
    Decision: APPROVED ✅
```

If a task fails review 3 times (max revision), CQ stops and asks for human intervention. Most tasks pass within 1–2 revisions.

## The three gates

| Gate | When | What it enforces | Level |
|------|------|-----------------|-------|
| **Refine** | `/c4-plan` creates 4+ tasks | Critique loop must run first | Go binary |
| **Polish** | Worker submits code (diff ≥ 5 lines) | Self-review must converge | Go binary |
| **Review** | After every implementation task | 6-axis evaluation by separate agent | Go binary |

These are **compiled into the CQ binary** — not prompts, not suggestions, not optional. The system enforces quality structurally.

## What you get

```
/c4-finish

  ● Polish: 1 round → 0 changes
  ● Build:  go build ./... ✓
  ● Tests:  189 passed, 0 failed
  ● Commit: feat(webhook): Stripe payment webhook with idempotency
```

Behind the scenes:
- 5 tasks planned, critique-loop validated
- 3 workers self-reviewed via polish gate
- 2 review issues caught and fixed automatically
- 0 human intervention required

## Next steps

- **Ship without babysitting**: → [Idea to Ship](/examples/idea-to-ship)
- **Run on remote GPUs**: → [Distributed Experiments](/examples/distributed-experiments)

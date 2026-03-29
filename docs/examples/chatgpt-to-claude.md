# ChatGPT Ideas → Claude Implementation

Start an idea in ChatGPT, save it to your CQ brain, then pick it up in Claude Code for implementation. One brain, two hands.

---

## The Scenario

You're on your phone, chatting with ChatGPT about a feature idea. You work out the concept, edge cases, and rough API design. Later, at your desk, you open Claude Code and implement it — with all the context already there.

---

## Step 1: Brainstorm in ChatGPT

You're discussing a webhook retry system with ChatGPT:

```
You: "I need exponential backoff for webhook delivery.
      Max 5 retries, 2^n seconds between attempts.
      Should I use a separate queue or inline retry?"

ChatGPT: "A separate queue is better for observability..."
         [detailed discussion follows]
```

---

## Step 2: Save the Snapshot

When the idea is solid, ask ChatGPT to save it:

```
You: "Save this conversation to my CQ knowledge base.
      Title it 'webhook-retry-design'."
```

ChatGPT calls `cq_snapshot`:

```
✓ Saved snapshot: "webhook-retry-design"
  - 12 messages captured
  - Key topics: exponential backoff, dead letter queue, idempotency
```

---

## Step 3: Pick Up in Claude Code

Open Claude Code at your desk:

```
/pi "implement the webhook retry system — I designed it in ChatGPT earlier"
```

CQ searches your knowledge base automatically during `/pi`:

```
Found knowledge: "webhook-retry-design" (saved 2h ago from ChatGPT)
─────────────────────────────────────────────────────────
Key decisions:
- Separate retry queue (not inline)
- Exponential backoff: 2^n seconds, max 5 attempts
- Dead letter queue after max retries
- Idempotency key per delivery attempt
```

The `/pi` session starts with full context — no re-explaining needed.

---

## Step 4: Plan and Implement

```
/c4-plan    → Tasks created from the ChatGPT-informed design
/c4-run     → Workers implement with full context
/c4-finish  → Polish, test, commit
```

---

## Why This Matters

Without CQ:
```
ChatGPT session → closed → gone forever
Claude session  → starts from zero → you re-explain everything
```

With CQ:
```
ChatGPT session → cq_snapshot → knowledge base
Claude session  → cq_recall  → full context loaded
```

**The brain persists. The AI tool is just a window into it.**

---

## Next Steps

- [Connect Remote MCP](remote-mcp.md) — set up mcp.pilab.kr
- [Idea Session Management](idea-sessions.md) — manage multiple idea threads

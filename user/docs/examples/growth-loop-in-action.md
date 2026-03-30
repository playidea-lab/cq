# Growth Loop in Action

Watch CQ evolve over 5 sessions — from knowing nothing about you to anticipating your preferences.

---

## What is the Growth Loop?

Most AI tools start every session from zero. CQ closes the loop:

```
Session → Preferences captured → Rules generated → AI behavior changes
```

This isn't memory. It's **evolution**. Here's what it looks like in practice.

---

## Session 1: You Explain Everything

You're working on a mesh recovery research project.

```
You: "Run the experiment with MPJPE as primary metric.
      Always check MPJPE before looking at other metrics."

CQ: runs experiment, reports MPJPE first
```

When the session ends, CQ captures:

```
Preference detected (count: 1):
  "Check MPJPE metric first when evaluating experiments"
  Level: observation
```

Nothing changes yet. One mention isn't a pattern.

---

## Session 2: You Mention It Again

```
You: "What's the MPJPE? Show me that first."
```

```
Preference reinforced (count: 2):
  "Check MPJPE metric first"
  Level: observation
```

Still just tracking. Two mentions could be coincidence.

---

## Session 3: Pattern Emerges

```
You: "Start with MPJPE numbers, then we'll look at the rest."
```

```
Preference confirmed (count: 3):
  "Check MPJPE metric first"
  Level: hint → written to CLAUDE.md
```

CQ writes a hint to your project's `CLAUDE.md`:

```markdown
# Hints (auto-generated from session patterns)
- Check MPJPE metric first when evaluating experiment results
```

Next session, this hint is loaded into the AI's system prompt.

---

## Session 4-5: Becomes a Rule

By session 5, the same preference has appeared 5 times:

```
Preference promoted (count: 5):
  "Check MPJPE metric first"
  Level: rule → written to .claude/rules/
```

CQ creates a rule file:

```markdown
# .claude/rules/experiment-metrics.md
- Always report MPJPE as the primary metric in experiment results
- Show MPJPE before PA-MPJPE, HD, or other secondary metrics
```

Rules are stronger than hints — they're loaded into every session's system prompt.

---

## Session 6+: CQ Already Knows

```
CQ: "Experiment complete. Results:
     MPJPE: 45.2mm (↓3.1 from baseline)
     PA-MPJPE: 38.7mm
     HD: 1.74mm"
```

You didn't ask. CQ already knows MPJPE comes first.

---

## Real Example: 5 Research Sessions

After 5 mesh recovery research sessions, CQ auto-generated these patterns:

| Count | Level | What CQ Learned |
|-------|-------|-----------------|
| 5x | **Rule** | "Run experiments via Hub automatically" |
| 4x | Hint | "Use `@key=value` metric output format" |
| 4x | Hint | "Check MPJPE/HD/MSD metrics first" |
| 3x | Hint | "Single-DRR experiments before multi-view" |

---

## How Preferences Flow

```
You work normally
       │
       ▼
Session ends → CQ captures decisions, preferences, discoveries
       │
       ▼
Count < 3: stored as observation (invisible)
Count = 3: promoted to hint (CLAUDE.md)
Count = 5: promoted to rule (.claude/rules/)
       │
       ▼
Next session: AI loads rules + hints into system prompt
       │
       ▼
AI behavior changes — without you asking
```

---

## Managing Your Growth

### See what CQ has learned

```sh
cat CLAUDE.md              # Hints
ls .claude/rules/          # Rules
```

### Delete a rule you disagree with

```sh
rm .claude/rules/experiment-metrics.md
```

Deleted rules are permanently suppressed — CQ won't re-generate them.

### Knowledge flows across AI tools

Preferences stored via Remote MCP are available everywhere:
- Learned in Claude Code → available in ChatGPT
- Learned in Cursor → available in Claude Desktop

One brain. Consistent behavior. Everywhere.

---

## Next Steps

- [ChatGPT → Claude](chatgpt-to-claude.md) — cross-AI knowledge flow in practice
- [Research Loop](research-loop.md) — automated experiment cycles

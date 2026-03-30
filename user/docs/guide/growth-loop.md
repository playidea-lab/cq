# Knowledge Loop

> Connect, Adapt, Evolve — every AI session feeds the next.

Every time you work with an AI, you teach it something. Usually that knowledge disappears when the session ends. The Knowledge Loop captures it, accumulates it, and injects it back — so each session starts smarter than the last.

---

## The Problem

AI sessions start from zero. You explain your conventions, correct the same mistakes, re-state the same constraints — every time. By session 100, you've spent hours re-teaching what the AI already learned and forgot.

The Knowledge Loop closes that loop permanently.

---

## What Is the Knowledge Loop?

A three-stage pipeline that runs automatically every time a session ends:

```
You work with AI
       │
       ▼
Session ends → CQ extracts what was learned
       │
       ▼
Knowledge record → count increments in PreferenceLedger
       │
       ├── count ≥ 3 → hint added to CLAUDE.md (soft guidance)
       │
       └── count ≥ 5 → promoted to rule in .claude/rules/ (permanent behavior)
                               │
                               ▼
                    Next session: AI already knows this
```

No configuration required. No tagging. CQ observes and extracts automatically.

---

## How It Works

### Stage 1: Session → Knowledge Record

When a session closes, CQ analyzes what happened:

- Corrections you made to AI output ("not like that, do it this way")
- Patterns in commands you ran repeatedly
- Explicit instructions you gave during the session
- Experiment results and which approaches worked

Each observation becomes a knowledge record with a type and content:

```sh
cq knowledge search "test patterns"
# → "prefer table-driven tests in Go, name subtests descriptively" (session 2026-03-28)
# → "use t.Run() for subtests, not nested test functions" (session 2026-03-22)
```

### Stage 2: Knowledge Record → Preference Ledger

Related records accumulate in the PreferenceLedger. Each time the same pattern appears, its count increments:

```sh
cq preferences list
```

```
ID       Count  Level  Preference
pref-01  7      RULE   Use table-driven tests in Go
pref-02  5      RULE   Run go vet before committing
pref-03  4      HINT   Check MPJPE/HD/MSD metrics in that order
pref-04  2      -      Sort imports before committing
```

### Stage 3: Next Session Context Injection

Before each session starts, CQ injects relevant knowledge into the AI's context:

- **Rules** (count ≥ 5): Written to `.claude/rules/` — loaded into every system prompt
- **Hints** (count ≥ 3): Added to `CLAUDE.md` — soft suggestions the AI sees

The AI doesn't need to be told. It already knows.

---

## Cross-Platform: Claude Discovers, ChatGPT Uses It

In **Pro** and **Team** tiers, the PreferenceLedger lives in the cloud, not on your machine.

A preference observed in a Claude Code session is available in your ChatGPT session the next day. A pattern learned in Cursor shows up in Claude Desktop. Your AI knowledge is unified across tools.

```
Monday: You correct Claude Code three times about a pattern.
        → pref-07 count: 3 → hint created

Tuesday: You open ChatGPT.
         → CQ injects pref-07 hint into ChatGPT's MCP context
         → ChatGPT already knows the pattern. No correction needed.

Friday: pref-07 count reaches 5.
        → Promoted to rule in .claude/rules/
        → All AI tools now treat this as a permanent constraint.
```

No export, no import, no manual sync. The Knowledge Loop handles it.

---

## Persona: AI Learns to Code Like You

Over time, the Knowledge Loop builds a model of your preferences across every dimension of how you work:

- Code style (naming conventions, error handling patterns, test structure)
- Review criteria (what you approve, what you reject, why)
- Experiment methodology (which metrics matter, how you evaluate results)
- Communication style (how detailed you want explanations, what you skip)

```sh
cq preferences list --rules
```

```
ID       Count  Preference
pref-01  12     Always use context.Context as first parameter
pref-02  9      Return errors, don't log and return
pref-03  7      Table-driven tests with descriptive subtest names
pref-04  6      Check MPJPE before HD before MSD in pose estimation
pref-05  5      Commit messages: imperative mood, 50 char subject limit
```

These rules load into every AI session automatically. The AI doesn't just follow instructions — it has internalized your judgment.

---

## Team: Teammate's Discovery Becomes Your Context

In **Team** tier, the Knowledge Loop extends across your team.

When Alice's AI discovers that "gradient norm > 10 before epoch 5 predicts training instability", that pattern — after it's confirmed through multiple sessions — propagates to your AI's context automatically.

You skip the trial-and-error Alice already paid for.

```
Alice's sessions (week 1):
  → Discovers: gradient norm check pattern
  → count reaches 5 → team rule created

Your session (week 2):
  → CQ injects Alice's rule into your context
  → "Check gradient norms before epoch 5"
  → You never ran into the instability Alice debugged
```

What gets shared: behavioral patterns, workflow sequences, tool preferences.
What never gets shared: file paths, repo names, email addresses, personal identifiers.

---

## A Concrete Example: Day 1 vs Day 30

**Day 1 — AI doesn't know you:**

```
You: "Write a test for this function"
AI: func TestFoo(t *testing.T) {
      result := Foo(input)
      if result != expected {
        t.Errorf("...")
      }
    }

You: "Use table-driven tests. And use t.Run for subtests."
AI: [rewrites with table-driven pattern]

You: "The subtest names should be descriptive, not 'case 1'"
AI: [rewrites again]
```

Three corrections. Fifteen minutes of back-and-forth.

**Day 30 — AI codes like you:**

```
You: "Write a test for this function"
AI: func TestFoo(t *testing.T) {
      tests := []struct{
        name   string
        input  string
        expect string
      }{
        {name: "empty input returns error", ...},
        {name: "valid input returns processed result", ...},
      }
      for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) { ... })
      }
    }
```

Zero corrections. The AI already knows your style.

---

## Managing Your Knowledge

```sh
cq preferences list              # All preferences with counts
cq preferences list --hints      # Only hints (count ≥ 3)
cq preferences list --rules      # Only rules (count ≥ 5)
cq preferences show <id>         # Detail for one preference

cq rule delete "check mpjpe first"    # Delete + permanently suppress
```

Deleting a rule suppresses it permanently — CQ will never re-promote that pattern.

---

## Tier Scope

| Tier | Knowledge scope |
|------|----------------|
| Free | Single project, local SQLite |
| Pro | All your projects + all your AI tools, cloud-synced |
| Team | Everything in Pro + teammates' discoveries |

---

## Configuration

The Knowledge Loop runs automatically. To disable:

```yaml
# .c4/config.yaml
growth_loop:
  enabled: false
```

To trigger manually after a session:

```sh
cq session close    # Triggers summary + preference extraction
```

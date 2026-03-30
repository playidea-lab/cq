# Growth Loop

CQ learns how you work and permanently changes AI behavior to match your preferences.

## The Problem It Solves

Every AI session starts from zero. You explain your conventions, correct the same mistakes, re-state the same constraints — session after session. Growth Loop closes that loop.

## How It Works

```
Session ends → preferences extracted → count incremented
                                              │
                                    count ≥ 3 → hint written to CLAUDE.md
                                    count ≥ 5 → promoted to rule in .claude/rules/
                                                 AI behavior changes permanently
```

Four components:

| Component | Role |
|-----------|------|
| **PreferenceLedger** | Stores observed preferences with occurrence counts |
| **Hints** | Suggestions added to `CLAUDE.md` (count ≥ 3) |
| **Rules** | Permanent behavior rules in `.claude/rules/` (count ≥ 5) |
| **GlobalPromoter** | Depersonalizes your patterns and shares them to the community pool |

## A Real Example

After 5 ML research sessions, CQ auto-learned these patterns from observed behavior:

| Occurrences | Level | What CQ Learned |
|-------------|-------|-----------------|
| 5x | **Rule** | "Run experiments via Hub automatically" |
| 4x | Hint | "Use `@key=value` metric output format" |
| 4x | Hint | "Check MPJPE/HD/MSD metrics first" |
| 3x | Hint | "Always run `cq doctor` before starting" |

These rules are injected into `CLAUDE.md` and `.claude/rules/` — loaded into every future session's system prompt.

## Preference → Hint → Rule Lifecycle

### Stage 1: Observation

Preferences are extracted when a session closes. Sources:
- Corrections you made to AI output
- Patterns in how you reviewed code
- Commands you ran repeatedly
- Explicit instructions you gave during the session

### Stage 2: Hint (count ≥ 3)

CQ adds a suggestion to `CLAUDE.md`:

```markdown
<!-- CQ hint: count=3 -->
Check MPJPE/HD/MSD metrics first when evaluating pose estimation results.
```

Hints are soft guidance — the AI sees them but they don't gate behavior.

### Stage 3: Rule (count ≥ 5)

CQ promotes to a permanent rule in `.claude/rules/`:

```markdown
# Auto-generated rule (promoted from hint, count=5)
## Experiment Metrics
Always evaluate MPJPE, HD, and MSD in that order.
Report all three before drawing conclusions.
```

Rules are loaded into the system prompt on every session start. They are as binding as anything in your `CLAUDE.md`.

### Suppression

Delete a rule and CQ permanently suppresses it — it will never be re-promoted from that preference pattern:

```sh
cq rule delete "check mpjpe first"    # Suppresses permanently
```

## PreferenceLedger

The ledger stores your preference history:

```sh
cq preferences list              # All preferences with counts
cq preferences list --hints      # Only hints (count ≥ 3)
cq preferences list --rules      # Only rules (count ≥ 5)
cq preferences show <id>         # Detail for one preference
```

Example output:

```
ID       Count  Level  Preference
pref-01  5      RULE   Run experiments via Hub automatically
pref-02  4      HINT   Use @key=value metric output format
pref-03  4      HINT   Check MPJPE/HD/MSD metrics first
pref-04  2      -      Sort imports before committing
```

## Knowledge Flows Outward

Your preferences don't stay local. The **GlobalPromoter** strips identifying information (paths, usernames, emails, project names) and contributes the pattern to the community knowledge pool.

What gets shared:
- Behavioral patterns ("check metric X before Y")
- Workflow sequences ("run Z after W")
- Tool preferences ("use T instead of U")

What never gets shared:
- File paths
- Repository names
- Email addresses
- Personal identifiers

Community patterns are available to all users. If many users independently discover the same best practice, it surfaces in the community pool — skipping your trial-and-error phase.

## Interaction with Sessions

The Growth Loop is triggered automatically on session close:

```sh
cq session close    # Triggers summary + preference extraction
```

Or configure it to run on every `cq claude` exit. Preferences extracted from the session are merged into the ledger and count increments happen atomically.

## Connected and Full Tiers

In **connected** and **full** tiers, the PreferenceLedger lives in Supabase — shared across:
- All your devices
- All AI tools (Claude Code, ChatGPT, Cursor, Gemini)

A preference observed in a Claude Code session is available in your ChatGPT session the next day. The Growth Loop accumulates across your entire AI usage, not just one tool.

In **solo** tier, the ledger is local SQLite. Preferences accumulate but don't sync across devices or tools.

## Opt Out

To disable automatic extraction on session close:

```yaml
# .c4/config.yaml
growth_loop:
  enabled: false
```

Individual rules can be deleted and suppressed without disabling the whole system.

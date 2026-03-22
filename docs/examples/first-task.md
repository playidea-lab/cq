# Example: Your First Task

::: info solo tier
This example works with the `solo` tier — no cloud setup required. Just CQ + one AI coding assistant.
:::

## Scenario

You want to build a script that reads a CSV file and prints a summary: row count, column names, and basic stats.

No code written yet. Just an idea.

## Step 1 — Describe your idea

Open a terminal in your project folder and start CQ:

```sh
cq claude   # or: cq cursor / cq codex / cq gemini
```

Then type:

> **You:** "I want a Python script that reads a CSV file and prints a summary — row count, column names, and basic stats for numeric columns."

CQ detects the `/pi` ideation trigger and starts exploring:

```
💡 Ideation mode — no code yet, just thinking.

Got it. A CSV summary script. A few quick questions:
- Should it handle missing values gracefully?
- Output to terminal only, or also save to a file?
- Any specific libraries you prefer? (pandas, csv module, polars?)

> pandas is fine, terminal output only, handle missing values
```

When you're ready, it generates a plan:

```
📋 Plan ready

Task: Build CSV summary script
  - Read any CSV file passed as argument
  - Print: row count, column names, dtype per column
  - For numeric columns: mean, min, max, null count
  - Handle missing values without crashing

Shall I proceed?  [Y/n]
```

## Step 2 — Run it

Type `Y` (or just press Enter). CQ spawns a worker:

```
/c4-run

  ● Spawning worker for: Build CSV summary script
  ✓ Created: scripts/csv_summary.py
  ✓ Tests pass
  ✓ Committed: feat: add CSV summary script

Done in 43 seconds.
```

Your project now has a working script:

```sh
python scripts/csv_summary.py data/my_data.csv

  Rows:     1,204
  Columns:  8  (age, income, score, city, ...)

  Numeric columns:
    age     → mean: 34.2  min: 18  max: 72  nulls: 0
    income  → mean: 52400  min: 18000  max: 210000  nulls: 3
    score   → mean: 7.4   min: 1   max: 10  nulls: 12
```

## What happened

```
You described the goal
  ↓
/pi refined the requirements (30 seconds)
  ↓
/c4-run spawned a worker in an isolated branch
  ↓
Worker wrote the code, self-reviewed (polish gate), committed
  ↓
Result lands in your repo — verified by Go-level gates
```

No boilerplate. No copy-pasting from Stack Overflow. No debugging loops.

## Next steps

- **Modify it**: "Add a flag to export the summary as JSON"
- **Fix something**: → [Quick Bug Fix](/examples/quick-fix)
- **Build something bigger**: → [Feature Planning](/examples/feature-planning)

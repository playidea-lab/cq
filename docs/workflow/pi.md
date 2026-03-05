# /pi (Play Idea)

Trigger: `/pi` or keywords: `play idea`, `아이디어`, `ideation`, `brainstorm`

## What it does

`/pi` is the ideation step that comes **before** `/c4-plan`. Use it when you have a vague idea and need to think it through before committing to a plan.

It runs through four modes:

1. **Diverge** — freely explore possibilities, analogies, and adjacent ideas
2. **Research** — web search + knowledge base for real-world context and prior art
3. **Converge** — narrow down to a concrete, actionable concept
4. **Debate** — stress-test the idea with counter-arguments and risk analysis

When the idea is sufficiently crystallized, `/pi` writes `idea.md` and **automatically calls `/c4-plan`** — no extra step needed.

## Example

```
/pi
```

CQ will ask: "What's the idea?" — then drive the conversation through diverge → research → converge → debate until you're ready to plan.

Or jump straight in:

```
/pi "add real-time collaboration to the editor"
```

## When to use

| Situation | Use |
|-----------|-----|
| Clear requirements | `/c4-plan` directly |
| Vague idea, need exploration | `/pi` → auto-transitions to `/c4-plan` |
| Brainstorming alternatives | `/pi` |
| Researching prior art / SOTA | `/pi` (Research mode) |

## Output

- `idea.md` — structured summary of the converged idea (goal, constraints, risks, key decisions)
- Automatic transition to `/c4-plan` with the idea as context

## After /pi

`/c4-plan` is called automatically. If you want to pause between ideation and planning, exit `/pi` early — then run `/c4-plan` manually when ready.

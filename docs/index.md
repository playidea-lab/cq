---
layout: home

hero:
  name: "CQ"
  text: "External Brain for AI"
  tagline: "AI codes fast but forgets, skips planning, and doesn't learn. CQ is the brain it's missing — plans, verifies, remembers, and becomes you."
  actions:
    - theme: brand
      text: Get Started
      link: /guide/install
    - theme: alt
      text: View on GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 📋
    title: Plans Before Coding
    details: "AI tools jump to code. CQ asks 'what are we building?' first — requirements, architecture, DoD. Then breaks it into verified tasks."

  - icon: 🔒
    title: Gates, Not Trust
    details: "Refine gate blocks bad plans. Polish gate blocks unreviewed code. Review gate runs 6-axis evaluation. Compiled into the binary — not optional."

  - icon: 🧠
    title: Observes You. Becomes You.
    details: "AI tools forget every session. CQ watches how you code, review, and decide — then mirrors it. Your patterns, your team's, everyone's. It becomes you."

  - icon: ⚡
    title: Zero Config
    details: "curl install → cq auth → start. No API keys. No config files. Brain in the cloud, hands on your machine."

---

## How It Works

```
 You say          CQ does                        You get
─────────────────────────────────────────────────────────────
 "이런 거 만들자"   /pi → brainstorm + research     idea.md
 "만들어"          /c4-plan → tasks + review       plan
 ⏳               /c4-run → parallel workers      code + tests
 ☕               /c4-finish → polish + verify    shipped
```

Every step is **gated**: plans require critique review, implementations require polish, reviews require 6-axis evaluation. Nothing ships without passing.

---

## The Numbers

| Metric | Value |
|--------|-------|
| Tasks completed | 1,200+ |
| Review approval rate | 93% |
| Setup time (connected) | 2 minutes |
| API keys required | 0 (connected tier) |
| Languages | Go, Python, TypeScript, Rust |

---

## What Makes CQ Different

### 🧠 It Learns You

Most AI coding tools start fresh every session. CQ builds a **3-layer ontology** of your patterns:

- **L1 Local**: Your coding style, review preferences, common decisions
- **L2 Project**: Cross-position patterns, team conventions
- **L3 Collective**: Shared patterns from the community

After 100 tasks, CQ knows your naming conventions. After 500, it anticipates your review feedback.

### 🔒 Quality Is Not Optional

AI can write code fast. But who checks it? CQ enforces quality at the system level:

- **Polish gate**: `c4_submit` rejects code that hasn't been reviewed (diff ≥ 5 lines)
- **Refine gate**: `c4_add_todo` rejects plans without critique (batch ≥ 4 tasks)
- **Review tasks**: Every implementation gets a 6-axis review (correctness, security, reliability, observability, tests, readability)

These aren't suggestions. They're Go-level gates that **cannot be bypassed**.

### 🖥️ Your Team Runs 24/7

Each worker gets one task, fresh context, isolated worktree. No context pollution. No interference.

```sh
/c4-run    # spawns parallel workers, auto-respawns until queue empty
```

Set it before bed. Wake up to committed, reviewed, tested code.

---

## Works With Any AI

CQ is the orchestration layer. The AI is pluggable:

```sh
cq claude    # Claude Code (recommended)
cq cursor    # Cursor
cq codex     # OpenAI Codex
cq gemini    # Gemini CLI
```

---

## Get Started

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login    # GitHub OAuth — no API key needed
cq claude        # or: cq cursor / cq codex / cq gemini
```

Then just say what you need. The brain is in the cloud.

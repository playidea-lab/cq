---
layout: home

hero:
  name: "CQ"
  text: "Install. Login. Build."
  tagline: "No API keys. No config files. Just cq auth and start building. The brain is in the cloud — your machine is the hands."
  actions:
    - theme: brand
      text: Get Started
      link: /guide/install
    - theme: alt
      text: View on GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: ⚡
    title: 2-Minute Setup
    details: "curl install, cq auth login, done. No API keys to manage, no config to write. Cloud handles tasks, knowledge, LLM, and quality gates."

  - icon: 💡
    title: Idea → Ship (Nonstop)
    details: "/pi to brainstorm → auto plan → auto run → auto finish. One flow from thought to committed code. No babysitting."

  - icon: 🔒
    title: Quality by System
    details: "Refine, polish, and review gates are compiled into the binary. Not prompts, not suggestions — Go-level enforcement that cannot be bypassed."

  - icon: 🧠
    title: Learns You
    details: "3-layer ontology (personal → project → collective) tracks your patterns across 1,200+ tasks. Reviews get sharper every session."

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

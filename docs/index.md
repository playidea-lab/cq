---
layout: home

hero:
  name: "CQ"
  text: "The Evolving External Brain"
  tagline: "AI codes fast but remembers nothing. CQ plans, verifies, remembers across sessions, and evolves to think like you."
  actions:
    - theme: brand
      text: Get Started
      link: /guide/install
    - theme: alt
      text: GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: "\U0001F4CB"
    title: Plans Before Coding
    details: "Other AI tools jump straight to code. CQ asks 'what are we building?' first — requirements, architecture, Definition of Done. Then breaks it into verifiable tasks."

  - icon: "\U0001F9E0"
    title: Evolves With You
    details: "Session 1: you explain your preferences. Session 5: CQ already knows. Preferences accumulate → auto-promote to rules → AI behavior changes. Not just memory — growth."

  - icon: "\U0001F504"
    title: Works With Any AI
    details: "Claude, Cursor, ChatGPT, Codex, Gemini. Knowledge flows across tools — what you teach in ChatGPT is available in Claude. One brain, many hands."

---

## How It Works

```
 You say              CQ does                    Result
──────────────────────────────────────────────────────────
 "build this"         /pi → brainstorm + research   idea.md
 "go"                 /c4-plan → tasks + review      plan
 ⏳                   /c4-run → parallel workers     code + tests
 ☕                   /c4-finish → polish + verify    done
```

## Get Started

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq
```

Then tell it what you want. CQ auto-routes: small fixes are handled directly, medium tasks go through `/c4-quick`, large features get the full `/pi` planning pipeline.

**[See Examples →](/examples/first-task)**

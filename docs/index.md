---
layout: home

hero:
  name: "CQ"
  text: "AI Project Orchestration Engine"
  tagline: Plan → Build → Review → Ship. Automated with Claude Code.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/install
    - theme: alt
      text: Quick Start
      link: /guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 🗂️
    title: Structured Workflow
    details: Every task has a Definition of Done, automated review, and checkpoint gates. No more guessing what's done.

  - icon: ⚡
    title: Parallel Workers
    details: Spawn multiple Claude Code agents on isolated worktrees. Each worker gets full context for one task.

  - icon: 🧠
    title: Knowledge Accumulation
    details: Decisions and discoveries are recorded automatically. Future tasks learn from past patterns.

  - icon: 🔒
    title: Secure by Default
    details: AES-256-GCM secret store. API keys never in config files. Shell command review via pre-commit hooks.

  - icon: 🏷️
    title: Named Sessions
    details: Resume any Claude Code session by name with `cq claude -t <name>`. List sessions and unread mail with `cq ls`.

  - icon: 📬
    title: Inter-Session Mail
    details: Send messages between sessions from CLI or MCP tools. Coordinate across parallel agents without leaving the terminal.

  - icon: 🩺
    title: Auto-Heal
    details: StaleChecker automatically detects and resets stuck in-progress tasks. No more manually unblocking frozen workers.

  - icon: ☁️
    title: One-Command Cloud Setup
    details: "`cq auth login` opens GitHub OAuth and auto-configures Supabase credentials. No manual config editing required."

  - icon: 🛡️
    title: Workflow Gates
    details: Hook-based quality enforcement. `git commit` is blocked until `/c4-finish` completes the built-in polish loop. Deprecated skills (`/c4-polish`, `/c4-refine`) are intercepted and redirected automatically.

  - icon: 📱
    title: Headless Auth
    details: "`cq auth login --device` shows a user_code to enter in a browser (RFC 8628 Device Flow). Works in SSH tunnels and containers."

  - icon: 💡
    title: Play Idea (/pi)
    details: Brainstorm and refine ideas before planning. Diverge, research, debate — then auto-launch /c4-plan when ready.
---

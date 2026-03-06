---
layout: home

hero:
  name: "CQ"
  text: "Coding Team for AI Researchers"
  tagline: Your experiments and tools — built by AI, directed by you.
  actions:
    - theme: brand
      text: Install
      link: /guide/install
    - theme: alt
      text: View on GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 🗣️
    title: Just Say It
    details: Describe what you need. AI writes, runs, and reviews the code. No IDE required.

  - icon: 🖥️
    title: Run Everywhere
    details: Spawn AI workers across multiple servers. Experiments and builds run in parallel, around the clock.

  - icon: ☀️
    title: Wake Up to Results
    details: Set it before bed. Your code is written, tested, and committed by morning.

---

## Why CQ?

I built CQ because I needed two things at once: run deep learning experiments across multiple servers *and* build the tools around them — without switching contexts.

The experiments run as distributed workers. The tools get built by AI through `/pi` → `/c4-run`. I just set the direction. The AI team handles the rest.

That's **Human Outside the Loop** — not "AI assists you" but "AI does the work, you steer."

CQ works with any AI coding assistant: Claude Code, Gemini CLI, Codex, or Cursor. One setup. One workflow.

---

## Two Ways to Use CQ

### 🔬 Experiment Automation

Running ML experiments across multiple GPUs or cloud servers:

```
/pi  →  describe your experiment setup
      AI plans the pipeline, writes training scripts,
      submits jobs to multiple servers,
      and reports results when done.
```

You wake up to a comparison table of results.

### 🛠️ Tool Development

Building a research tool, CLI, or internal system:

```
/pi  →  describe what the tool should do
      AI plans the architecture, implements features,
      runs tests, and commits working code.
```

No boilerplate. No debugging loops. Just describe and ship.

---

## Get Started in 30 Seconds

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq claude   # or: cq cursor / cq codex / cq gemini
```

Then type what you need. AI takes it from there.
---

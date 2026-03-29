---
layout: home

hero:
  name: "CQ"
  text: "The Evolving External Brain"
  tagline: "AI that learns you, works for you, and evolves with every session."
  actions:
    - theme: alt
      text: "\U0001F4CB GitHub"
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: "\U0001F310"
    title: Distribute
    details: "Workers run tasks independently in parallel. Set it up before bed — wake up to tested, reviewed code."

  - icon: "\U0001F517"
    title: Connect
    details: "One brain across Claude, ChatGPT, Cursor, Codex, Gemini. Knowledge saved in one AI is available in all of them."

  - icon: "\U0001F9EC"
    title: Mimic
    details: "CQ learns how you think — your judgment, habits, preferences. It builds a persona that decides like you would."

  - icon: "\U0001F4C8"
    title: Evolve
    details: "Bad patterns fade. Good patterns become rules. By session 5, your AI already knows how you work — without being told."

---

<div class="install-block">
  <code id="install-cmd">curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh</code>
  <button class="copy-btn" onclick="navigator.clipboard.writeText(document.getElementById('install-cmd').textContent).then(()=>{this.textContent='Copied!';setTimeout(()=>this.textContent='Copy',1500)})">Copy</button>
</div>

## How It Works

```
    Distribute → Connect → Mimic → Evolve
         ↑                            │
         └────────────────────────────┘
```

```
 You say              CQ does                    Result
──────────────────────────────────────────────────────────
 "build this"         /pi  → brainstorm + research   idea.md
 "go"                 /c4-plan → tasks + review       plan
 ⏳                   /c4-run  → parallel workers     code + tests
 ☕                   /c4-finish → polish + verify    done
```

**[See Examples →](/cq/examples/first-task)**

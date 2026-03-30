# Tiers

> "Infrastructure is free. Intelligence is the product."

CQ has three tiers. **Free** gives you everything to get started — local memory, full MCP toolset, and one GPU. Upgrade to **Pro** when you want your AI to remember across projects and platforms. Add **Team** for shared GPU infrastructure and collective knowledge.

---

## Comparison

| Feature | Free | Pro | Team |
|---------|------|-----|------|
| **Price** | $0 | **$36/yr** (Early Bird) | **$36/seat/yr** |
| Project-local memory | Yes | Yes | Yes |
| Cross-project memory | No | Yes | Yes |
| Cross-platform sync (Claude↔ChatGPT↔Cursor) | No | Yes | Yes |
| All MCP tools | Yes | Yes | Yes |
| E2E encrypted relay (NAT traversal) | Yes | Yes | Yes |
| GPUs | 1 (local only) | Unlimited | Unlimited (pooled) |
| AI autonomous experiment loop | No | Yes | Yes |
| Persona learning (AI codes like you) | No | Yes | Yes |
| Team knowledge auto-sharing | No | No | Yes |
| Team GPU pooling | No | No | Yes |
| Privacy isolation between orgs | No | No | Yes |
| Billing | — | Annual only | Annual only |

> **Early Bird**: Pro is $36/yr for the first 1,000 users, locked forever at that price. After the first 1,000, pricing will increase.

---

## Free — $0

Everything runs locally. Full MCP toolset. One GPU worker (your local machine or one attached server).

```sh
# No login required. Configure your own LLM keys.
cq secret set openai.api_key    # Stored encrypted in ~/.cq/secrets.db
```

```yaml
# .c4/config.yaml
llm_gateway:
  enabled: true
  default: openai
  providers:
    openai:
      enabled: true
      default_model: gpt-4o-mini
```

What you get:
- Full MCP tool suite (knowledge record/search, task orchestration, relay)
- Project-local knowledge base (SQLite, stays on your machine)
- Knowledge Loop within a single project
- E2E encrypted relay for NAT traversal
- 1 device at a time

What you don't get:
- Knowledge doesn't follow you to a second project
- Knowledge doesn't sync to ChatGPT, Cursor, or other AI tools
- No persona learning across tools

Good for: offline use, air-gapped environments, trying CQ before committing.

---

## Pro — $36/yr (Early Bird, first 1,000 users)

Your AI memory becomes persistent, cross-project, and cross-platform. Connect unlimited GPUs.

```sh
cq auth login    # GitHub OAuth, one-time
cq serve         # Start relay + knowledge sync + token refresh
```

What you get on top of Free:
- **Cross-project memory** — knowledge from project A surfaces in project B automatically
- **Cross-platform sync** — discover something in Claude Code, use it in ChatGPT the next day
- **Unlimited GPUs** — connect any machine via `cq serve`; it joins your GPU pool
- **AI autonomous experiment loop** — CQ plans, submits, evaluates, and iterates without you watching
- **Persona learning** — AI learns your coding style, preferred patterns, and judgment over time

**Trigger to upgrade from Free:**
- You run `cq import chatgpt` and want phase 2–3 (cross-platform sync)
- You start a second project and want knowledge to carry over

```sh
# After login, connect a GPU machine (any machine, any network):
ssh gpu-server "cq serve"    # That machine joins your GPU pool
cq gpu status                # See it appear
# GPU: gpu-server (RTX 4090, 24GB) — online, idle
```

Annual billing only. No monthly option.

---

## Team — $36/seat/yr

Everything in Pro, plus collective intelligence and shared infrastructure for your team.

```sh
# Team admin setup:
cq auth login
cq team create my-org
cq team invite alice@company.com bob@company.com

# Each member runs:
cq auth login
cq serve
```

What you get on top of Pro:
- **Team knowledge auto-sharing** — when one teammate's AI discovers a pattern, it surfaces in everyone's context automatically
- **Team GPU pooling** — all connected machines form a shared GPU pool; submit jobs from anywhere, run on any available GPU
- **Privacy isolation** — your org's knowledge is isolated from other orgs; no cross-org leakage

```sh
# Submit a training job from your laptop, run on team GPU:
cq job submit --script train.py --gpu any
# Submitted job-a4f2 → assigned to alice-rtx4090 (available)
# Streaming logs...
```

When Alice's AI learns "always check gradient norms before epoch 5", that pattern propagates to your AI's context — skipping the trial-and-error that Alice already paid for.

Annual billing only. Per seat.

---

## What Tier Am I On?

```sh
cq auth status
```

```
Tier: Pro
User: changmin@pilab.co.kr
Relay: connected (relay.pilab.kr)
Knowledge: cloud (pgvector, 847 records)
GPUs: 2 connected (local + gpu-server)
```

Tier is determined by your login state and whether `cq serve` is running:

- **Free**: Not logged in, or logged in but running fully offline
- **Pro**: Logged in + `cq serve` + no Team configuration
- **Team**: Logged in + `cq serve` + team workspace configured

Data stored locally (Free) is preserved when you upgrade. Cloud sync picks up from where local left off.

---

## Knowledge Loop (all tiers)

The Knowledge Loop runs on every tier, but scope differs:

| Tier | Knowledge scope |
|------|----------------|
| Free | Single project only |
| Pro | All your projects + all your AI tools |
| Team | All your projects + all AI tools + teammates' discoveries |

See [Knowledge Loop](/guide/growth-loop) for how the learning cycle works.

---

## Research Loop (Pro and Team)

Run autonomous ML experiments on your connected GPUs without watching:

```sh
cq research run --goal "maximize accuracy on CIFAR-10" --budget 20
```

```
Loop started: research-7f3a
Iteration 1: submitting experiment...
  Training on gpu-server (RTX 4090)...
  Result: acc=0.847, loss=0.421 @epoch=30
  Planning iteration 2 based on results...
Iteration 2: adjusting learning rate schedule...
  Result: acc=0.863, loss=0.391 @epoch=30
...
Best checkpoint saved to Drive: ckpt-research-7f3a-iter8.pt
```

The loop runs until it hits the budget or a stopping condition. Results stream back through the relay.

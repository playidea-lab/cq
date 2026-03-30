# Tiers

CQ has three tiers. Start with **Free**, upgrade to **Pro** when you want cloud sync and GPU access from anywhere, and add **Team** for collaborative GPU workloads.

## Comparison

| Feature | Free | Pro | Team |
|---------|------|-----|------|
| Task orchestration | Local SQLite | Supabase (cloud) | Supabase (cloud) |
| Knowledge base | Local SQLite | pgvector (cloud) | pgvector (cloud) |
| Multi-worker execution | Single machine | Any machine | Any machine |
| Knowledge Loop | Preferences only | Full (cross-session) | Full (cross-session) |
| Research Loop | No | Yes | Yes |
| Remote AI Workspace (ChatGPT/Claude Desktop) | No | Yes | Yes |
| Drive (file storage) | No | Yes | Yes |
| Hub (distributed GPU jobs) | No | Yes | Yes |
| Relay (NAT traversal, E2E encrypted) | No | Yes | Yes |
| Connected workers | 1 (local only) | Unlimited | Unlimited |
| API keys required | Yes (your own) | 0 | 0 |
| Price | Free | $5–10/mo | Contact us |
| Setup | `config.yaml` required | `cq auth login` | `cq auth login` + `cq serve` |

## Free

Everything runs locally. You manage your own LLM API keys.

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

```sh
cq secret set openai.api_key    # Stored encrypted in ~/.c4/secrets.db
```

Good for: offline use, air-gapped environments, full data control.

## Pro

Connect GPUs anywhere, anytime. No API keys needed — CQ's LLM proxy handles it.

```sh
cq auth login    # GitHub OAuth, one-time
cq serve         # Start relay + event sync + token refresh
cq claude        # Start building
```

What you get:
- GPU workers connected via E2E encrypted relay — any machine, any network
- Knowledge persists across sessions and across AI tools
- [Knowledge Loop](growth-loop.md) accumulates your preferences and experiment results automatically
- [Remote AI Workspace](remote-brain.md) — access your knowledge from ChatGPT, Claude Desktop, Cursor
- `cq relay call` — reach other CQ instances through NAT

Good for: individual developers and ML researchers who want GPU access from anywhere and persistent AI memory across all tools.

## Team

Everything in **Pro**, plus collaborative GPU infrastructure.

```sh
cq auth login
cq serve
```

Then on a GPU machine (or cloud VM):

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login
cq serve    # This machine becomes a GPU worker
```

What you get (on top of Pro):
- Submit ML training jobs from your laptop, run on shared GPU servers
- **Research Loop** — autonomous experiment cycle: plan → train → evaluate → iterate
- Drive — cloud file storage with TUS resumable upload and content-addressable versioning
- Artifact upload — job outputs automatically saved to Drive on completion
- DAG engine — chain dependent jobs with automatic dependency resolution
- Team knowledge pool — shared experiment results and best practices

Good for: ML research teams, organizations with remote GPU infrastructure.

## Knowledge Loop (all tiers)

The Knowledge Loop is available across all tiers, but works best in **Pro** and **Team**:

```
Session 1: you correct a mistake       → preference stored (count: 1)
Session 3: same preference recurs      → hint added to CLAUDE.md
Session 5: 5th occurrence              → promoted to permanent rule
Session 6+: AI follows rule without prompting
```

See [Knowledge Loop](growth-loop.md) for details.

## Research Loop (Pro and Team)

The Research Loop runs autonomously on your connected GPU workers:

```
plan experiment → submit to Hub → train on GPU → evaluate metrics
      ↑                                                    │
      └────────────────────────────────────────────────────┘
              (iterate until stopping condition)
```

Start a loop:

```sh
cq research run --goal "maximize MPJPE on H36M" --budget 10
```

Results stream back through the Hub. The best checkpoint is saved to Drive automatically.

## Switching Tiers

Tier is determined by your login state and whether `cq serve` is running. There is no configuration flag.

- **Free**: Not logged in, or no `cloud.url` in config
- **Pro**: Logged in + `cq serve` running (no Team Hub configured)
- **Team**: Logged in + `cq serve` + multiple GPU workers connected

Data stored in SQLite (Free) is preserved when you upgrade. Cloud sync picks up from where local left off.

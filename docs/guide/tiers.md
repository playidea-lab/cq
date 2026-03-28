# Tiers

CQ has three tiers. Start with **solo**, upgrade to **connected** when you want cloud sync, and add **full** for distributed GPU workloads.

## Comparison

| Feature | solo | connected | full |
|---------|------|-----------|------|
| Task orchestration | Local SQLite | Supabase (cloud) | Supabase (cloud) |
| Knowledge base | Local SQLite | pgvector (cloud) | pgvector (cloud) |
| Multi-worker execution | Single machine | Single machine | Any machine |
| Growth Loop | Preferences only | Full (cross-session) | Full (cross-session) |
| Research Loop | No | No | Yes |
| Remote Brain (ChatGPT/Claude Desktop) | No | Yes | Yes |
| Drive (file storage) | No | Yes | Yes |
| Hub (distributed jobs) | No | No | Yes |
| Relay (NAT traversal) | No | Yes | Yes |
| API keys required | Yes (your own) | 0 | 0 |
| Setup | `config.yaml` required | `cq auth login` | `cq auth login` + `cq serve` |

## solo

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

## connected

Brain in the cloud, hands on your machine. No API keys needed — CQ's LLM proxy handles it.

```sh
cq auth login    # GitHub OAuth, one-time
cq serve         # Start relay + event sync + token refresh
cq claude        # Start building
```

What you get:
- Knowledge persists across sessions and across AI tools
- [Growth Loop](growth-loop.md) accumulates your preferences automatically
- [Remote Brain](remote-brain.md) — access your knowledge from ChatGPT, Claude Desktop, Cursor
- `cq relay call` — reach other CQ instances through NAT

Good for: individual developers who want persistent AI memory across all tools.

## full

Everything in **connected**, plus distributed execution.

```sh
cq auth login
cq serve
```

Then on a GPU machine (or cloud VM):

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq auth login
cq serve    # This machine becomes a worker
```

What you get (on top of connected):
- Submit ML training jobs from your laptop, run on GPU servers
- **Research Loop** — autonomous experiment cycle: plan → train → evaluate → iterate
- Drive — cloud file storage with TUS resumable upload and content-addressable versioning
- Artifact upload — job outputs automatically saved to Drive on completion
- DAG engine — chain dependent jobs with automatic dependency resolution

Good for: ML researchers, teams with remote GPU infrastructure.

## Growth Loop (all tiers)

The Growth Loop is available across all tiers, but works best in **connected** and **full**:

```
Session 1: you correct a mistake       → preference stored (count: 1)
Session 3: same preference recurs      → hint added to CLAUDE.md
Session 5: 5th occurrence              → promoted to permanent rule
Session 6+: AI follows rule without prompting
```

See [Growth Loop](growth-loop.md) for details.

## Research Loop (full only)

The Research Loop runs autonomously:

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

- **solo**: Not logged in, or no `cloud.url` in config
- **connected**: Logged in + `cq serve` running (no Hub configured)
- **full**: Logged in + `cq serve` + Hub workers connected

Data stored in SQLite (solo) is preserved when you upgrade. Cloud sync picks up from where local left off.

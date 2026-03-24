# Tiers

CQ is distributed in three tiers. Choose based on your setup.

## solo (default)

**Local only. No external services required.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
# or explicitly:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier solo
```

Included features:
- Full task management (plan → run → review → finish)
- **Polish & Refine gates** — Go-level quality enforcement
- Local SQLite database
- Git worktree isolation per worker
- Skills embedded in binary
- `cq doctor --fix` environment auto-repair
- Secret store (`~/.c4/secrets.db`)
- Personal Ontology Pipeline (POP)

Best for: personal projects, offline environments, getting started.

---

## connected

**Cloud-first. No API key required — just `cq auth`.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

Start immediately with:

```sh
cq auth login   # GitHub OAuth — no API key needed
```

The CQ cloud (Supabase SSOT) becomes your backend automatically. No manual config file required.

Additional features on top of `solo`:
- **Cloud SSOT** — tasks, knowledge, and LLM calls routed through the cloud. No API key configuration needed.
- **Supabase** cloud storage (tasks, documents, team data)
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama (cloud-managed keys)
- **C3 EventBus** — gRPC event bus for real-time notifications
- **C0 Drive** — file storage via Supabase Storage
- **C9 Knowledge** — semantic search + pgvector for cross-project knowledge sharing
- **Persona/Soul Evolution** — coding style pattern learning
- **C6 Secret Central** — encrypted secret sync (Supabase-backed, cache-first)
- **Relay** — WSS-based NAT traversal for remote MCP access; any machine behind NAT exposes tools via local proxy (`localhost:4140/w/{worker}/mcp`). No tokens in `.mcp.json` — `cq serve` auto-injects JWT on every request. Auto-reconnects on drops, auto-refreshes JWT. WSL2 support with Windows Task Scheduler auto-registration.
- **Telegram bot** — job completion notifications + slash commands via BotFather (`cq setup`)
- **Knowledge auto-pull** — knowledge base synced on session start

Best for: teams, multi-machine setups, AI-powered workflows.

---

## full

**All features, including distributed worker queue and desktop app.**

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

Additional features on top of `connected`:
- **Supabase Worker Queue** — distributed worker queue via LISTEN/NOTIFY (NAT-safe, outbound connection only)
- **CDP** — Chrome DevTools Protocol automation
- **GPU** — local GPU job scheduler
- **Research Loop (C9)** — ML experiment loop with c9-* skills (11 skills)
- **3-Layer Ontology** — L1 local → L2 project → L3 collective pattern learning

Best for: production deployments, ML workflows, large teams.

---

## Comparison

| Feature | solo | connected | full |
|---------|:----:|:---------:|:----:|
| Task management | ✅ | ✅ | ✅ |
| Polish & Refine gates | ✅ | ✅ | ✅ |
| Local SQLite | ✅ | ✅ | ✅ |
| Skills embedded | ✅ | ✅ | ✅ |
| Secret store | ✅ | ✅ | ✅ |
| POP (Personal Ontology) | ✅ | ✅ | ✅ |
| Persona/Soul Evolution | ✅ | ✅ | ✅ |
| Supabase sync | — | ✅ | ✅ |
| LLM Gateway | — | ✅ | ✅ |
| EventBus | — | ✅ | ✅ |
| Knowledge (semantic + auto-pull) | — | ✅ | ✅ |
| Telegram bot | — | ✅ | ✅ |
| Secret Central | — | ✅ | ✅ |
| Skill Health Pipeline | — | ✅ | ✅ |
| Distributed workers (LISTEN/NOTIFY) | — | — | ✅ |
| CDP automation | — | — | ✅ |
| GPU scheduler | — | — | ✅ |
| Research Loop (c9-*) | — | — | ✅ |
| 3-Layer Ontology (L1→L2→L3) | — | — | ✅ |

## Config file location

CQ looks for config at `~/.c4/config.yaml`. For `solo` tier, no config is required — it works out of the box.

For `connected` and `full` tiers, run `cq auth login` to connect automatically. The cloud config (`~/.c4/config.yaml`) is patched automatically after login — no manual setup required.

## Config templates

Copy to `~/.c4/config.yaml` and customize.

### solo

```yaml
# CQ Solo tier configuration template
# Copy to ~/.c4/config.yaml and customize

# Task storage
task_store:
  type: sqlite
  path: ~/.c4/tasks.db

# LLM Gateway (optional - for c4_llm_call tool)
# llm_gateway:
#   default_provider: anthropic
#   providers:
#     anthropic:
#       api_key_env: ANTHROPIC_API_KEY

# Permission reviewer (bash hook)
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true
```

### connected

```yaml
# CQ Connected tier configuration template
# Copy to ~/.c4/config.yaml and customize
# Requires: Supabase account, C3 EventBus (optional)

# Task storage
task_store:
  type: supabase  # or sqlite for local fallback

# Cloud (Supabase)
cloud:
  url: https://your-project.supabase.co
  anon_key: your-anon-key
  # service_role_key: your-service-role-key  # for admin operations

# C3 EventBus (optional)
# eventbus:
#   host: localhost
#   port: 50051

# Permission reviewer
permission_reviewer:
  enabled: true
  mode: hook
  auto_approve: true

# LLM Gateway
# llm_gateway:
#   default_provider: anthropic
#   providers:
#     anthropic:
#       api_key_env: ANTHROPIC_API_KEY
#       base_url: https://your-proxy.example.com  # optional: override API endpoint for LLM proxy

# Background daemon (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30   # reset in_progress tasks stuck longer than this
    interval_seconds: 60
```

### full

```yaml
# CQ Full tier configuration template
# Copy to ~/.c4/config.yaml and customize
# Requires: connected tier setup + Supabase (cloud.url)

# (include all connected settings above, plus:)

# Hub — distributed worker queue (uses Supabase LISTEN/NOTIFY)
hub:
  enabled: true
  # api_key: use: cq secret set hub.api_key <value>  (Supabase service role key)
  # cloud session JWT is used as fallback when no key is configured

# Background daemon (cq serve)
serve:
  stale_checker:
    enabled: true
    threshold_minutes: 30
    interval_seconds: 60
```

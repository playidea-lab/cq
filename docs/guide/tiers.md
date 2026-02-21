# Tiers

CQ is distributed in three tiers. Choose based on your setup.

## solo (default)

**Local only. No external services required.**

```sh
curl -fsSL .../install.sh | sh
# or explicitly:
curl -fsSL .../install.sh | sh -s -- --tier solo
```

Included features:
- Full task management (plan → run → review → finish)
- Local SQLite database
- Git worktree isolation per worker
- Skills embedded in binary
- `cq doctor` environment checks
- Secret store (`~/.c4/secrets.db`)

Best for: personal projects, offline environments, getting started.

---

## connected

**Adds cloud sync, LLM Gateway, and EventBus.**

```sh
curl -fsSL .../install.sh | sh -s -- --tier connected
```

Additional features on top of `solo`:
- **Supabase** cloud storage (tasks, documents, team data)
- **LLM Gateway** — unified API for Anthropic, OpenAI, Gemini, Ollama
- **C3 EventBus** — gRPC event bus for real-time notifications
- **C0 Drive** — file storage via Supabase Storage

Requires: Supabase project URL + anon key. Create a free project at [supabase.com](https://supabase.com), then:

```sh
# Copy the connected config template
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/configs/connected.yaml > ~/.c4/config.yaml
# Edit cloud.url and cloud.anon_key (or use secret store)
cq secret set supabase.anon_key your-anon-key
```

Best for: teams, multi-machine setups, AI-powered workflows.

---

## full

**All features, including distributed job queue and desktop app.**

```sh
curl -fsSL .../install.sh | sh -s -- --tier full
```

Additional features on top of `connected`:
- **C5 Hub** — distributed worker queue (pull model, lease-based)
- **CDP** — Chrome DevTools Protocol automation
- **GPU** — local GPU job scheduler
- **C1 Messenger** — Tauri desktop dashboard
- **Research** — paper/experiment tracking loop

Best for: production deployments, ML workflows, large teams.

---

## Comparison

| Feature | solo | connected | full |
|---------|:----:|:---------:|:----:|
| Task management | ✅ | ✅ | ✅ |
| Local SQLite | ✅ | ✅ | ✅ |
| Skills embedded | ✅ | ✅ | ✅ |
| Secret store | ✅ | ✅ | ✅ |
| Supabase sync | — | ✅ | ✅ |
| LLM Gateway | — | ✅ | ✅ |
| EventBus | — | ✅ | ✅ |
| C5 Hub (distributed) | — | — | ✅ |
| CDP automation | — | — | ✅ |
| GPU scheduler | — | — | ✅ |

## Config templates

Starter configs for each tier are in the [`configs/`](https://github.com/PlayIdea-Lab/cq/tree/main/configs) directory:

```sh
cp configs/connected.yaml ~/.c4/config.yaml
```

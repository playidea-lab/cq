# Config Reference

Config file location: `~/.c4/config.yaml` (global) or `.c4/config.yaml` (per-project).

## When do you need a config?

- **solo tier** — no config required. CQ works out of the box.
- **connected / full tier** — config is provided by your team or organization. Place it at `~/.c4/config.yaml`.

The sections below document available options for reference.

## Sections

### `pop` — Personal Ontology Pipeline (solo+)

```yaml
pop:
  enabled: true
  confidence_threshold: 0.8   # minimum confidence to notify/crystallize to Soul
  max_proposals_per_cycle: 20
  soul_path: ""               # default: .c4/souls/{user}/soul-developer.md
```

### `hub` — Distributed Worker Queue (full tier)

Workers connect to Supabase directly via `pgx LISTEN/NOTIFY` — no local Hub process, no Hub URL to configure.

```yaml
hub:
  enabled: true
  # api_key: use: cq secret set hub.api_key <value>  (Supabase service role key)
  # cloud session JWT is used as fallback when no key is configured
```

The `hub` section uses `cloud.url` and `cloud.direct_url` (port 5432) for the LISTEN/NOTIFY connection. No `hub.url` needed.

::: tip Telegram notifications
Job completion notifications are sent via Telegram. Run `cq setup` to pair a Telegram bot via BotFather.
:::

### `llm_gateway` — LLM providers (connected / full tier)

```yaml
llm_gateway:
  default_provider: anthropic
  providers:
    anthropic:
      enabled: true
      default_model: claude-sonnet-4-6
    openai:
      enabled: false
    ollama:
      enabled: true
      base_url: "http://localhost:11434"
```

::: tip API Keys
Never put API keys in config files. Use the secret store:
```sh
cq secret set anthropic.api_key sk-ant-...
```
:::

### `permission_reviewer` — shell command security hook

```yaml
permission_reviewer:
  enabled: true
  mode: hook        # "hook" (regex) or "model" (LLM review)
  auto_approve: true
  allow_patterns: []
  block_patterns: []
```

### `serve` — background services

```yaml
serve:
  eventbus:
    enabled: false
  gpu:
    enabled: false
  agent:
    enabled: false   # requires cloud.url + cloud.anon_key
  eventsink:
    enabled: false
    port: 4141
    token: ""
  stale_checker:
    enabled: true
    threshold_minutes: 30   # reset in_progress tasks stuck longer than this
    interval_seconds: 60
```

### `worktree`

```yaml
worktree:
  auto_cleanup: true   # remove worktrees after task submit
```

### `validation` — build and test commands

Override the auto-detected commands:

```yaml
validation:
  build_command: "make build"    # default: auto-detected by language
  test_command: "make test"      # default: auto-detected by language
```

Auto-detection: Go → `go build ./...`, Python → `uv run pytest`, Node → `npm run build`, Rust → `cargo build`.

### `cloud` — Supabase (connected / full tier)

Configured by your organization. You should not need to modify this section directly.

```yaml
cloud:
  url: "https://xxxx.supabase.co"
  anon_key: ""
  # direct_url: "postgresql://..."   # optional: used by hub for LISTEN/NOTIFY (port 5432)
```

# Config Reference

Config file location: `~/.c4/config.yaml` (global) or `.c4/config.yaml` (per-project).

## When do you need a config?

- **solo tier** — no config required. CQ works out of the box.
- **connected / full tier** — config is provided by your team or organization. Place it at `~/.c4/config.yaml`.

The sections below document available options for reference.

## Sections

### `hub` — C5 Hub (full tier)

```yaml
hub:
  enabled: true
  url: "http://localhost:8585"
  api_key: ""   # use: cq secret set hub.api_key <value>
```

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
```

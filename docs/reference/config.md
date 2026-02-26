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
  hub:
    enabled: false       # start C5 Hub as a subprocess (full tier)
    binary: "c5"         # binary name to find in PATH
    port: 8585           # port to pass as --port to c5
    args: []             # additional CLI args
  eventbus:
    enabled: false
  gpu:
    enabled: false
  agent:
    enabled: false   # requires cloud.url + cloud.anon_key
  eventsink:
    enabled: false
    port: 4141       # C5 Hub → C4 이벤트 수신 엔드포인트
    token: ""
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 stale 판정
    interval_seconds: 60
  ssesubscriber:
    enabled: false   # full tier 전용 (C5 Hub + C3 EventBus 빌드 태그 필요)
```

When `serve.hub.enabled: true`, `cq serve` automatically starts the `c5` binary as a child process and manages its lifecycle:
- Starts on `cq serve` launch (skips gracefully if binary not found in PATH)
- Health-checked at `http://127.0.0.1:{port}/v1/health` every few seconds
- Stopped cleanly on `cq serve` exit (SIGTERM → 5s wait → SIGKILL)

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

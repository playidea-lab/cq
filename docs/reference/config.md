# Config Reference

Config file location: `~/.c4/config.yaml` (global) or `.c4/config.yaml` (per-project).

## Starter template

Copy a pre-built template from the [configs/ directory](https://github.com/PlayIdea-Lab/cq/tree/main/configs):

```sh
# solo (local only)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/configs/solo.yaml > ~/.c4/config.yaml

# connected (cloud + LLM)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/configs/connected.yaml > ~/.c4/config.yaml
```

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

### `cloud` — Supabase (connected / full tier)

```yaml
cloud:
  url: "https://xxxx.supabase.co"
  anon_key: ""   # use: cq secret set supabase.anon_key <value>
```

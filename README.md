# CQ — AI Project Orchestration Engine

**CQ** is a project management engine for Claude Code.
It automates the full development lifecycle — planning, implementation, review, and delivery — through a structured workflow powered by C4 Engine.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Opens a new terminal and you're ready:

```sh
cq --help
```

## Tiers

Choose the tier that fits your setup:

| Tier | Description | Use when |
|------|-------------|----------|
| `solo` | Local only, no external deps | Personal / offline |
| `connected` | + Supabase, LLM Gateway, EventBus | Team / cloud sync |
| `full` | + Hub, Drive, CDP, GPU, C1 Messenger | Full production |

```sh
# Install a specific tier (default: solo)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

## Quick Start

```sh
# 1. Check environment
cq doctor

# 2. Initialize C4 in your project (generates .mcp.json + CLAUDE.md)
cd your-project
cq claude   # for Claude Code
cq cursor   # for Cursor

# 3. Open Claude Code — C4 MCP tools are now available
```

## Workflow

```
/c4-plan "feature description"   → discovery + design + tasks
/c4-run                          → spawn workers, implement in parallel
/c4-finish                       → build · test · install · commit
/c4-status                       → check progress at any time
```

## Config Templates

See `configs/` for per-tier starter configs:

```sh
cp configs/solo.yaml ~/.c4/config.yaml        # solo
cp configs/connected.yaml ~/.c4/config.yaml   # connected
cp configs/full.yaml ~/.c4/config.yaml        # full
```

## Update

Re-run the install command to update to the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Requirements

- macOS (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) installed
- `curl` available

## License

[MIT + Commons Clause](LICENSE) — free to use and modify, commercial resale prohibited.

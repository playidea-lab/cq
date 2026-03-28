# Installation Guide

Get CQ running in under 2 minutes.

## One-Line Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

This installs the `cq` binary, sets up `.mcp.json`, and initializes `.c4/`. Restart Claude Code after install — 169 MCP tools register automatically.

### Custom install directory

```sh
C4_INSTALL_DIR=/opt/cq curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

### Non-interactive (CI / headless)

```sh
C4_GLOBAL_INSTALL=y curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## What the Installer Does

1. Checks Go 1.22+, Python 3.11+, uv
2. Clones the repo (or `git pull` if already present)
3. Builds the Go binary (`c4-core/bin/cq`)
4. Installs Python dependencies (`uv sync`)
5. Merges `.mcp.json` (preserves existing entries)
6. Initializes `.c4/` directory
7. Optionally installs `~/.local/bin/cq` globally

## Prerequisites

| Item | Required | Notes |
|------|----------|-------|
| Go 1.22+ | Yes | For building the MCP server |
| Python 3.11+ | Yes | For LSP / document parsing sidecar |
| uv | Yes | Python package manager — [install](https://docs.astral.sh/uv/) |
| Claude Code | Yes | The AI tool CQ connects to |
| jq | Optional | Faster `.mcp.json` merging |

## Platform Notes

### macOS (ARM64)

Never `cp` the binary — use `go build -o` directly to preserve code signing:

```sh
# Wrong — breaks code signing
cp c4-core/bin/cq ~/.local/bin/cq

# Correct
cd c4-core && go build -o ~/.local/bin/cq ./cmd/c4/
```

### Linux (systemd)

Register CQ as a user service for auto-start on boot:

```sh
cq serve install    # Register service
cq serve status     # Check status
cq serve start      # Start
```

### Windows (WSL2)

CQ is WSL2-aware. The relay component automatically applies TCP keepalive to survive NAT timeouts common in WSL2 networking.

## Verify Installation

```sh
cq doctor
```

Expected output:

```
[✓] cq binary: v1.37
[✓] Claude Code: installed
[✓] MCP server: .mcp.json connected
[✓] .c4/ directory: initialized
```

Fix any `[✗]` items before proceeding.

## Login

Cloud sync, the Growth Loop, and the Research Loop all require authentication:

```sh
cq auth login      # GitHub OAuth via browser
cq auth status     # Verify login
```

No API keys needed — credentials are embedded in the binary at build time.

## MCP Configuration

The installer writes `.mcp.json` automatically. For reference, the structure is:

```json
{
  "mcpServers": {
    "cq": {
      "command": "/path/to/cq/c4-core/bin/cq",
      "args": ["mcp", "--dir", "/path/to/cq"],
      "env": {
        "C4_PROJECT_ROOT": "/path/to/cq"
      }
    }
  }
}
```

For global use across all projects, add to `~/.claude.json`:

```json
{
  "mcpServers": {
    "cq": {
      "command": "~/.local/bin/cq",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Code after any `.mcp.json` change.

## Update

```sh
cq update    # Pull latest binary and rebuild
```

## Uninstall

```sh
# 1. Remove "cq" entry from .mcp.json or ~/.claude.json
# 2. Optionally remove the binary
rm -f ~/.local/bin/cq
# 3. Optionally remove the source
rm -rf /path/to/cq
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| "MCP server not found" | Check binary path in `.mcp.json`; rebuild with `cd c4-core && go build -o bin/cq ./cmd/c4/` |
| macOS code signing error | Use `go build -o` directly, never `cp` |
| Python sidecar error | Run `uv sync`; verify Python 3.11+ with `python3 --version` |
| Go build fails | Run `go version` (need 1.22+); then `cd c4-core && go mod download` |
| `c4_llm_call` tool missing | Set `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` in your environment |

## Next Steps

- [Quick Start](quickstart.md) — run your first plan in 5 minutes
- [Tiers](tiers.md) — solo vs connected vs full

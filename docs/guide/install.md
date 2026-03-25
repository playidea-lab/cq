# Installation

## Step 1 — Open a terminal

::: code-group

```sh [Mac]
# Press ⌘ + Space, type "Terminal", press Enter
# Or: Applications → Utilities → Terminal
```

```sh [Windows]
# Press Start, search "Windows Terminal" or "Git Bash"
# Git Bash download: https://git-scm.com/downloads
```

```sh [Linux]
# Ctrl + Alt + T  (most distributions)
```

:::

## Step 2 — Install an AI coding assistant

CQ works with any of these. Pick one:

- **[Claude Code](https://docs.anthropic.com/en/docs/claude-code/getting-started)** — recommended
- **[Gemini CLI](https://github.com/google-gemini/gemini-cli)** — `npm install -g @google/gemini-cli`
- **[Codex CLI](https://github.com/openai/codex)** — `npm install -g @openai/codex`

## Step 3 — Install CQ

**Supported platforms**: macOS (Apple Silicon / Intel), Linux (x86\_64 / ARM64), Windows (via Git Bash)

## One-line install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

This will:
1. Detect your OS and architecture
2. Download the binary from GitHub Releases
3. Install to `~/.local/bin/cq`
4. Add `~/.local/bin` to your PATH (`.zshrc` / `.bashrc` / `.profile`)
5. Add shell completion to your RC file (`cq completion zsh/bash/fish`)
6. Run `cq doctor` to verify the environment

Open a new terminal and confirm:

```sh
cq --help
```

::: tip Auto-fix your environment
If something isn't set up correctly, run:
```sh
cq doctor --fix
```
This auto-patches CLAUDE.md, hooks, .mcp.json, and Hub auth in one command.
:::

## Step 4 — Start your first project

```sh
cd your-project-folder
cq       # auto-detects AI tool, handles login + service install
```

Then describe what you want to build. → [See examples](/examples/first-task)

::: tip Single binary
CQ ships as a single binary with all features included. No tier selection needed during install.
:::

## Install to a custom directory

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --install-dir /usr/local/bin
```

## Dry run (preview only)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --dry-run
```

## Update

Re-run the install command. The installer only replaces the binary at `~/.local/bin/cq` — your config and project data in `~/.c4/` are not modified.

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Manual install

Binaries for all platforms are available on [GitHub Releases](https://github.com/PlayIdea-Lab/cq/releases/latest).

File naming: `cq-{os}-{arch}`

| Platform | Binary |
|----------|--------|
| macOS Apple Silicon | `cq-darwin-arm64` |
| Linux x86_64 | `cq-linux-amd64` |
| Linux ARM64 | `cq-linux-arm64` |

```sh
chmod +x cq-darwin-arm64
mv cq-darwin-arm64 ~/.local/bin/cq
```

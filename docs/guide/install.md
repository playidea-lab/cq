# Installation

## Requirements

- macOS Apple Silicon (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) CLI installed — [get it here](https://docs.anthropic.com/en/docs/claude-code/getting-started)
- `curl` available in your shell

## One-line install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

This will:
1. Detect your OS and architecture
2. Download the `solo` tier binary from GitHub Releases
3. Install to `~/.local/bin/cq`
4. Add `~/.local/bin` to your PATH (`.zshrc` / `.bashrc` / `.profile`)
5. Run `cq doctor` to verify the environment

Open a new terminal and confirm:

```sh
cq --help
```

## Install a specific tier

```sh
# connected — adds Supabase, LLM Gateway, EventBus
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected

# full — all features including Hub, Drive, CDP, GPU
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

See [Tiers](/guide/tiers) for details on which tier to choose.

## Install to a custom directory

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --install-dir /usr/local/bin
```

## Dry run (preview only)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --dry-run
```

## Update

Re-run the install command. The binary is replaced in-place.

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Manual install

All binaries (3 tiers × 3 platforms = 9 files) are available on [GitHub Releases](https://github.com/PlayIdea-Lab/cq/releases/latest).

File naming: `cq-{tier}-{os}-{arch}`

| Platform | Examples |
|----------|---------|
| macOS Apple Silicon | `cq-solo-darwin-arm64`, `cq-connected-darwin-arm64`, `cq-full-darwin-arm64` |
| Linux x86_64 | `cq-solo-linux-amd64`, `cq-connected-linux-amd64`, `cq-full-linux-amd64` |
| Linux ARM64 | `cq-solo-linux-arm64`, `cq-connected-linux-arm64`, `cq-full-linux-arm64` |

```sh
chmod +x cq-solo-darwin-arm64
mv cq-solo-darwin-arm64 ~/.local/bin/cq
```

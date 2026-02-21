# Installation

## Requirements

- macOS (arm64) or Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) CLI installed
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
curl -fsSL .../install.sh | sh -s -- --tier connected

# full — all features including Hub, Drive, CDP, GPU
curl -fsSL .../install.sh | sh -s -- --tier full
```

See [Tiers](/guide/tiers) for details on which tier to choose.

## Install to a custom directory

```sh
curl -fsSL .../install.sh | sh -s -- --install-dir /usr/local/bin
```

## Dry run (preview only)

```sh
curl -fsSL .../install.sh | sh -s -- --dry-run
```

## Update

Re-run the install command. The binary is replaced in-place.

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## Manual install

Download the binary directly from [GitHub Releases](https://github.com/PlayIdea-Lab/cq/releases/latest):

| Platform | Tier | File |
|----------|------|------|
| macOS ARM | solo | `cq-solo-darwin-arm64` |
| Linux x86 | solo | `cq-solo-linux-amd64` |
| Linux ARM | solo | `cq-solo-linux-arm64` |
| macOS ARM | connected | `cq-connected-darwin-arm64` |
| Linux x86 | full | `cq-full-linux-amd64` |

```sh
chmod +x cq-solo-darwin-arm64
mv cq-solo-darwin-arm64 ~/.local/bin/cq
```

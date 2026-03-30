# Quick Start

GPU Anywhere, Anytime, Anything — from install to first result in 2 minutes.

## 1. Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Expected output:

```
Downloading cq v0.72.0 for darwin/arm64...
Installing to /usr/local/bin/cq
cq installed successfully.

Run: cq auth login
```

Verify:

```sh
cq --version
# cq v0.72.0 (darwin/arm64)
```

> **Linux / WSL2**: The same one-liner works. cq is a single static binary with no runtime dependencies.

---

## 2. Login

```sh
cq auth login
```

This opens your browser for GitHub OAuth. After approval, you'll see:

```
Logged in as changmin@pilab.co.kr
Relay connected: relay.pilab.kr:443
```

> **Free tier**: You can skip login and use CQ locally with your own API keys. See [Tiers](/guide/tiers) for differences.

---

## 3. Connect to Your AI

CQ works through MCP (Model Context Protocol). Add it to Claude Code:

**Option A — Claude Code CLI (recommended)**

```sh
cq mcp install claude-code
```

Expected output:

```
Writing /Users/you/project/.mcp.json
MCP server registered: cq
Restart Claude Code to activate.
```

**Option B — Manual `.mcp.json`**

```json
{
  "mcpServers": {
    "cq": {
      "command": "/usr/local/bin/cq",
      "args": ["mcp", "serve"],
      "env": {}
    }
  }
}
```

Place this in your project root or `~/.claude/.mcp.json` for global access.

After restarting Claude Code, you should see CQ in the tools list. Run a quick check:

```sh
cq doctor
```

```
[OK] Binary: /usr/local/bin/cq v0.72.0
[OK] Auth: logged in as changmin@pilab.co.kr
[OK] Relay: connected (latency 12ms)
[OK] MCP: registered in .mcp.json
[OK] Knowledge: 0 records (fresh start)
```

---

## 4. First Commands

### Search your knowledge base

```sh
cq knowledge search "GPU setup"
```

On a fresh install this returns nothing — your knowledge base grows as you work. After a few sessions:

```
[1] GPU worker setup (2026-03-28) — score 0.91
    "Connect RTX 4090 via cq serve, latency 8ms. Use cq gpu status to verify."
[2] CUDA environment (2026-03-15) — score 0.87
    "Always run nvidia-smi before submitting jobs. Driver 525+ required."
```

### Test the relay

```sh
cq relay ping
```

```
Relay: relay.pilab.kr:443
Latency: 11ms
E2E encryption: yes (ECDH + AES-256-GCM)
Your relay address: usr-changmin-a3f8.relay.pilab.kr
```

### Ask Claude to do something

Inside Claude Code (or any MCP-connected AI), CQ tools are now available. Try:

```
"Search my knowledge base for anything about authentication patterns"
```

Claude calls `cq_knowledge_search` automatically and injects results into context.

```
Found 3 records:
- JWT validation: always verify signature, never trust alg:none (2026-03-10)
- Auth middleware pattern: use context.WithValue for user propagation (2026-03-18)
- OAuth flow: state param required to prevent CSRF (2026-03-22)
```

### Record something

```
"Save a note: prefer table-driven tests in Go, always name subtests descriptively"
```

Claude calls `cq_knowledge_record`. Next session, this preference surfaces automatically.

---

## 5. What's Happening Behind the Scenes

Every session, CQ:

1. **Captures** decisions, corrections, and experiment results
2. **Extracts** preferences from what you corrected or repeated
3. **Promotes** patterns that appear 3+ times into hints, 5+ times into permanent rules
4. **Syncs** across all your AI tools — learn in Claude, use in ChatGPT

By session 5, CQ knows how you work without being told. By session 30, it codes like you.

See [Knowledge Loop](/guide/growth-loop) for the full picture.

---

## What Next?

| I want to... | Go to |
|-------------|-------|
| Connect a GPU server for remote training | [Worker Setup](/guide/worker-setup) |
| Understand Free / Pro / Team differences | [Tiers](/guide/tiers) |
| See how knowledge accumulates over time | [Knowledge Loop](/guide/growth-loop) |
| Set up E2E encrypted relay (NAT traversal) | [Relay](/guide/relay) |
| Connect ChatGPT to my CQ workspace | [Remote Brain](/guide/remote-brain) |
| Run GPU experiments autonomously | [Worker Guide](/guide/worker) |

---

## Troubleshooting

```sh
cq doctor    # Diagnose issues automatically
```

| Symptom | Fix |
|---------|-----|
| "MCP server not found" | Run `cq mcp install claude-code` or check binary path in `.mcp.json` |
| Login fails / browser doesn't open | Run `cq auth login --no-browser` and paste the URL manually |
| Relay connection refused | Check firewall: outbound TCP 443 must be open |
| `cq doctor` shows auth error after update | Run `cq auth login` again — token may have expired |
| macOS code signing error | Install via the curl one-liner, not `cp` from another machine |

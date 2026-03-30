# Remote AI Workspace

Connect any MCP-compatible AI — ChatGPT, Claude Desktop, Cursor — to your CQ knowledge base via [mcp.pilab.kr](https://mcp.pilab.kr).

## Why Remote AI Workspace

CQ installed locally gives you 169 MCP tools and GPU access. The Remote AI Workspace gives you the **knowledge layer** — accessible from any AI tool, on any device, without a local install.

- Knowledge recorded in Claude Code is available in ChatGPT
- Experiment results from your GPU server sync to your desktop
- Any AI can read and write to your shared workspace

## Connect in 2 Steps

### Step 1: Log in at mcp.pilab.kr

Visit [https://mcp.pilab.kr](https://mcp.pilab.kr) and log in with GitHub. This creates your OAuth token.

### Step 2: Add the MCP server to your AI tool

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

That's it. GitHub OAuth handles authentication — the URL is the same for everyone; access is controlled by your token.

## AI Tool Setup

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

Restart Claude Desktop.

### ChatGPT (Custom GPT / Connector)

In ChatGPT's MCP connector settings, add:

```
URL: https://mcp.pilab.kr/mcp
Auth: OAuth 2.1 (GitHub)
```

### Cursor

Add to `.cursor/mcp.json` in your project or `~/.cursor/mcp.json` globally:

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

## What the Remote AI Workspace Can Do

The Remote MCP server exposes a subset of CQ tools focused on knowledge:

| Tool | Description |
|------|-------------|
| `cq_knowledge_record` | Save a discovery, experiment result, or decision |
| `cq_knowledge_search` | Search your knowledge base |
| `cq_session_summary` | Get a summary of recent sessions |
| `cq_preferences_list` | View your accumulated preferences |

These tools work identically across Claude Code, ChatGPT, and Cursor — same data, same backend.

## AI Self-Capture

The Remote AI Workspace is engineered so AI tools **proactively** save knowledge without you asking.

Tool descriptions are written to trigger automatic saves:

- When an AI discovers a bug root cause → `cq_knowledge_record` fires automatically
- When a solution to a non-obvious problem is found → saved immediately
- When a pattern emerges across multiple files → recorded with context

You don't need to prompt "save this" — the AI does it when it recognizes something worth keeping.

## Session Summary

When a session ends, the AI can call `cq_session_summary` to capture:

- Key decisions made
- Preferences expressed
- Problems solved and how
- Files changed and why

This feeds directly into the [Knowledge Loop](growth-loop.md) — preferences extracted from summaries accumulate into hints and rules.

## OAuth Flow

```
Your AI tool → mcp.pilab.kr/mcp
                     │
               OAuth 2.1 (GitHub)
                     │
               Validate token
                     │
               Route to your Supabase namespace
                     │
               Return knowledge results
```

The Cloudflare Worker (`mcp.pilab.kr`) acts as an OAuth 2.1 proxy. It validates your GitHub token, identifies your user namespace in Supabase, and proxies knowledge operations. No CQ binary required on the remote machine.

## Cross-Platform Knowledge Sync

When you record knowledge in any tool, it's immediately available in all others:

```
Claude Code session:  discovers caching bug root cause
  → cq_knowledge_record("Redis TTL not set for anonymous sessions")

Same day, ChatGPT:    "why does the cache behave inconsistently?"
  → cq_knowledge_search("cache") returns the earlier discovery
```

No copy-paste. No re-explanation. Knowledge follows you.

## Requirements

- CQ account (free — sign up at [mcp.pilab.kr](https://mcp.pilab.kr))
- GitHub account for OAuth
- Any MCP-compatible AI tool

No local CQ installation required for the Remote AI Workspace connection.

## Relationship to Local Installation

The Remote AI Workspace and local CQ use the **same Supabase backend**. They are not separate systems:

| Feature | Local CQ | Remote AI Workspace |
|---------|----------|---------------------|
| Task orchestration | Yes | No |
| GPU job execution | Yes | No |
| File access | Yes | No |
| Knowledge read/write | Yes | Yes |
| Knowledge Loop | Yes | Yes (via summary) |
| Setup required | Install + build | OAuth login |

Use local CQ for building and GPU experiments. Use the Remote AI Workspace for knowledge access from tools where you can't install software.

## Next Steps

- [Knowledge Loop](growth-loop.md) — how knowledge accumulates into preferences and rules
- [Tiers](tiers.md) — Remote AI Workspace is available in Pro and Team tiers

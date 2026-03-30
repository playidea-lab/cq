# Connect Remote MCP

Access your CQ knowledge base from any MCP-compatible AI — ChatGPT, Claude Desktop, Cursor — without installing CQ locally.

---

## What You'll Do

Connect ChatGPT (or any MCP client) to your CQ AI Workspace via `mcp.pilab.kr`. After setup, you can search knowledge, save snapshots, and check project status from anywhere.

---

## Step 1: Log In

Visit [mcp.pilab.kr](https://mcp.pilab.kr) and log in with GitHub. This creates your OAuth token.

```sh
cq auth login    # If you haven't already
```

---

## Step 2: Add MCP Server to Your AI Tool

### Claude Desktop / Claude Code

Add to your MCP config:

```json
{
  "cq-brain": {
    "url": "https://mcp.pilab.kr/mcp",
    "type": "streamable-http"
  }
}
```

### ChatGPT

In ChatGPT settings → MCP Servers → Add:

```
URL: https://mcp.pilab.kr/mcp
```

ChatGPT will walk through OAuth — log in with GitHub when prompted.

### Cursor

In `.cursor/mcp.json`:

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

---

## Step 3: Use It

Once connected, your AI has access to these tools:

| Tool | What it does |
|------|-------------|
| `cq_snapshot` | Save a conversation snapshot to your knowledge base |
| `cq_recall` | Search your knowledge base |
| `cq_status` | Check project status |

### Example: Search knowledge from ChatGPT

```
"Search my CQ knowledge for WebSocket retry patterns"
```

ChatGPT calls `cq_recall` → returns your past decisions and code patterns.

### Example: Save an idea from Claude Desktop

```
"Save this conversation as a snapshot — we figured out the caching strategy"
```

Claude calls `cq_snapshot` → stored in Supabase, searchable from any tool.

---

## How It Works

```
Your AI tool → mcp.pilab.kr/mcp
                    │
                    ▼
             Cloudflare Worker (OAuth proxy)
                    │
                    ▼
             Supabase Edge Function (MCP server)
                    │
                    ▼
             Your knowledge base (Supabase DB)
```

No CQ binary needed on the remote machine. The AI Workspace lives in the cloud.

---

## Next Steps

- [ChatGPT → Claude workflow](chatgpt-to-claude.md) — start ideas in ChatGPT, implement in Claude
- [Knowledge Loop](growth-loop-in-action.md) — see how preferences evolve across sessions

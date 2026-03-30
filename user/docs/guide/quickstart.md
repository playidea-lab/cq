# Quick Start

From install to your first result in 2 minutes.

## 1. Install

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 2. Launch

```sh
cq
```

CQ auto-detects your AI tool (Claude Code, Cursor, Codex, Gemini) and connects.

## 3. Do Something

Just talk. CQ auto-routes based on what you ask:

### Small — Direct Fix (30 seconds)

```
"Fix the typo in auth/handler.go line 42"
```

CQ handles it directly. No planning, no workers. Just a fix.

### Medium — Quick Task (2 minutes)

```
/quick "add a health check endpoint to the API"
```

CQ creates a task with Definition of Done, spawns a worker, and submits the result. One command.

### Large — Full Pipeline (5+ minutes)

```
/pi "build a webhook delivery system with retry logic"
```

CQ brainstorms → plans → spawns parallel workers → polishes → commits. You drink coffee.

---

## That's It

There's no step 4. CQ figures out the right workflow for each request.

**What happens behind the scenes:**
- Every session, CQ captures your decisions and preferences
- By session 5, it knows how you work without being told
- Knowledge flows across AI tools — learn in ChatGPT, use in Claude

---

## What Next?

| I want to... | Go to |
|-------------|-------|
| Understand the 4 pillars (Distribute/Connect/Mimic/Evolve) | [Home](/) |
| See a real bug fix walkthrough | [Bug Fix Example](/examples/bug-fix) |
| Build a large feature with planning | [Feature Planning](/examples/feature-planning) |
| Connect ChatGPT to my CQ brain | [Remote MCP](/examples/remote-mcp) |
| Watch CQ learn my preferences | [Growth Loop](/examples/growth-loop-in-action) |
| Run GPU experiments autonomously | [Research Loop](/examples/research-loop) |

## Troubleshooting

```sh
cq doctor    # Check what's wrong
```

| Symptom | Fix |
|---------|-----|
| "MCP server not found" | Check binary path in `.mcp.json`; run `cq doctor` |
| macOS code signing error | Use `go build -o` directly, never `cp` |
| Python sidecar error | Run `uv sync`; verify Python 3.11+ |

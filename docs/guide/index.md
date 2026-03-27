# What is CQ?

**CQ** is an external brain for AI — persistent memory, quality gates, and distributed execution that works across ChatGPT, Claude Code, Cursor, Codex CLI, and Gemini.

## Core capabilities

### External Brain — Memory across all AI platforms

Every AI conversation contributes to your knowledge base. ChatGPT discovers a bug root cause? Claude picks it up in the next session. CQ uses tool description engineering so AI **proactively** saves knowledge without being asked.

- **Cross-platform memory** — ChatGPT, Claude, Gemini, Codex all share the same brain
- **Session summary** — Automatic end-of-conversation capture as a safety net
- **Vector + text search** — pgvector embeddings + FTS + ilike fallback
- **Remote MCP access** — OAuth 2.1 via Cloudflare Worker, no local install needed

### Project orchestration

- **Tasks** have a Definition of Done and are tracked in cloud (Supabase) or local SQLite
- **Workers** are AI agent instances that each handle one task in an isolated git worktree
- **Reviews** are automatic — 6-axis quality gates (correctness, security, reliability, observability, testing, readability)
- **Knowledge** is recorded from every completed task and used to improve future ones

### Distributed execution

- **Hub** — Distributed job queue with DAG support, artifact auto-upload, cron scheduling
- **Relay** — WebSocket NAT traversal (Fly.io), WSL2-aware, TCP keepalive
- **Drive** — Cloud file storage with TUS resumable upload and dataset versioning
- **File Index** — Search files across all connected devices

## How it works

```
You describe a feature → /plan creates tasks with DoD
                       → /run spawns workers (one per task)
                       → each worker: implement → test → submit
                       → reviewer worker checks the output
                       → build, test, commit
```

CQ uses the **Model Context Protocol (MCP)** — your AI tool talks to CQ via 169+ tools. Only ~40 core tools are shown by default; set `CQ_TOOL_TIER=full` to expose all of them.

CQ auto-routes requests by size: **Small** (direct edit), **Medium** (`/quick`), **Large** (`/pi` → plan → run → finish).

## What CQ is not

- Not an AI model — it orchestrates AI coding tools
- Not a code generator — workers write code, CQ manages the process
- Not opinionated about your stack — works with any language or framework

## Next

- [Install CQ →](/guide/install)

# Tutorial: Your First 5 Minutes with CQ

> For researchers new to AI-assisted development.
> No ML/DL knowledge required — just a terminal and a project.

## What you'll do

1. Install CQ (30 seconds)
2. Start it in your project (10 seconds)
3. Ask it to do something (your first task)
4. See the result

## Step 1: Install

Open a terminal and paste:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Close and reopen your terminal (or run `source ~/.zshrc`).

## Step 2: Start CQ in your project

```sh
cd ~/your-project    # any project with code
cq
```

You'll see:

```
CQ ready.
```

That's it. CQ has:
- Connected to Claude Code
- Set up a knowledge base for your project
- Started a background service for remote workers
- Created `CLAUDE.md` with project instructions

## Step 3: Try something

Now you're in Claude Code with CQ. Just describe what you want:

### Small request (direct edit)

```
README.md에 프로젝트 설명 추가해줘
```

CQ recognizes this as a small task and edits the file directly. No overhead.

### Medium request (one worker)

```
이 코드 리뷰해줘
```

CQ runs a 6-axis code review: correctness, security, reliability, observability, test coverage, readability.

### Large request (full pipeline)

```
로그인 기능 만들어줘
```

CQ automatically runs the full pipeline:

```
/pi (brainstorm) → /c4-plan (design) → /c4-run (implement) → /c4-finish (verify)
```

Multiple workers execute tasks in parallel. Each gets its own git branch. Every implementation is reviewed before merging.

## Step 4: Check what happened

```
/c4-status
```

Shows your tasks, their status, and what's ready next.

## What CQ does behind the scenes

| You see | CQ does |
|---------|---------|
| "CQ ready." | Initializes project, connects MCP server, starts background service |
| You type a request | Auto-routes to Small/Medium/Large workflow |
| Workers run | Each worker: isolated git worktree, one task, fresh context |
| Code is submitted | 6-axis review runs automatically |
| Session ends | Knowledge recorded, patterns learned for next time |

## Key commands

| Command | What it does |
|---------|-------------|
| `/pi` | Brainstorm and explore an idea |
| `/c4-plan` | Plan a feature with tasks |
| `/c4-run` | Start parallel workers |
| `/c4-status` | Check progress |
| `/c4-quick` | Quick one-off task |

## For teams

When a teammate clones your repo, they get the same `.mcp.json` and `CLAUDE.md`. They just need to:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq
```

CQ's knowledge base is shared — what one person learns, everyone benefits from.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `cq: command not found` | Reopen terminal or `source ~/.zshrc` |
| MCP tools not loading | Run `cq doctor --fix` |
| Worker not connecting | Check `cq serve status` |
| Need help | [Ask on GitHub Discussions](https://github.com/PlayIdea-Lab/cq/discussions) |

## Next

- [Full workflow guide →](/workflow/)
- [Remote GPU worker setup →](/guide/worker-setup)
- [Report an issue →](https://github.com/PlayIdea-Lab/cq/issues)

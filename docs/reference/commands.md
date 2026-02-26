# Commands Reference

## cq CLI

### `cq claude` / `cq cursor` / `cq codex`

Initialize CQ in the current project for a specific AI agent.

```sh
cq claude    # Claude Code
cq cursor    # Cursor
cq codex     # Codex CLI
```

Creates `.mcp.json`, `CLAUDE.md`, `.c4/` directory, and skill symlinks.

### `cq doctor`

Check environment health (8 items).

```sh
cq doctor           # full report
cq doctor --json    # JSON output (for CI)
cq doctor --fix     # auto-fix safe issues
```

| Check | What it verifies |
|-------|-----------------|
| cq binary | Binary exists + version |
| .c4 directory | Database files present |
| .mcp.json | Valid JSON + binary path exists |
| CLAUDE.md | File exists + symlink valid |
| hooks | Security hook installed |
| Python sidecar | `uv` available |
| C5 Hub | Hub config + health endpoint |
| Supabase | Cloud config + connection |

### `cq secret`

Manage API keys and secrets (stored in `~/.c4/secrets.db`, AES-256-GCM).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret get anthropic.api_key
cq secret list
cq secret delete anthropic.api_key
```

Keys are never stored in config files.

### `cq auth`

Authenticate with C4 Cloud (GitHub OAuth).

```sh
cq auth login    # Open browser for GitHub OAuth flow
cq auth logout   # Clear stored credentials (~/.c4/session.json)
cq auth status   # Show current authentication status
```

### `cq ls`

List named Claude Code sessions (tmux-style).

```sh
cq ls            # Show all named sessions with their UUIDs
```

### `cq session`

Manage named Claude Code sessions.

```sh
cq session name <session-name>   # Attach a name to the current session
cq session rm <session-name>     # Remove a named session
```

Sessions can be resumed with `cq claude -t <session-name>`.

### `cq mail`

Inter-session mail for passing messages between Claude Code sessions.

```sh
cq mail send <to> <body>   # Send a message to a named session
cq mail ls                 # List messages (shows unread count)
cq mail read <id>          # Read a message (marks as read)
cq mail rm <id>            # Delete a message
```

### `cq serve`

Run background services (EventBus, GPU scheduler, Agent listener).

```sh
cq serve              # start on :4140
cq serve --port 4141
cq serve install      # Install as OS service (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall    # Uninstall the OS service
cq serve status       # Show OS service status and manual serve process status
```

### `cq version`

Print the current binary version and build tier.

---

## Skills (Claude Code slash commands)

Skills are invoked inside Claude Code as `/skill-name`.

| Skill | Trigger | Description |
|-------|---------|-------------|
| `/c4-plan` | "계획", "plan", "설계" | Discovery → Design → Tasks |
| `/c4-run` | "실행", "run", "ㄱㄱ" | Spawn workers for pending tasks |
| `/c4-finish` | "마무리", "finish" | Build → test → docs → commit |
| `/c4-status` | "상태", "status" | Visual task progress |
| `/c4-quick` | "quick", "빠르게" | Single task, no planning |
| `/c4-polish` | "polish" | Build-test-review-fix loop until zero changes. Called automatically by `/c4-run`. |
| `/c4-refine` | "refine" | *(Deprecated — plan critique is now built into `/c4-plan`)* |
| `/c4-checkpoint` | checkpoint reached | Approve / request changes / replan |
| `/c4-validate` | "검증", "validate" | Run lint + tests |
| `/c4-review` | "review" | 3-pass code review with 6-axis evaluation |
| `/c4-add-task` | "태스크 추가" | Add task interactively |
| `/c4-submit` | "제출", "submit" | Submit completed task |
| `/c4-interview` | "interview" | Deep requirements interview |
| `/c4-stop` | "stop", "중단" | Halt execution, preserve progress |
| `/c4-clear` | "clear" | Reset C4 state for debugging |
| `/c4-swarm` | "swarm" | Spawn coordinator-led agent team |
| `/c4-standby` | "대기", "standby" | Become a C5 Hub worker (full tier) |
| `/c4-init` | "init", "초기화" | Initialize C4 in current project |
| `/c4-release` | "release" | Generate CHANGELOG from git history |
| `/c4-help` | "help" | Quick reference for all skills |
| `/c2-paper-review` | "논문 리뷰", "paper review" | Academic paper review (C2 lifecycle) |
| `/research-loop` | "research loop" | Paper-experiment improvement loop |

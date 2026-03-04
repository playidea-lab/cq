# Commands Reference

## cq CLI

### `cq claude` / `cq cursor` / `cq codex`

Initialize CQ in the current project for a specific AI coding tool.

```sh
cq claude    # Claude Code
cq cursor    # Cursor
cq codex     # OpenAI Codex CLI
cq gemini    # Gemini CLI
```

Each command creates `CLAUDE.md`, `.c4/`, skills, and the MCP config for that tool:

| Command | MCP config | Agent instructions |
|---------|-----------|-------------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI config) | `CLAUDE.md` |

> Any tool supporting the [AGENTS.md standard](https://agents.md) (e.g. Gemini Code Assist) can read `CLAUDE.md` directly without a dedicated `cq` command.

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

List named Claude Code sessions with columnar output.

```sh
cq ls
```

Example output:

```
● my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  ✉2
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    Analyzing training data pipeline
```

- `●` marks the current active session
- `✉N` shows unread inter-session mail count
- Memo (if set) is displayed on the line below

### `cq session`

Manage named Claude Code sessions.

```sh
cq session name <session-name>              # Attach a name to the current session
cq session name <session-name> -m "memo"   # Attach name with memo
cq session rm <session-name>               # Remove a named session
```

Sessions can be resumed with `cq claude -t <session-name>`.

Use `/c4-attach` inside Claude Code to name a session without leaving the editor.

### `cq mail`

Inter-session mail for passing messages between Claude Code sessions.

```sh
cq mail send <to> <body>   # Send a message to a named session
cq mail ls                 # List messages (shows unread count)
cq mail read <id>          # Read a message (marks as read)
cq mail rm <id>            # Delete a message
```

### `cq serve`

Run background services (EventBus, GPU scheduler, Agent listener, C5 Hub).

```sh
cq serve              # start on :4140
cq serve --port 4141
cq serve install      # Install as OS service (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall    # Uninstall the OS service
cq serve status       # Show OS service status and manual serve process status
```

Health endpoint: `GET http://127.0.0.1:4140/health` — returns status of all registered components.

**Hub component** (`full` tier): when `serve.hub.enabled: true` in config, `cq serve` automatically starts the `c5` binary as a managed subprocess. No separate process management needed.

```sh
# Check component health (hub + any others)
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"hub":{"status":"ok","detail":"port 8585"}}}
```

### `cq version`

Print the current binary version and build tier.

### `cq pop`

Personal Ontology Pipeline status and control.

```sh
cq pop status   # Show gauge values, pipeline state, and knowledge stats
```

---

## Skills (Claude Code slash commands)

Skills are invoked inside Claude Code as `/skill-name`.

| Skill | Trigger | Description |
|-------|---------|-------------|
| `/pi` | "play idea", "아이디어", "ideation" | Brainstorm before planning; auto-launches `/c4-plan` |
| `/c4-plan` | "계획", "plan", "설계" | Discovery → Design → Tasks |
| `/c4-run` | "실행", "run", "ㄱㄱ" | Spawn workers for pending tasks |
| `/c4-finish` | "마무리", "finish" | Build → test → docs → commit |
| `/c4-status` | "상태", "status" | Visual task progress |
| `/c4-quick` | "quick", "빠르게" | Single task, no planning |
| `/c4-polish` | "polish" | *(Deprecated — built into `/c4-finish`)* |
| `/c4-refine` | "refine" | *(Deprecated — built into `/c4-finish`)* |
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
| `/c4-attach` | "세션 이름", "attach" | Name the current session for later resume |
| `/c4-reboot` | "reboot", "재시작" | Reboot the current named session |
| `/c4-init` | "init", "초기화" | Initialize C4 in current project |
| `/c4-release` | "release" | Generate CHANGELOG from git history |
| `/c4-help` | "help" | Quick reference for all skills |
| `/c2-paper-review` | "논문 리뷰", "paper review" | Academic paper review (C2 lifecycle) |
| `/research-loop` | "research loop" | Paper-experiment improvement loop |
| `/c9-init` | "c9-init" | Initialize C9 ML research project |
| `/c9-loop` | "c9-loop" | Main loop driver for ML research |
| `/c9-run` | "c9-run" | Submit experiments to C5 Hub |
| `/c9-check` | "c9-check" | Parse results + convergence check |
| `/c9-standby` | "c9-standby" | Wait mode, auto-triggers CHECK |
| `/c9-finish` | "c9-finish" | Save best model + document |
| `/c9-steer` | "c9-steer" | Phase transition without editing YAML |
| `/c9-survey` | "c9-survey" | Survey arXiv + SOTA with Gemini |
| `/c9-report` | "c9-report" | Collect remote experiment results |
| `/c9-conference` | "c9-conference" | Claude + Gemini research debate |
| `/c9-deploy` | "c9-deploy" | Deploy best model to edge server |

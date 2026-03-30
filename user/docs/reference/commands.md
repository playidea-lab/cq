# Commands Reference

## cq CLI

> Current version: **v1.46**

### `cq` / `cq claude`

Launch Claude Code with CQ project initialization.

```sh
cq                          # Launch claude (no telegram)
cq -t <name>                # Named session (fixed UUID via --session-id)
cq --bot                    # Show bot menu → connect telegram
cq --bot <name>             # Connect specific telegram bot
cq -t <name> --bot <name>   # Named session + telegram
```

### Other AI tools

```sh
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

Check environment health (13 items).

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
| hooks | Gate + permission-reviewer hooks installed |
| Python sidecar | `uv` available |
| Supabase Worker Queue | Supabase connection + worker queue health |
| Supabase | Cloud config + connection |
| os-service | LaunchAgent / systemd service installed and running |
| tool-socket | UDS socket responsive (`cq serve` running) |
| zombie-serve | No orphaned serve processes |
| sidecar | Python sidecar not hung |
| skill-health | All evaluated skills pass trigger threshold (≥ 0.90) |

### `cq update`

Self-update CQ to the latest release.

```sh
cq update           # Check and install the latest version
cq update --check   # Check only, do not install
```

### `cq secret`

Manage API keys and secrets (stored in `~/.c4/secrets.db`, AES-256-GCM).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret set hub.api_key <your-hub-key>   # Worker queue API key (preferred over config.yaml)
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

List registered Telegram bots.

```sh
cq ls
```

### `cq sessions`

List named Claude Code sessions.

```sh
cq sessions
```

Example output:

```
  my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  ✉2
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    Analyzing training data pipeline
```

- `✉N` shows unread inter-session mail count
- Memo (if set) is displayed on the line below

### `cq session`

Manage named Claude Code sessions.

```sh
cq session name <session-name>              # Attach a name to the current session
cq session name <session-name> -m "memo"   # Attach name with memo
cq session rm <session-name>               # Remove a named session
```

Sessions can be resumed with `cq -t <session-name>`.

Use `/c4-attach` inside Claude Code to name a session without leaving the editor.

### `cq mail`

Inter-session mail for passing messages between Claude Code sessions.

```sh
cq mail send <to> <body>   # Send a message to a named session
cq mail ls                 # List messages (shows unread count)
cq mail read <id>          # Read a message (marks as read)
cq mail rm <id>            # Delete a message
```

### `cq jobs`

BubbleTea TUI for Hub job monitoring.

```sh
cq jobs          # Open interactive job monitor
cq jobs --watch  # Auto-refresh mode
```

Features: detail panel with per-job metrics, adaptive multi-row charts, compare mode for side-by-side job diffs.

### `cq workers`

Worker Connection Board TUI — view and manage connected Hub workers.

```sh
cq workers       # Open worker board
```

Shows worker status, heartbeat, current job assignment, and resource utilization per worker.

### `cq dashboard`

Unified TUI dashboard with board menu — entry point for all monitoring views.

```sh
cq dashboard     # Open dashboard (board menu → jobs / workers / pop / sessions)
```

### `cq setup`

Pair a Telegram bot for notifications.

```sh
cq setup
```

Walks through Telegram bot pairing: enter your bot token, start the bot, and CQ auto-detects your chat ID. After setup, CQ sends experiment and task notifications via Telegram.

### `cq serve`

Run background services (EventBus, GPU scheduler, Agent listener, Relay).

```sh
cq serve                  # start on :4140
cq serve --port 4141
cq serve --watchdog       # OS service with watchdog: auto-restarts relay on failure, heartbeat self-healing
cq serve install          # Install as OS service (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall        # Uninstall the OS service
cq serve status           # Show OS service status and manual serve process status
```

When hub is enabled in config, the worker is automatically enabled alongside the hub.

Health endpoint: `GET http://127.0.0.1:4140/health` — returns status of all registered components.

```sh
# Check component health
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"eventbus":{"status":"ok"}}}
```

### `cq pop`

Personal Ontology Pipeline status and control.

```sh
cq pop status   # Show gauge values, pipeline state, and knowledge stats
```

### `cq craft`

Skill Marketplace — publish, search, and install community skills.

```sh
cq craft publish   # Publish a skill to the marketplace
cq craft search    # Search available skills
cq craft install   # Install a skill from the marketplace
```

### `cq version`

Print the current binary version and build tier.

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
| `/company-review` | "PR 리뷰", "diff 리뷰" | PI Lab 표준 코드 리뷰 |
| `/c4-submit` | "제출", "submit" | Submit completed task |
| `/simplify` | "simplify", "단순화" | Review changed code for quality and efficiency |
| `/c4-add-task` | "태스크 추가" | Add task interactively |
| `/c4-stop` | "stop", "중단" | Halt execution, preserve progress |
| `/c4-clear` | "clear" | Reset C4 state for debugging |
| `/c4-swarm` | "swarm" | Spawn coordinator-led agent team |
| `/c4-standby` | "대기", "standby" | Become a distributed worker via Supabase (full tier) |
| `/done` | "done", "세션 종료" | Mark session done with full capture |
| `/c4-attach` | "세션 이름", "attach" | Name the current session for later resume |
| `/c4-reboot` | "reboot", "재시작" | Reboot the current named session |
| `/session-distill` | "session distill", "세션 요약" | Distill session into persistent knowledge |
| `/init` | "init", "초기화" | Initialize C4 in current project |
| `/c4-release` | "release" | Generate CHANGELOG from git history |
| `/c4-help` | "help" | Quick reference for all skills |
| `/claude-md-improver` | "CLAUDE.md 개선" | Analyze and improve CLAUDE.md |
| `/skill-tester` | "skill tester", "스킬 테스트" | Test and evaluate skill quality |
| `/pr-review` | "PR 만들어", "PR 체크리스트" | PR/MR creation checklist and review guide |
| `/craft` | "craft", "스킬 만들어줘" | Interactively create skills, agents, rules |
| `/tdd-cycle` | "TDD", "RED-GREEN-REFACTOR" | TDD cycle guide |
| `/debugging` | "debugging", "디버깅" | Systematic debugging workflow |
| `/spec-first` | "spec-first", "설계 문서" | Spec-First development guide |
| `/incident-response` | "incident", "장애", "서버 다운" | Production incident response workflow |
| `/c2-paper-review` | "논문 리뷰", "paper review" | Academic paper review (deprecated) |
| `/research-loop` | "research loop" | Paper-experiment improvement loop |
| `/experiment-workflow` | "experiment workflow" | End-to-end experiment lifecycle |
| `/c9-init` | "c9-init" | Initialize C9 ML research project |
| `/c9-loop` | "c9-loop" | Main loop driver for ML research |
| `/c9-survey` | "c9-survey" | Survey arXiv + SOTA with Gemini |
| `/c9-conference` | "c9-conference" | Claude + Gemini research debate |
| `/c9-steer` | "c9-steer" | Phase transition without editing YAML |
| `/c9-report` | "c9-report" | Collect remote experiment results |
| `/c9-finish` | "c9-finish" | Save best model + document |
| `/c9-deploy` | "c9-deploy" | Deploy best model to edge server |

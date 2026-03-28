# Commands Reference

CQ provides a CLI (`cq`) for project setup, session management, and environment control, plus Skills (slash commands) used inside Claude Code and compatible AI tools.

---

## cq CLI

### `cq` / `cq claude`

Launch Claude Code with CQ project initialization.

```sh
cq                          # Launch claude (no telegram)
cq -t <name>                # Named session (fixed UUID via --session-id)
cq --bot                    # Show bot menu → connect telegram
cq --bot <name>             # Connect specific telegram bot
cq -t <name> --bot <name>   # Named session + telegram
```

### Other AI Tools

```sh
cq cursor    # Launch Cursor
cq codex     # Launch OpenAI Codex CLI
cq gemini    # Launch Gemini CLI
```

Each command creates `CLAUDE.md`, `.c4/`, skills, and the MCP config for that tool:

| Command | MCP config | Agent instructions |
|---------|-----------|-------------------|
| `cq claude` | `.mcp.json` | `CLAUDE.md` |
| `cq cursor` | `.cursor/mcp.json` | `CLAUDE.md` |
| `cq codex` | `~/.codex/config.toml` | `.codex/agents/` |
| `cq gemini` | (Gemini CLI config) | `CLAUDE.md` |

> Any tool supporting the [AGENTS.md standard](https://agents.md) (e.g. Gemini Code Assist) can read `CLAUDE.md` directly without a dedicated `cq` command.

---

### `cq auth`

Authenticate with CQ Cloud (GitHub OAuth).

```sh
cq auth login            # Open browser for GitHub OAuth flow
cq auth login --device   # Headless/SSH: show user_code → enter in browser (RFC 8628)
cq auth login --link     # Print auth URL directly → open manually
cq auth logout           # Clear stored credentials (~/.c4/session.json)
cq auth status           # Show current authentication status
```

---

### `cq secret`

Manage API keys and secrets (stored in `~/.c4/secrets.db`, AES-256-GCM).

```sh
cq secret set anthropic.api_key sk-ant-...
cq secret set openai.api_key sk-...
cq secret set hub.api_key <your-hub-key>   # Worker queue API key
cq secret get anthropic.api_key
cq secret list
cq secret delete anthropic.api_key
```

Keys are never stored in config files.

---

### `cq doctor`

Check environment health (13 items).

```sh
cq doctor           # Full report
cq doctor --json    # JSON output (for CI)
cq doctor --fix     # Auto-fix safe issues
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
| skill-health | All evaluated skills pass trigger threshold (>= 0.90) |

---

### `cq serve`

Run background services (EventBus, GPU scheduler, Agent listener).

```sh
cq serve              # Start on :4140
cq serve --port 4141  # Custom port
cq serve install      # Install as OS service (macOS LaunchAgent / Linux systemd / Windows Service)
cq serve uninstall    # Uninstall the OS service
cq serve start        # Start the OS service
cq serve stop         # Stop the OS service
cq serve status       # Show OS service status and manual serve process status
```

Health endpoint: `GET http://127.0.0.1:4140/health` — returns status of all registered components.

```sh
curl http://127.0.0.1:4140/health
# {"status":"ok","components":{"eventbus":{"status":"ok"}}}
```

Components managed by `cq serve`:

| Component | Config key | Description |
|-----------|-----------|-------------|
| eventbus | `serve.eventbus.enabled` | C3 gRPC EventBus daemon (UDS) |
| gpu | `serve.gpu.enabled` | GPU/CPU job scheduler |
| agent | `serve.agent.enabled` | Supabase Realtime @cq mention dispatcher |
| hub | `serve.hub.enabled` | Hub distributed job queue poller |
| relay | `relay.enabled` | WebSocket relay client (NAT traversal) |
| stale_checker | `serve.stale_checker.enabled` | Stuck task detector |
| cron | (with hub) | Cron schedule poller |

---

### `cq hub`

Manage distributed Hub jobs and workers.

```sh
cq hub status             # Show Hub connection and queue status
cq hub workers            # List connected workers (with affinity scores)
cq hub list               # List recent jobs
```

---

### `cq sessions`

List named Claude Code sessions.

```sh
cq sessions
```

Example output:

```
  my-feature   a07c5035  ~/git/myproject       Mar 01 10:30
  auth-fix     5a98a761  ~/git/myproject        Feb 28 23:12  [2 unread]
  data-work    869fd61e  ~/git/data             Feb 26 18:03
    Analyzing training data pipeline
```

- `[N unread]` shows unread inter-session mail count
- Memo (if set) is displayed on the line below

---

### `cq session`

Manage named Claude Code sessions.

```sh
cq session name <session-name>              # Attach a name to the current session
cq session name <session-name> -m "memo"   # Attach name with memo
cq session rm <session-name>               # Remove a named session
```

Sessions can be resumed with `cq -t <session-name>`.

Use `/c4-attach` inside Claude Code to name a session without leaving the editor.

---

### `cq mail`

Inter-session mail for passing messages between Claude Code sessions.

```sh
cq mail send <to> <body>   # Send a message to a named session
cq mail ls                 # List messages (shows unread count)
cq mail read <id>          # Read a message (marks as read)
cq mail rm <id>            # Delete a message
```

---

### `cq ls`

List registered Telegram bots.

```sh
cq ls
```

---

### `cq setup`

Pair a Telegram bot for notifications.

```sh
cq setup
```

Walks through Telegram bot pairing: enter your bot token, start the bot, and CQ auto-detects your chat ID.

---

### `cq pop`

Personal Ontology Pipeline status and control.

```sh
cq pop status   # Show gauge values, pipeline state, and knowledge stats
```

---

### `cq version`

Print the current binary version and build tier.

```sh
cq version
```

---

### `cq update`

Update the CQ binary to the latest version.

```sh
cq update
```

---

### `cq completion`

Generate shell completion scripts.

```sh
cq completion zsh   # zsh completion script
cq completion bash  # bash completion script
cq completion fish  # fish completion script
```

`cq init` and `install.sh` automatically add completions to `~/.zshrc` / `~/.bashrc`.

---

## Skills (Claude Code Slash Commands)

Skills are slash commands invoked inside Claude Code. All skills are embedded in the CQ binary — no internet required after install.

### Core Workflow

| Skill | Triggers | Available States | Description |
|-------|----------|-----------------|-------------|
| `/pi` | play idea, ideation | Any | Brainstorm before planning. Diverge/converge/research/debate. Auto-launches `/c4-plan`. |
| `/c4-plan` | plan, design | INIT, HALTED | Discovery -> Design -> Lighthouse contracts -> Task creation. |
| `/c4-run` | run, execute | PLAN, HALTED, EXECUTE | Spawn workers for pending tasks. Continuous until queue empty. |
| `/c4-finish` | finish, complete | After implementation | Build -> test -> docs -> commit. Post-implementation completion. |
| `/c4-status` | status | Any | Visual task graph, queue summary, worker status. |
| `/c4-quick` | quick | PLAN, HALTED, EXECUTE | Create + assign one task immediately, skip planning. |

### Quality Loop

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-checkpoint` | (auto) | 4-lens review: holistic / user-flow / cascade / ship-ready. |
| `/c4-validate` | validate | Run lint + tests. CRITICAL blocks commit, HIGH requires review. |
| `/c4-review` | review | Comprehensive 3-pass code review with 6-axis evaluation. |
| `/c4-polish` | polish | *(Deprecated -- built into `/c4-finish`)* |
| `/c4-refine` | refine | *(Deprecated -- built into `/c4-finish`)* |

### Task Management

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-add-task` | add task | Add task interactively with DoD, scope, and domain. |
| `/c4-submit` | submit | Submit completed task with automated validation. |
| `/c4-interview` | interview | Deep requirements interview (PM/architect mode). |
| `/c4-stop` | stop | Halt execution, transition to HALTED. Preserves progress. |
| `/c4-clear` | clear | Reset C4 state. Clears tasks, events, locks. |

### Collaboration

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-swarm` | swarm | Spawn coordinator-led agent team. Modes: implementation, review, investigate. |
| `/c4-standby` | standby | Convert session into distributed worker via Supabase. *full tier only* |

### Session and Utilities

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/init` | init | Initialize C4 in current project. |
| `/c4-attach` | attach | Attach a name to the current session. |
| `/c4-reboot` | reboot | Reboot the current named session. |
| `/c4-release` | release | Generate CHANGELOG from git history. |
| `/c4-help` | help | Quick reference for all skills and MCP tools. |

### Research

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/research-loop` | research loop | Paper-experiment improvement loop. |
| `/c2-paper-review` | paper review | *(Deprecated -- use `/c4-review`)* |

### C9 Research Loop (ML)

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c9-init` | c9-init | Initialize C9 ML research project. Creates `state.yaml`. |
| `/c9-loop` | c9-loop | Main loop driver -- reads current phase, auto-executes next step. |
| `/c9-run` | c9-run | Submit experiment YAMLs to Supabase worker queue. |
| `/c9-check` | c9-check | Parse results + convergence check. |
| `/c9-standby` | c9-standby | Wait during RUN phase; auto-triggers CHECK when training completes. |
| `/c9-finish` | c9-finish | Save best model + document results. |
| `/c9-steer` | c9-steer | Change phase without editing `state.yaml` directly. |
| `/c9-survey` | c9-survey | Survey arXiv + SOTA using Gemini Google Search grounding. |
| `/c9-report` | c9-report | Collect experiment results from remote server. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) debate mode. |
| `/c9-deploy` | c9-deploy | Deploy best model to edge server. |

---

## State Machine

Skills respect the project state machine:

```
INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> REFINE -> POLISH -> COMPLETE
```

| State | Available Skills |
|-------|-----------------|
| INIT | `/init`, `/c4-plan` |
| DISCOVERY / DESIGN | `/c4-plan` (auto-progresses) |
| PLAN | `/c4-run`, `/c4-quick`, `/c4-status` |
| EXECUTE | `/c4-run`, `/c4-quick`, `/c4-stop`, `/c4-status`, `/c4-validate`, `/c4-submit`, `/c4-add-task`, `/c4-swarm` |
| CHECKPOINT | `/c4-checkpoint`, `/c4-add-task` |
| HALTED | `/c4-run`, `/c4-quick`, `/c4-plan` |
| COMPLETE | `/c4-finish`, `/c4-release` |

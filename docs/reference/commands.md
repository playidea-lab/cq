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

### `cq serve`

Run background services (EventBus, GPU scheduler, Agent listener).

```sh
cq serve              # start on :4140
cq serve --port 4141
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
| `/c4-finish` | "마무리", "finish" | Build → test → install → commit |
| `/c4-status` | "상태", "status" | Visual task progress |
| `/c4-quick` | "quick", "빠르게" | Single task, no planning |
| `/c4-polish` | "polish" | Fix until reviewer says zero changes |
| `/c4-refine` | "refine" | Iterative review-fix loop |
| `/c4-checkpoint` | checkpoint reached | Approve / request changes |
| `/c4-validate` | "검증", "validate" | Run lint + tests |
| `/c4-add-task` | "태스크 추가" | Add task interactively |
| `/c4-submit` | "제출", "submit" | Submit completed task |
| `/c4-standby` | "대기", "standby" | Become a C5 Hub worker |

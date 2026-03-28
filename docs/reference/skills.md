# Skills Reference

Skills are slash commands invoked inside Claude Code. All 36 skills are embedded in the CQ binary (`skills_embed` build tag) — no internet required after install.

## Ideation

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/pi` | play idea, 아이디어, ideation, /pi | Brainstorm and refine ideas before planning. Diverge/converge/research/debate modes. Writes `idea.md` and auto-launches `/c4-plan`. |

## Core Workflow

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-plan` | plan, 계획, 설계, 기획 | Discovery → Design → Lighthouse contracts → Task creation. Full structured plan for a feature. |
| `/c4-run` | run, 실행, ㄱㄱ | Spawn workers for all pending tasks in parallel. Continuous mode — auto-respawns until queue empty. |
| `/c4-finish` | finish, 마무리, 완료 | Build → test → docs → commit. Post-implementation completion routine. |
| `/c4-status` | status, 상태 | Visual task graph with progress, dependency graph, queue summary, and worker status. |
| `/c4-quick` | quick, 빠르게 | Create + assign one task immediately, skip planning. For small focused changes. |

## Quality Loop

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-polish` | polish | *(Deprecated — polish loop is now built into `/c4-finish`. No separate invocation needed.)* |
| `/c4-refine` | refine | *(Deprecated — quality loop is now built into `/c4-finish`. No separate invocation needed.)* |
| `/c4-checkpoint` | (auto at checkpoint) | Phase gate: 4-lens review (holistic / user-flow / cascade / ship-ready). Approve, request changes, replan, or redesign. |
| `/c4-validate` | validate, 검증 | Run lint + tests with severity-based handling. CRITICAL blocks commit, HIGH requires review, MEDIUM is recommended. |
| `/c4-review` | review | Comprehensive 3-pass code or paper review with 6-axis evaluation. Generates formal review document. |

## Task Management

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-add-task` | add task, 태스크 추가 | Add task interactively with DoD, scope, and domain guidance. Infers ID from existing patterns. |
| `/c4-submit` | submit, 제출 | Submit completed task with automated validation. Verifies commit SHA, triggers checkpoint if needed. |
| `/c4-interview` | interview | Deep exploratory requirements interview. Acts as senior PM/architect to discover hidden requirements and edge cases. |
| `/c4-stop` | stop, 중단 | Stop execution, transition to HALTED state. Preserves progress for later resumption. |
| `/c4-clear` | clear | Reset C4 state for debugging. Clears tasks, events, locks in `.c4/` with optional config preservation. |

## Collaboration & Scaling

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-swarm` | swarm | Spawn coordinator-led agent team. Modes: standard (implementation), review (read-only audit), investigate (hypothesis competition). |
| `/c4-standby` | standby, 대기, worker mode | Convert session into persistent distributed worker via Supabase. Waits for jobs, executes, reports back. *full tier only* |

## Research & Documents

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c2-paper-review` | 논문 리뷰, paper review | *(Deprecated — use `/c4-review` instead.)* |
| `/research-loop` | research loop | Paper-experiment improvement loop. Iterates review → plan → experiment → re-review until target quality reached. |

## C9 Research Loop (ML)

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c9-init` | c9-init, c9 초기화 | Initialize a new C9 research project. Creates `state.yaml` with metric, convergence conditions, and Hub URL. |
| `/c9-loop` | c9-loop | Main loop driver — reads current phase from `state.yaml` and auto-executes next step. |
| `/c9-run` | c9-run | Submit experiment YAMLs to Supabase worker queue for the current round. |
| `/c9-check` | c9-check | Parse experiment results + convergence check. Equivalent to C4's checkpoint. |
| `/c9-standby` | c9-standby | Wait during RUN phase; auto-triggers CHECK when training completes via mail. |
| `/c9-finish` | c9-finish | Save best model + document results when research loop completes. |
| `/c9-steer` | c9-steer | Change phase and update reason without editing `state.yaml` directly. |
| `/c9-survey` | c9-survey | Survey latest arXiv papers + SOTA benchmarks using Gemini Google Search grounding. |
| `/c9-report` | c9-report | Collect experiment results from remote server via distributed worker. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) debate mode — research conference simulation. |
| `/c9-deploy` | c9-deploy | Deploy best model to edge server. Can run independently of `/c9-finish`. |

## Skill Health

> Requires `connected` or `full` tier (LLM Gateway needed for haiku classification).

Measure and monitor whether skills trigger correctly — ensuring Claude classifies user prompts accurately before and after changes.

| MCP Tool | Description |
|----------|-------------|
| `c4_skill_eval_run` | Run k-trial haiku classification on a skill's EVAL.md test cases. Returns `trigger_accuracy`. |
| `c4_skill_eval_generate` | Generate EVAL.md test cases (positive + negative prompts) for a skill using haiku. |
| `c4_skill_eval_status` | Show trigger accuracy summary for all evaluated skills. `ok` = ≥ 0.90. |

`cq doctor` includes a `skill-health` check that warns when any skill drops below the 0.90 threshold.

## Utilities

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/init` | init, 초기화 | Initialize C4 in current project. Detects installation path, runs `cq claude/cursor/codex`. |
| `/c4-release` | release | Generate CHANGELOG from git history. Conventional Commits analysis, semantic version suggestion, tag creation. |
| `/c4-help` | help | Quick reference for skills, agents, and MCP tools. Decision tree + keyword search across all 36 skills. |
| `/c4-attach` | 세션 이름, attach, name this session | Attach a name to the current session for later resume with `cq claude -t <name>`. Optionally add a memo. |
| `/c4-reboot` | reboot, 재시작 | Reboot the current named session. `cq` resumes with the same session UUID automatically. |

---

## Machine-readable

Download as JSONL for programmatic use:

```sh
curl https://playidea-lab.github.io/cq/api/skills.jsonl
```

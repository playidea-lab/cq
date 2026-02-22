# Skills Reference

Skills are slash commands invoked inside Claude Code. All 22 skills are embedded in the CQ binary (`skills_embed` build tag) — no internet required after install.

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
| `/c4-polish` | polish | Continuous build-test-review-fix loop until reviewer finds zero changes. Coding equivalent of "done done". Run before `/c4-finish`. |
| `/c4-refine` | refine | Stress-test the **plan** before implementation starts. Spawns fresh plan critic each round to review specs, DoDs, and task design. Stops when CRITICAL + HIGH = 0. Run after `/c4-plan`, before `/c4-run`. |
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
| `/c4-standby` | standby, 대기, worker mode | Convert session into persistent C5 Hub worker. Waits for jobs, executes, reports back. *full tier only* |

## Research & Documents

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c2-paper-review` | 논문 리뷰, paper review | Academic paper review via C2 document lifecycle. 3-pass review, 6-axis evaluation, bilingual output, persona learning. |
| `/research-loop` | research loop | Paper-experiment improvement loop. Iterates review → plan → experiment → re-review until target quality reached. |

## Utilities

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-init` | init, 초기화 | Initialize C4 in current project. Detects installation path, runs `cq claude/cursor/codex`. |
| `/c4-release` | release | Generate CHANGELOG from git history. Conventional Commits analysis, semantic version suggestion, tag creation. |
| `/c4-help` | help | Quick reference for skills, agents, and MCP tools. Decision tree + keyword search across all 22 skills. |

---

## Machine-readable

Download as JSONL for programmatic use:

```sh
curl https://playidealab.github.io/cq/api/skills.jsonl
```

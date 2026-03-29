# Skills Reference

Skills are slash commands invoked inside Claude Code. All 42 skills are embedded in the CQ binary (`skills_embed` build tag) -- no internet required after install.

---

## Ideation

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/pi` | play idea, ideation, /pi | Brainstorm and refine ideas before planning. Diverge/converge/research/debate modes. Writes `idea.md` and auto-launches `/plan`. |

---

## Core Workflow

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/plan` | plan, design, spec | Discovery -> Design -> Lighthouse contracts -> Task creation. Full structured plan for a feature. |
| `/run` | run, execute | Spawn workers for all pending tasks in parallel. Continuous mode -- auto-respawns until queue empty. |
| `/finish` | finish, complete | Build -> test -> docs -> commit. Post-implementation completion routine. |
| `/status` | status | Visual task graph with progress, dependency graph, queue summary, and worker status. |
| `/quick` | quick | Create + assign one task immediately, skip planning. For small focused changes. |

---

## Quality Loop

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/checkpoint` | (auto at checkpoint) | Phase gate: 4-lens review (holistic / user-flow / cascade / ship-ready). Approve, request changes, replan, or redesign. |
| `/validate` | validate | Run lint + tests. CRITICAL blocks commit, HIGH requires review, MEDIUM is recommended. |
| `/review` | review | Comprehensive 3-pass code or paper review with 6-axis evaluation. Generates formal review document. |
| `/polish` | polish | *(Deprecated -- polish loop is now built into `/finish`.)* |
| `/refine` | refine | *(Deprecated -- quality loop is now built into `/finish`.)* |

---

## Task Management

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/add-task` | add task | Add task interactively with DoD, scope, and domain guidance. Infers ID from existing patterns. |
| `/submit` | submit | Submit completed task with automated validation. Verifies commit SHA, triggers checkpoint if needed. |
| `/c4-interview` | interview | Deep exploratory requirements interview. Acts as senior PM/architect to discover hidden requirements and edge cases. |
| `/c4-stop` | stop | Stop execution, transition to HALTED state. Preserves progress for later resumption. |
| `/c4-clear` | clear | Reset C4 state for debugging. Clears tasks, events, locks in `.c4/` with optional config preservation. |

---

## Collaboration and Scaling

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c4-swarm` | swarm | Spawn coordinator-led agent team. Modes: standard (implementation), review (read-only audit), investigate (hypothesis competition). |
| `/c4-standby` | standby, worker mode | Convert session into persistent distributed worker via Supabase. Waits for jobs, executes, reports back. *full tier only* |

---

## Research and Documents

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/research-loop` | research loop | Paper-experiment improvement loop. Iterates review -> plan -> experiment -> re-review until target quality reached. |
| `/c2-paper-review` | paper review | *(Deprecated -- use `/review` instead.)* |

---

## C9 Research Loop (ML)

> Requires `connected` or `full` tier for Hub job submission.

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/c9-init` | c9-init | Initialize a new C9 research project. Creates `state.yaml` with metric, convergence conditions, and Hub URL. |
| `/c9-loop` | c9-loop | Main loop driver -- reads current phase from `state.yaml` and auto-executes next step. |
| `/c9-run` | c9-run | Submit experiment YAMLs to Supabase worker queue for the current round. |
| `/c9-check` | c9-check | Parse experiment results + convergence check. Equivalent to C4's checkpoint. |
| `/c9-standby` | c9-standby | Wait during RUN phase; auto-triggers CHECK when training completes via mail. |
| `/c9-finish` | c9-finish | Save best model + document results when research loop completes. |
| `/c9-steer` | c9-steer | Change phase and update reason without editing `state.yaml` directly. |
| `/c9-survey` | c9-survey | Survey latest arXiv papers + SOTA benchmarks using Gemini Google Search grounding. |
| `/c9-report` | c9-report | Collect experiment results from remote server via distributed worker. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) debate mode -- research conference simulation. |
| `/c9-deploy` | c9-deploy | Deploy best model to edge server. Can run independently of `/c9-finish`. |

---

## Utilities

| Skill | Triggers | Description |
|-------|----------|-------------|
| `/init` | init | Initialize C4 in current project. Detects installation path, runs `cq claude/cursor/codex`. |
| `/c4-release` | release | Generate CHANGELOG from git history. Conventional Commits analysis, semantic version suggestion, tag creation. |
| `/c4-help` | help | Quick reference for skills, agents, and MCP tools. Decision tree + keyword search. |
| `/c4-attach` | attach, name this session | Attach a name to the current session for later resume with `cq claude -t <name>`. Optionally add a memo. |
| `/c4-reboot` | reboot | Reboot the current named session. `cq` resumes with the same session UUID automatically. |

---

## Skill Health

> Requires `connected` or `full` tier (LLM Gateway needed for haiku classification).

Measure and monitor whether skills trigger correctly -- ensuring Claude classifies user prompts accurately.

| MCP Tool | Description |
|----------|-------------|
| `c4_skill_eval_run` | Run k-trial haiku classification on a skill's EVAL.md test cases. Returns `trigger_accuracy`. |
| `c4_skill_eval_generate` | Generate EVAL.md test cases (positive + negative prompts) for a skill using haiku. |
| `c4_skill_eval_status` | Show trigger accuracy summary for all evaluated skills. `ok` = >= 0.90. |

`cq doctor` includes a `skill-health` check that warns when any skill drops below the 0.90 threshold.

---

## Skill Catalog (42 Skills)

Complete list by category:

| Skill | Category |
|-------|----------|
| `/pi` | Ideation |
| `/plan` | Core Workflow |
| `/run` | Core Workflow |
| `/finish` | Core Workflow |
| `/status` | Core Workflow |
| `/quick` | Core Workflow |
| `/checkpoint` | Quality Loop |
| `/validate` | Quality Loop |
| `/review` | Quality Loop |
| `/polish` | Quality Loop (deprecated) |
| `/refine` | Quality Loop (deprecated) |
| `/add-task` | Task Management |
| `/submit` | Task Management |
| `/c4-interview` | Task Management |
| `/c4-stop` | Task Management |
| `/c4-clear` | Task Management |
| `/c4-swarm` | Collaboration |
| `/c4-standby` | Collaboration |
| `/research-loop` | Research |
| `/c2-paper-review` | Research (deprecated) |
| `/c9-init` | C9 Research |
| `/c9-loop` | C9 Research |
| `/c9-run` | C9 Research |
| `/c9-check` | C9 Research |
| `/c9-standby` | C9 Research |
| `/c9-finish` | C9 Research |
| `/c9-steer` | C9 Research |
| `/c9-survey` | C9 Research |
| `/c9-report` | C9 Research |
| `/c9-conference` | C9 Research |
| `/c9-deploy` | C9 Research |
| `/init` | Utilities |
| `/c4-release` | Utilities |
| `/c4-help` | Utilities |
| `/c4-attach` | Utilities |
| `/c4-reboot` | Utilities |

---

## Machine-readable

Download as JSONL for programmatic use:

```sh
curl https://playidea-lab.github.io/cq/api/skills.jsonl
```

---
name: research-loop
description: |
  Drive the paper-experiment improvement loop through iterative cycles of review →
  plan → experiment → re-review until target quality is reached. Manages research
  projects with automated experiment submission and score tracking. Re-entrant
  design: each invocation picks up where it left off. Use when iterating on
  research papers with experiment validation. Triggers: "연구 반복", "실험 루프",
  "논문 개선", "research loop", "iterate on paper", "experiment loop",
  "improve research", "paper quality loop".
allowed-tools: Read, Bash, Agent, mcp__cq__*
---

# Research Loop: Paper-Experiment Iteration Tracker

> Drive the paper-experiment improvement loop. Track iterations of review → plan →
> experiment → re-review until the paper reaches target quality.

## Subcommands

| Subcommand | Description |
|---|---|
| `start <name>` | Create a new research project |
| `status` | Show current project status |
| `record` | Record review/experiment results for current iteration |
| `next` | Ask what to do next and execute it |
| `approve <action>` | Continue / pause / complete the project |
| *(no args)* | Same as `next` — auto-detect and suggest |

**Arguments**:
```
--paper: path to paper PDF
--repo: path to experiment code repo
--target: target review score (default: 7.0)
--project-id: explicit project ID (for multi-project scenarios)
```

---

## Phase 1: Parse & Resolve Project

1. Parse subcommand: `start` | `status` | `record` | `next` | `approve` (default: `next`)
2. If `--project-id` not given:
   - Call `c4_research_status` to list active projects
   - If exactly one active project exists, use it
   - If multiple, ask user which one
   - If none and subcommand ≠ `start`, tell user to run `start <name>`

---

## Phase 2: Start — Create Project

**Trigger**: subcommand = `start`

1. Call `c4_research_start(name, paper_path, repo_path, target_score)`
2. Save returned `project_id` and `iteration_id`
3. Display:
   ```
   Research project created: <name>
   - Project ID: <project_id>
   - Target score: <target>
   - Iteration 1 started (status: reviewing)

   Next step: Run a review of the paper.
   - If paper in repo: /c2-review <paper_path> --project <name>
   - Or record manually: /research-loop record
   ```

**Checkpoint**: "프로젝트를 생성했습니다. 리뷰를 시작할까요?"
If user agrees and `paper_path` available, proceed to Phase 3 (Review).

---

## Phase 3: Next — Auto-Detect Action

**Trigger**: subcommand = `next` or no args

1. Call `c4_research_next(project_id)` to get suggested action
2. Branch based on `action`:

### action = "review"

Tell user:
```
Iteration <N>: 리뷰가 필요합니다.
```

**Option A** — If `paper_path` available and auto_review accessible:
```
/c2-review를 실행하여 6축 리뷰를 수행합니다.
```
Invoke `/c2-review <paper_path>`.

**Option B** — Manual:
```
리뷰를 직접 수행한 후 결과를 기록하세요:
/research-loop record --score <점수> --gaps "갭1, 갭2"
```

After review complete, call `c4_research_record` with:
```
c4_research_record(
  project_id: <pid>,
  review_score: <weighted_average>,
  axis_scores: {quality: X, novelty: X, technical: X, experimental: X, discussion: X, presentation: X},
  gaps: [{type: "experiment", desc: "...", priority: "A1"}, ...],
  status: "planning"
)
```

**Checkpoint**: "리뷰 결과: {score}/10. 발견된 갭 {N}개. 실험 계획을 수립할까요?"

### action = "plan_experiments"

1. Retrieve current iteration via `c4_research_status`
2. For each gap with `type: "experiment"`:
   - Analyze what experiment would address it
   - Determine command, GPU requirements, expected outcome
3. Present experiment plan:

```
## Experiment Plan (Iteration <N>)

| # | Gap | Experiment | Command | GPU | Priority |
|---|-----|-----------|---------|-----|----------|
| 1 | Missing LOSO CV | 5-fold LOSO cross-validation | python train.py --cv loso | 1 | A1 |
| 2 | No temporal baseline | Add LSTM baseline | python train.py --model lstm | 1 | A2 |
```

4. Record experiments:
```
c4_research_record(
  project_id: <pid>,
  experiments: [
    {name: "LOSO CV", status: "planned", command: "...", gpu_count: 1},
    ...
  ],
  status: "experimenting"
)
```

**Checkpoint**: "실험 계획 {N}개를 수립했습니다. 실험을 제출할까요?"

### action = "run_experiments"

**CRITICAL: @key=value metric tracking**
When writing or modifying experiment scripts, ALL print() that report metrics MUST include
`@key=value` annotations. cq MetricWriter auto-parses stdout for `@(\w+)=(<number>)` and
sends to experiment_checkpoint in real-time. Example:
```python
print(f'Epoch {epoch} @loss={loss:.4f} @acc={acc:.4f} @val_loss={val_loss:.4f}')
print(f'Final @hd_gt={hd:.4f} @msd_gt={msd:.4f}')
```

1. Get current iteration via `c4_research_status`
2. For each experiment with `status: "planned"`:
   - Submit via `c4_job_submit(command, working_dir, gpu_count)`
   - Record job_id:
     ```
     c4_research_record(
       project_id: <pid>,
       experiments: [
         {name: "LOSO CV", status: "submitted", job_id: <job_id>},
         ...
       ]
     )
     ```

3. Display submitted jobs:
```
## Submitted Experiments

| Experiment | Job ID | Status |
|-----------|--------|--------|
| LOSO CV | job-abc123 | submitted |
| LSTM baseline | job-def456 | submitted |

Experiments are running asynchronously.
Run `/research-loop next` again after experiments complete to continue.
```

**IMPORTANT**: Do NOT block waiting for experiments. They may take hours.

4. After experiments complete (on re-entry), upload results to Drive:
```
# Per-experiment outputs (checkpoints, eval results)
c4_drive_upload(
  local_path: "<output_path>",
  drive_path: "/research/{project_name}/iter-{N}/{exp_name}/",
  metadata: {"experiment": exp_name, "iteration": N, "status": "completed"}
)

# Or bulk upload experiment directory
c4_drive_dataset_upload(
  path: "<experiment_output_dir>",
  name: "research-{project_name}-iter-{N}"
)
```

5. Record experiment results with artifact references:
```
c4_experiment_record(
  title: "{exp_name}: {key_metric}={value}",
  content: |
    Experiment: {exp_name}
    Metrics: {all_metrics}
    Config: {hyperparams}
    Artifacts: /research/{project_name}/iter-{N}/{exp_name}/ (c4_drive_download)
  tags: ["research-loop", "{project_name}", "iter-{N}"]
)
```

**Checkpoint**: "실험 {N}개를 제출했습니다. 완료 후 다시 `/research-loop`을 실행하세요."

### action = "complete"

```
Target score reached! Score: {score} >= Target: {target}

## Project Summary
- Project: <name>
- Total iterations: <N>
- Final score: <score>/10
- Status: COMPLETED
```

Call `c4_research_approve(project_id, action: "complete")`

#### Artifact & Drive 보존

연구 완료 시, 핵심 아티팩트를 Drive에 업로드하여 재사용 가능하게 만든다:

```
# 1. Best model checkpoint → Drive
c4_drive_upload(
  local_path: "<best_checkpoint_path>",
  drive_path: "/research/{project_name}/models/best_model.pt",
  metadata: {"score": score, "iteration": N, "experiment": best_exp_name}
)

# 2. 실험 코드 스냅샷 (디렉토리) → Dataset
c4_drive_dataset_upload(
  path: "<repo_path>/experiments/",
  name: "research-{project_name}-experiments"
)

# 3. 평가 결과/로그 → Drive
c4_drive_upload(
  local_path: "<eval_results_path>",
  drive_path: "/research/{project_name}/results/final_eval.json"
)
```

Drive 경로 규칙:
- `/research/{project_name}/models/` — 체크포인트, 최종 모델
- `/research/{project_name}/results/` — 평가 결과, 로그
- `/research/{project_name}/data/` — 전처리된 데이터 (Dataset 권장)

#### Knowledge 기록 (finish 패턴)

연구 루프 완료 시, Research DB 결과를 Knowledge DB에도 기록:

```
c4_experiment_record(
  title: "Research: {project_name} iteration {N} — score {score}/10",
  content: |
    Project: {project_name}
    Total iterations: {N}
    Final score: {score}/10 (target: {target}/10)
    Axis scores: quality={q}, novelty={nv}, technical={tc}, experimental={ex}, discussion={di}, presentation={pr}
    Resolved gaps: {resolved_gaps_summary}
    Artifacts:
      - Model: /research/{project_name}/models/best_model.pt
      - Results: /research/{project_name}/results/
      - Dataset: research-{project_name}-experiments (c4_drive_dataset_pull)
  tags: ["research-loop", "{project_name}", "iteration-{N}"]
)
```

→ 이전 세션의 실험 기록이 누적되어 있으면 `c4_experiment_search(query="{project_name}")`로 조회 가능
→ 아티팩트 경로가 기록에 포함되어 다른 세션/워커에서 `c4_drive_download`로 즉시 재사용 가능

### action = "none"

```
Project is <status>. No action needed.
- To resume: /research-loop approve continue
```

---

## Phase 4: Record — Manual Result Entry

**Trigger**: subcommand = `record`

Parse arguments:
```
--score: overall review score (float)
--axis: JSON string of axis scores
--gaps: comma-separated gap descriptions
--exp-name: experiment name to update
--exp-status: experiment status (planned/submitted/completed/failed)
--exp-result: experiment result summary
```

Call `c4_research_record` with provided values.

If `--score` provided without `--axis`, ask user for per-axis scores interactively.

Display updated iteration state after recording.

---

## Phase 5: Status — Display Project State

**Trigger**: subcommand = `status`

1. Call `c4_research_status(project_id)`

2. Display:
```
## Research Project: <name>
- Status: <status>
- Target: <target_score>
- Current iteration: <N>

### Iteration History
| # | Score | Status | Gaps | Experiments |
|---|-------|--------|------|-------------|
| 1 | 4.0/10 | done | 5 gaps | 3/3 completed |
| 2 | 6.5/10 | experimenting | 2 gaps | 1/2 running |

### Current Iteration Details
- Score: <score>/10
- Axis scores: quality=7, novelty=5, ...
- Open gaps: <list>
- Pending experiments: <list>
```

3. Call `c4_research_next` and show suggestion:
```
### Suggested Next Action
<action>: <reason>
```

---

## Phase 6: Approve — Control Flow

**Trigger**: subcommand = `approve`

Parse action: `continue` | `pause` | `complete`

- **continue**: Mark current iteration done, create new iteration
  ```
  c4_research_approve(project_id, action: "continue")
  ```
  Display new iteration info and suggest running review.

- **pause**: Pause the project (can resume later)
  ```
  c4_research_approve(project_id, action: "pause")
  ```

- **complete**: Mark project as completed
  ```
  c4_research_approve(project_id, action: "complete")
  ```

---

## Re-Entry Pattern

This skill is designed to be called multiple times across sessions:

```
Session 1: /research-loop start "PPAD Paper 1" --paper paper.pdf --repo /git/ppad
           → Creates project, suggests review
           /research-loop next
           → Runs c2-review, records score 4.0, identifies 5 gaps
           → Plans 3 experiments, submits to GPU

Session 2: /research-loop
           → Checks state: experiments completed
           → Records results, marks iteration done
           → Suggests new review of updated paper

Session 3: /research-loop
           → Runs review: score 6.5, 2 remaining gaps
           → Plans and submits 2 more experiments

Session 4: /research-loop
           → Review: score 7.5 >= target 7.0
           → "Target reached! Complete project?"
```

Each call is stateless — all state in SQLite via `c4_research_*` tools.

---

## Error Handling

- **No c4 MCP tools available**: "c4 MCP 서버가 연결되어 있지 않습니다. c4를 먼저 시작하세요."
- **No active project**: "활성 프로젝트가 없습니다. `/research-loop start <name>`으로 시작하세요."
- **c2 not available for review**: Fall back to manual review recording
- **Experiment submission failure**: Log error, continue with remaining experiments

---

## Usage Examples

```
/research-loop start "PPAD Paper 1" --paper paper.pdf --repo /git/ppad --target 7.0
/research-loop status
/research-loop next
/research-loop record --score 6.5 --gaps "missing LOSO, no temporal baseline"
/research-loop approve continue
/research-loop approve complete
/research-loop                    # same as 'next'
```

# Research Loop: Paper-Experiment Iteration Tracker

> **Command**: `/research-loop [start <name> | status | record | next | approve] [options]`
>
> **Purpose**: Drive the paper-experiment improvement loop. Track iterations of
> review → plan → experiment → re-review until the paper reaches target quality.
>
> **Design**: Re-entrant. Each invocation checks state via `c4_research_next` and
> picks up where it left off. Long-running experiments are submitted async.

---

## Subcommands

| Subcommand | Description |
|---|---|
| `start <name>` | Create a new research project |
| `status` | Show current project status |
| `record` | Record review/experiment results for current iteration |
| `next` | Ask what to do next and execute it |
| `approve <action>` | Continue / pause / complete the project |
| *(no args)* | Same as `next` — auto-detect and suggest |

---

## Phase 1: Parse Arguments & Resolve Project

### 1.1 Parse Subcommand

```
$ARGUMENTS → parse:
  subcommand: start | status | record | next | approve (default: next)
  name: project name (required for start)
  --paper: path to paper PDF
  --repo: path to experiment code repo
  --target: target review score (default: 7.0)
  --project-id: explicit project ID (for multi-project scenarios)
```

### 1.2 Resolve Active Project

If `--project-id` is not given:

1. Call `c4_research_status` to list active projects (if the tool supports listing)
2. If exactly one active project exists, use it
3. If multiple, ask the user which one
4. If none and subcommand is not `start`, tell the user to run `/research-loop start <name>`

---

## Phase 2: Start — Create Project

**Trigger**: subcommand = `start`

### Instructions

1. Call `c4_research_start`:
   ```
   c4_research_start(
     name: <name>,
     paper_path: <--paper>,
     repo_path: <--repo>,
     target_score: <--target or 7.0>
   )
   ```

2. Save the returned `project_id` and `iteration_id`

3. Display:
   ```
   Research project created: <name>
   - Project ID: <project_id>
   - Target score: <target>
   - Iteration 1 started (status: reviewing)

   Next step: Run a review of the paper.
   - If the paper is in this repo: /c2-review <paper_path> --project <name>
   - Or record review results manually: /research-loop record
   ```

### Checkpoint
> "프로젝트를 생성했습니다. 리뷰를 시작할까요?"

If user agrees and paper_path is available, proceed to Phase 3 (Review).

---

## Phase 3: Next — Auto-Detect Action

**Trigger**: subcommand = `next` or no args

### Instructions

1. Call `c4_research_next(project_id: <pid>)` to get suggested action
2. Branch based on `action`:

#### action = "review"

Tell the user:
```
Iteration <N>: 리뷰가 필요합니다.
```

Option A — If paper_path is available and auto_review repo is accessible:
```
/c2-review를 실행하여 6축 리뷰를 수행합니다.
```
Then invoke `/c2-review <paper_path>` (the user must have c2 configured).

Option B — Manual:
```
리뷰를 직접 수행한 후 결과를 기록하세요:
/research-loop record --score <점수> --gaps "갭1, 갭2"
```

After review is complete (either path), call `c4_research_record` with the results:
```
c4_research_record(
  project_id: <pid>,
  review_score: <weighted_average>,
  axis_scores: {quality: X, novelty: X, technical: X, experimental: X, discussion: X, presentation: X},
  gaps: [{type: "experiment", desc: "...", priority: "A1"}, ...],
  status: "planning"
)
```

**Checkpoint**:
> "리뷰 결과: {score}/10. 발견된 갭 {N}개. 실험 계획을 수립할까요?"

#### action = "plan_experiments"

1. Retrieve current iteration data via `c4_research_status`
2. For each gap with `type: "experiment"`:
   - Analyze what experiment would address it
   - Determine command, GPU requirements, expected outcome
3. Present experiment plan as a table:

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

**Checkpoint**:
> "실험 계획 {N}개를 수립했습니다. 실험을 제출할까요?"

#### action = "run_experiments"

1. Get current iteration via `c4_research_status`
2. For each experiment with `status: "planned"`:
   - Submit via `c4_job_submit`:
     ```
     c4_job_submit(
       command: <experiment.command>,
       working_dir: <repo_path>,
       gpu_count: <experiment.gpu_count or 1>
     )
     ```
   - Record the job_id back:
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

**Checkpoint**:
> "실험 {N}개를 제출했습니다. 완료 후 다시 `/research-loop`을 실행하세요."

#### action = "complete"

```
Target score reached! Score: {score} >= Target: {target}

## Project Summary
- Project: <name>
- Total iterations: <N>
- Final score: <score>/10
- Status: COMPLETED
```

Call `c4_research_approve(project_id: <pid>, action: "complete")`

#### action = "none"

```
Project is <status>. No action needed.
- To resume: /research-loop approve continue
```

---

## Phase 4: Record — Manual Result Entry

**Trigger**: subcommand = `record`

### Instructions

Parse additional arguments:
```
--score: overall review score (float)
--axis: JSON string of axis scores
--gaps: comma-separated gap descriptions
--exp-name: experiment name to update
--exp-status: experiment status (planned/submitted/completed/failed)
--exp-result: experiment result summary
```

Call `c4_research_record` with provided values.

If `--score` is provided without `--axis`, ask the user for per-axis scores interactively.

Display updated iteration state after recording.

---

## Phase 5: Status — Display Project State

**Trigger**: subcommand = `status`

### Instructions

1. Call `c4_research_status(project_id: <pid>)`

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

### Instructions

Parse action: `continue` | `pause` | `complete`

- **continue**: Mark current iteration done, create new iteration
  ```
  c4_research_approve(project_id: <pid>, action: "continue")
  ```
  Then display new iteration info and suggest running a review.

- **pause**: Pause the project (can resume later)
  ```
  c4_research_approve(project_id: <pid>, action: "pause")
  ```

- **complete**: Mark project as completed
  ```
  c4_research_approve(project_id: <pid>, action: "complete")
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

Each call is stateless — all state is in SQLite via `c4_research_*` tools.

---

## Error Handling

- **No c4 MCP tools available**: "c4 MCP 서버가 연결되어 있지 않습니다. c4를 먼저 시작하세요."
- **No active project**: "활성 프로젝트가 없습니다. `/research-loop start <name>`으로 시작하세요."
- **c2 not available for review**: Fall back to manual review recording
- **Experiment submission failure**: Log error, continue with remaining experiments

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

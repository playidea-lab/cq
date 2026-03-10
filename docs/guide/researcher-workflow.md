# Researcher Workflow

End-to-end guide for running ML experiments with CQ — from idea to results.

::: info full tier required
The researcher workflow uses C5 Hub (distributed jobs) and C9 Knowledge (experiment tracking). Both require the `full` tier.
:::

## Overview

```
Idea → /pi → /c4-plan → /c4-run → cq hub submit → results → c4_experiment_record
```

CQ handles the full research loop: ideation, task breakdown, distributed execution, and knowledge capture.

---

## Step 1: Explore the idea

Before writing any code, use `/pi` to brainstorm:

```
/pi
```

`/pi` enters ideation mode — diverge, converge, research, debate. When you're ready, it automatically launches `/c4-plan`.

---

## Step 2: Plan the experiment

Describe your research goal:

```
/c4-plan "train a ResNet-50 on CIFAR-10 with cosine LR schedule"
```

CQ will:
1. Ask clarifying questions (Discovery)
2. Design the experiment structure (Design)
3. Break it into tasks with DoD
4. Create the task queue

---

## Step 3: Implement locally

```
/c4-run
```

Workers implement code in isolated git worktrees. When done, `/c4-run` automatically runs polish → finish.

---

## Step 4: Submit to Hub (distributed execution)

Once code is ready, submit to a GPU worker:

```sh
cq hub submit --run "python train.py" --project my-experiment
```

This:
1. Snapshots your project to Drive CAS
2. Posts a job to C5 Hub
3. A GPU worker pulls the job, downloads the snapshot, runs it, uploads results

Declare inputs/outputs in `cq.yaml`:

```yaml
run: python train.py

artifacts:
  input:
    - name: cifar10-mini
      local_path: data/cifar10
  output:
    - name: model-checkpoint
      local_path: checkpoints/
```

Monitor progress:

```sh
cq hub list
cq hub status <job-id>
```

---

## Step 5: Record results

After the experiment completes, record metrics in C9 Knowledge:

```
c4_experiment_record(exp_id="exp001", metrics={"accuracy": 0.923, "loss": 0.21})
```

Or in Claude Code:

```
Record experiment exp001: accuracy=0.923, loss=0.21, epochs=50
```

Search past experiments:

```
c4_experiment_search(query="ResNet CIFAR-10 cosine LR")
```

---

## Step 6: Iterate

```
/c4-plan "try label smoothing and mixup augmentation"
```

CQ tracks all iterations in the knowledge base. Use `c4_experiment_search` to find the best run before starting a new one.

---

## Full example: CIFAR-10 training

```sh
# 1. Initialize project
cq claude

# 2. Plan
# In Claude Code: /c4-plan "CIFAR-10 ResNet-50 baseline"

# 3. Implement
# In Claude Code: /c4-run

# 4. Submit to Hub
cq hub submit --run "python train.py --epochs 100"

# 5. Monitor
cq hub list

# 6. Record results (in Claude Code)
# c4_experiment_record(exp_id="exp001", metrics={"top1_acc": 0.923})
```

---

## Tips

- Use `c4_knowledge_search` to check if a similar experiment was already run before starting
- Keep `cq.yaml` in the project root — no code changes needed for Hub integration
- Workers are stateless: all credentials and config arrive via the job payload
- Use `cq hub watch <job-id>` to stream logs in real time

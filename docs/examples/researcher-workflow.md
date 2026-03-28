# Researcher Workflow: Plan → Train → Evaluate → Iterate

An end-to-end example for ML researchers using CQ to manage the full experiment lifecycle — from paper spec to final results.

---

## Scenario

You're running experiments for a computer vision paper. The target: beat DINOv2 baseline on a custom medical imaging dataset (MPJPE < 2.0 mm). You need to:

1. Define an experiment plan
2. Submit training jobs to GPU workers
3. Track metrics across runs
4. Iterate based on results
5. Record final findings

---

## Step 1: Start a Research Loop

Use `/c4-research` to create a tracked research session that persists across Claude Code sessions:

```
/c4-research start "DINOv2-Medical: beat 2.0mm MPJPE on CXR dataset" --target 2.0mm
```

```
[RESEARCH] Session started: rs-dino-medical-001
Target: MPJPE < 2.0mm
Phase: PLAN

Next: define your experiment hypothesis and first run
```

The session state is saved — if you close Claude Code and reopen it tomorrow, you can resume:

```
/c4-research status
```

```
Session: rs-dino-medical-001
Target: MPJPE < 2.0mm
Phase: PLAN
Best result: none yet
Experiments: 0 completed
```

---

## Step 2: Plan the First Experiment

Before running anything, record your hypothesis:

```
c4_knowledge_record(
    type="insight",
    content="Hypothesis: DINOv2 ViT-B/14 fine-tuned on CXR with frozen backbone should reach ~2.3mm. Need to unfreeze last 4 layers to close the gap."
)
```

Define the first experiment:

```
/c4-research next
```

```
[RESEARCH] Phase: PLAN → EXPERIMENT

Suggested experiment based on your hypothesis:
  name: exp-001-frozen-backbone
  model: DINOv2 ViT-B/14
  config:
    freeze_backbone: true
    epochs: 50
    lr: 1e-4
    batch_size: 32

Run it? (or modify the config)
```

Approve and it creates a task:

```
Task created: T-EXP-001
  command: python train.py --exp exp-001 --freeze-backbone
  gpu_count: 1
```

---

## Step 3: Submit Training Job

CQ submits the training job to the Hub:

```
c4_hub_submit(
    name="exp-001-frozen-backbone",
    workdir="/git/dino-medical",
    command="python train.py --exp exp-001 --freeze-backbone --epochs 50 --lr 1e-4",
    gpu_count=1
)
```

```json
{"job_id": "j-exp001-a4f2"}
```

Your `train.py` must emit metrics in CQ format:

```python
# train.py (relevant section)
for epoch in range(args.epochs):
    mpjpe, pa_mpjpe = evaluate(model, val_loader)
    print(f"Epoch {epoch+1}/{args.epochs} @mpjpe={mpjpe:.4f} @pa_mpjpe={pa_mpjpe:.4f}")
```

Monitor in real time:

```
c4_hub_watch(job_id="j-exp001-a4f2")
```

```
[GPU: A100 80GB]
Epoch 1/50  @mpjpe=8.421 @pa_mpjpe=6.103
Epoch 5/50  @mpjpe=5.812 @pa_mpjpe=4.231
Epoch 10/50 @mpjpe=4.103 @pa_mpjpe=3.018
Epoch 20/50 @mpjpe=3.421 @pa_mpjpe=2.512
Epoch 30/50 @mpjpe=3.108 @pa_mpjpe=2.203
Epoch 40/50 @mpjpe=3.021 @pa_mpjpe=2.147
Epoch 50/50 @mpjpe=2.981 @pa_mpjpe=2.109
```

Training complete. Best MPJPE: **2.981 mm** — above the 2.0 mm target.

---

## Step 4: Record the Result

```
c4_knowledge_record(
    type="experiment",
    content="""
exp-001: DINOv2 ViT-B/14, frozen backbone
  MPJPE:    2.981mm
  PA-MPJPE: 2.109mm
  Epochs:   50
  LR:       1e-4

Analysis: Frozen backbone limits adaptation to medical domain.
Next: unfreeze last 4 transformer blocks.
"""
)
```

---

## Step 5: Iterate — Unfreeze Layers

Based on exp-001, adjust the hypothesis. Run exp-002 with unfrozen layers:

```
c4_hub_submit(
    name="exp-002-unfreeze-last4",
    workdir="/git/dino-medical",
    command="python train.py --exp exp-002 --unfreeze-layers 4 --epochs 50 --lr 5e-5",
    gpu_count=1
)
```

This time, run exp-003 in parallel (try lower learning rate):

```
c4_hub_submit(
    name="exp-003-unfreeze-lr1e5",
    workdir="/git/dino-medical",
    command="python train.py --exp exp-003 --unfreeze-layers 4 --epochs 50 --lr 1e-5",
    gpu_count=1
)
```

Both jobs run simultaneously on separate workers.

Check when done:

```
c4_hub_summary(job_id="j-exp002-b7c1")
c4_hub_summary(job_id="j-exp003-c2d4")
```

```
exp-002: MPJPE=2.103mm  PA-MPJPE=1.814mm  ← below target!
exp-003: MPJPE=2.341mm  PA-MPJPE=2.012mm
```

exp-002 beats the 2.0 mm target.

---

## Step 6: Confirm Best Result

Update the research session with the winning experiment:

```
/c4-research record --exp exp-002 --mpjpe 2.103 --pa-mpjpe 1.814
```

```
[RESEARCH] Result recorded.
  Session best: MPJPE=2.103mm (exp-002)
  Target:       MPJPE < 2.0mm
  Status:       TARGET NOT YET MET (2.103 > 2.000)
```

Close, but not there yet. One more iteration.

---

## Step 7: Fine-Tune the Winner

```
c4_hub_submit(
    name="exp-004-longer-training",
    workdir="/git/dino-medical",
    command="python train.py --exp exp-004 --unfreeze-layers 4 --epochs 100 --lr 5e-5 --cosine-annealing",
    gpu_count=1
)
```

```
c4_hub_watch(job_id="j-exp004-d5e6")
```

```
Epoch 80/100  @mpjpe=2.021 @pa_mpjpe=1.743
Epoch 90/100  @mpjpe=1.982 @pa_mpjpe=1.701
Epoch 100/100 @mpjpe=1.941 @pa_mpjpe=1.678
```

**MPJPE: 1.941 mm — target met.**

---

## Step 8: Approve and Complete

```
/c4-research record --exp exp-004 --mpjpe 1.941 --pa-mpjpe 1.678
/c4-research approve complete
```

```
[RESEARCH] Session complete.

Summary: DINOv2-Medical — beat 2.0mm MPJPE target
  Best:       exp-004, MPJPE=1.941mm, PA-MPJPE=1.678mm
  Experiments: 4 runs
  Key finding: Unfreeze last 4 layers + 100 epochs + cosine annealing

Knowledge recorded. Session closed: rs-dino-medical-001
```

---

## Full Experiment Summary

| Exp | Config | MPJPE | PA-MPJPE | Note |
|-----|--------|-------|---------|------|
| exp-001 | frozen backbone, 50 epochs, lr=1e-4 | 2.981 | 2.109 | baseline |
| exp-002 | unfreeze 4 layers, 50 epochs, lr=5e-5 | 2.103 | 1.814 | first below 2.1 |
| exp-003 | unfreeze 4 layers, 50 epochs, lr=1e-5 | 2.341 | 2.012 | lr too low |
| exp-004 | unfreeze 4 layers, 100 epochs, cosine | **1.941** | **1.678** | **target met** |

---

## Knowledge Retrieval in Future Sessions

The recorded experiments are searchable:

```
c4_knowledge_search(query="DINOv2 medical MPJPE unfreeze")
```

```
Found 3 results:

1. [experiment] exp-004: DINOv2 ViT-B/14, unfreeze 4 layers...
   MPJPE=1.941mm — best result, target met

2. [insight] Hypothesis: frozen backbone limits domain adaptation...
   Next: unfreeze last 4 layers

3. [experiment] exp-001: frozen backbone baseline...
   MPJPE=2.981mm
```

---

## Tips

**Always record hypotheses before running.** It takes 30 seconds and saves hours of "why did I run this?" confusion later.

**Run parallel experiments for hyperparameter sweeps.** `c4_hub_submit` is non-blocking — submit 3–4 jobs and check results together.

**Use `@metric=value` format consistently.** CQ parses these automatically from stdout. No separate metrics file needed.

**Session state survives restarts.** `/c4-research status` works in any new Claude Code session — you don't lose context.

---

## Next Steps

- **DAG pipelines for multi-step workflows**: [Distributed Experiments](distributed-experiments.md)
- **Feature implementation workflow**: [Feature Planning](feature-planning.md)
- **Experiment tracking MCP tools**: [Usage Guide §6](../usage-guide.md)

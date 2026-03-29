# Research Loop — Autonomous Experiments

Run GPU experiments while you sleep. CQ plans, submits, evaluates, and iterates — autonomously cycling until your target metric is reached.

**Scenario**: Improving a 3D pose estimation model. Starting metric: MPJPE 48mm. Target: below 25mm.

---

## Phase 1: Idea — /pi

```
/pi "improve 3D pose estimation accuracy, currently MPJPE 48mm, target <25mm"
```

CQ enters brainstorming mode, surfaces related knowledge, and structures the approach:

```
[pi] Loading related knowledge...
  Found: "exp-038: DINOv2 features improved MPJPE by 8mm on H36M"
  Found: "insight: attention pooling outperforms average pooling for joint prediction"

[pi] Brainstorming directions:
  1. Backbone upgrade — DINOv2 ViT-L instead of ResNet50
  2. Loss function — add bone length consistency loss
  3. Data augmentation — occlusion simulation
  4. Architecture — cross-attention between joint features
```

---

## Phase 2: Plan and Implement

```
/c4-plan    → Requirements (EARS) + architecture decision + tasks
/c4-run 2   → Parallel workers implement code changes
```

Two workers run simultaneously on independent tasks. Worktree branches merge automatically.

---

## Phase 3: Submit to GPU

```python
c4_hub_submit(
    name="dinov2-boneloss-exp043",
    workdir="/git/pose-model",
    command="python train.py --config configs/exp043.yaml",
    gpu_count=1,
    exp_id="exp-043"
)
```

Metrics stream in real time via the `@key=value` convention:

```
[job-8b2f1e] Epoch 1/80   @loss=2.1432  @mpjpe=45.12
[job-8b2f1e] Epoch 40/80  @loss=0.4821  @mpjpe=26.88
[job-8b2f1e] Epoch 80/80  @loss=0.2941  @mpjpe=21.43
[job-8b2f1e] Status: SUCCEEDED
```

No SDK needed. Just `print(f"@mpjpe={value}")` in your training script.

---

## Phase 4: Autonomous Loop

Hand off to the Research Loop for multi-cycle improvement:

```
/c9-loop start \
  --goal "improve MPJPE below 18mm" \
  --budget 5 \
  --metric mpjpe \
  --direction minimize
```

The LoopOrchestrator runs autonomously:

```
[loop] Cycle 1 — best: 21.43mm
  Hypothesis: cross-attention between joint queries
  Execute: hub submit exp-044
  Result: @mpjpe=19.87mm  (improvement: 1.56mm)

[loop] Cycle 2 — best: 19.87mm
  Hypothesis: occlusion augmentation during training
  Execute: hub submit exp-045
  Result: @mpjpe=18.24mm  (improvement: 1.63mm)

[loop] Cycle 3 — best: 18.24mm
  Hypothesis: cosine LR schedule with warm restart
  Execute: hub submit exp-046
  Result: @mpjpe=17.61mm  — goal reached.
```

Go to sleep. Wake up to results.

---

## Architecture

```
Your Laptop                 Relay               Remote GPU Machine
┌──────────────┐            ┌──────────┐        ┌─────────────────┐
│ cq hub submit│──WebSocket►│  Relay   │──WSS──►│ cq worker start │
│              │            └──────────┘        │                 │
│ cq hub watch │◄─────────── metrics stream ────│ train.py output │
└──────────────┘                                │  @loss=0.312    │
                                                │  @mpjpe=24.3    │
                                                └─────────────────┘
```

NAT traversal handled automatically. No port forwarding needed.

---

## @key=value Metric Convention

| Output line | Captured |
|-------------|----------|
| `@loss=0.312` | loss: 0.312 |
| `@mpjpe=24.3 @hd=1.74` | mpjpe: 24.3, hd: 1.74 |

Multiple metrics on one line work. Parsed from stdout automatically.

---

## DAG Pipelines

For multi-stage experiments (preprocess → train → evaluate):

```python
c4_hub_dag_from_yaml(yaml_content="""
name: full-pipeline
nodes:
  - name: preprocess
    command: python preprocess.py
  - name: train
    command: python train.py
    gpu_count: 1
  - name: evaluate
    command: python evaluate.py
dependencies:
  - source: preprocess
    target: train
  - source: train
    target: evaluate
""")
```

Stages run in dependency order. Independent stages parallelize across workers.

---

## Results

After 3 autonomous cycles:

| Experiment | Change | MPJPE |
|-----------|--------|-------|
| exp-042 (baseline) | ResNet50 | 48.00mm |
| exp-043 | + DINOv2 + bone loss | 21.43mm |
| exp-044 | + cross-attention | 19.87mm |
| exp-045 | + occlusion aug | 18.24mm |
| exp-046 | + cosine LR | 17.61mm |

**63% reduction. 4 experiments. 6 hours GPU time. Zero human intervention after setup.**

---

## Next Steps

- [Growth Loop](growth-loop-in-action.md) — how CQ learns your research preferences
- [Remote MCP](remote-mcp.md) — access results from any AI tool

# Distributed ML Experiments

This example shows how to submit a GPU training job from your laptop, have a
remote worker pick it up via CQ's relay (NAT traversal included), stream
metrics in real time, and collect results into your knowledge base.

---

## Prerequisites

- CQ installed and logged in (`cq`)
- `cq serve` running (starts the relay connection)
- At least one remote worker connected via `cq worker start`
- A training script ready locally (e.g., `train.py`)

---

## Architecture Overview

```
Your Laptop                 Fly.io Relay            Remote GPU Machine
┌──────────────┐            ┌──────────┐            ┌─────────────────┐
│ cq hub submit│──WebSocket►│  Relay   │──WebSocket►│ cq worker start │
│              │            └──────────┘            │                 │
│ cq hub watch │◄──────────── metrics stream ───────│ train.py output │
└──────────────┘                                    │  @loss=0.312    │
                                                    │  @mpjpe=24.3    │
                                                    └─────────────────┘
```

The relay handles NAT traversal automatically — no port forwarding needed on
either side.

---

## Step 1: Check Worker Availability

Before submitting, confirm a worker is online:

```bash
cq hub workers
```

```
WORKER ID         STATUS    GPU          LAST SEEN
worker-gpu-a100   online    A100 80GB    2s ago
worker-rtx4090    online    RTX 4090     8s ago
```

---

## Step 2: Submit a Training Job

```bash
cq hub submit \
  --name "resnet50-baseline" \
  --workdir /home/user/projects/my-model \
  --command "python train.py --epochs 50 --lr 0.001" \
  --gpu \
  --exp-id exp-042
```

CQ assigns the job to an available GPU worker:

```
[hub] Job submitted: job-7f3a9c
[hub] Assigned to: worker-gpu-a100
[hub] Status: queued → running (2s)
[hub] Streaming output...
```

---

## Step 3: Stream Metrics in Real Time

Your training script outputs metrics using the `@key=value` convention:

```python
# train.py
for epoch in range(num_epochs):
    loss = train_one_epoch(model, loader)
    mpjpe = evaluate(model, val_loader)
    print(f"Epoch {epoch+1}/{num_epochs}  @loss={loss:.4f}  @mpjpe={mpjpe:.2f}")
```

CQ's MetricWriter parses stdout automatically. No SDK changes needed.
The metrics are stored in Supabase `experiment_checkpoints` in real time.

Live output on your laptop:

```
[job-7f3a9c] Epoch 1/50   @loss=1.2341  @mpjpe=48.32
[job-7f3a9c] Epoch 2/50   @loss=0.9812  @mpjpe=42.17
[job-7f3a9c] Epoch 5/50   @loss=0.6204  @mpjpe=35.91
[job-7f3a9c] Epoch 10/50  @loss=0.3891  @mpjpe=28.44
...
[job-7f3a9c] Epoch 50/50  @loss=0.2103  @mpjpe=22.71
[job-7f3a9c] Status: SUCCEEDED
```

---

## Step 4: Artifacts Auto-Uploaded to Drive

After the job completes, any files written to `artifacts/` in the workdir are
uploaded to CQ Drive automatically:

```
[hub] Uploading artifacts...
  checkpoints/epoch_50.pth    (142 MB)  ✓  TUS resumable upload
  results/metrics.json        (2 KB)    ✓
  logs/train.log              (88 KB)   ✓
[hub] Artifacts stored: drive://exp-042/
```

Drive uses content-addressable storage — the same checkpoint won't be
uploaded twice even if you rerun the experiment.

---

## Step 5: Review Results

```bash
cq hub summary job-7f3a9c
```

```
Job:        job-7f3a9c  (resnet50-baseline)
Status:     SUCCEEDED
Duration:   1h 23m
Worker:     worker-gpu-a100 (A100 80GB)

Metrics (final epoch):
  loss:    0.2103
  mpjpe:   22.71 mm
  pa_mpjpe: 16.84 mm

Artifacts:
  checkpoints/epoch_50.pth
  results/metrics.json

Exp ID: exp-042
```

---

## Step 6: Record Results as Knowledge

```bash
# In Claude Code session
c4_knowledge_record(
  type="experiment",
  content="exp-042: ResNet50 baseline — MPJPE 22.71mm / PA-MPJPE 16.84mm.
           LR=0.001, epochs=50, A100 80GB, 1h23m. 
           Artifact: drive://exp-042/checkpoints/epoch_50.pth"
)
```

This knowledge is searchable across all future sessions and by all AI
platforms connected to your CQ account.

---

## Running a Multi-Step Pipeline (DAG)

For experiments with preprocessing → training → evaluation stages:

```python
# In Claude Code
c4_hub_dag_from_yaml(yaml_content="""
name: full-training-pipeline
nodes:
  - name: preprocess
    command: python preprocess.py --output data/processed/
  - name: train
    command: python train.py --data data/processed/ --epochs 50
    gpu_count: 1
  - name: evaluate
    command: python evaluate.py --checkpoint artifacts/best.pth
dependencies:
  - source: preprocess
    target: train
  - source: train
    target: evaluate
""")
```

Then execute:

```python
c4_hub_dag_execute(dag_id="full-training-pipeline")
```

CQ runs stages in dependency order. Independent stages run in parallel
if multiple workers are available.

---

## @key=value Metric Convention

Any `@key=value` pattern in stdout is captured automatically.

| Output line | Captured metric | Value |
|-------------|----------------|-------|
| `@loss=0.312` | loss | 0.312 |
| `@mpjpe=24.3` | mpjpe | 24.3 |
| `@acc=0.943` | acc | 0.943 |
| `@hd_gt=1.74 @recall=0.88` | hd_gt, recall | 1.74, 0.88 |

Multiple metrics on one line are all captured. No special library needed.

---

## Next Steps

- [Researcher End-to-End Workflow](researcher-workflow.md) — full research loop with auto-cycling
- [Usage Guide](../usage-guide.md) — Hub commands reference

# Example: Distributed Experiments

Submit ML experiments from your laptop and run them on a remote GPU server — no SSH into the server during training.

::: info full tier required
This example requires the `full` tier binary, a running C5 Hub, and at least one connected worker. See [Remote Worker Setup](/guide/worker-setup).
:::

## Overview

```
Your laptop                    C5 Hub                  GPU server
────────────                   ──────                  ──────────
Write train.py + cq.yaml  ──►  job queue  ◄──────────  c5 worker (running)
cq hub submit                  (stores snapshot,        pulls job
                               queues job)              runs train.py
                                                        pushes results
```

The GPU server runs `c5 worker` in the background. You never SSH in to start training — you just submit.

---

## Set up the experiment folder

Create your experiment directory on your **laptop**:

```
mnist-exp/
├── train.py       # your training script
├── cq.yaml        # CQ artifact declarations
└── requirements.txt
```

**`cq.yaml`** — declares what to run and which data to use:

```yaml
run: python train.py

artifacts:
  input:
    - name: mnist_mini        # Drive dataset name
      local_path: data/mnist  # where to put it before running
  output:
    - name: mnist-checkpoint  # Drive dataset name for results
      local_path: checkpoints/ # local path to upload after running
```

**`train.py`** — plain Python, zero CQ code required:

```python
import torch
from pathlib import Path

# data/ and checkpoints/ are ready — worker put them there
data_path = Path("data/mnist")
# ... training logic ...
torch.save(model.state_dict(), "checkpoints/model.pt")
print("MPJPE: 42.3")
```

No `import cq`, no SDK, no changes to your existing scripts.

## Upload your dataset (once)

If you haven't uploaded `mnist_mini` yet:

```sh
cq drive dataset upload ./data/mnist --as mnist_mini -y
```

CQ uses content-addressed storage — the same data is never uploaded twice.

## Submit the job

From inside `mnist-exp/`:

```sh
cq hub submit
```

CQ:
1. Snapshots the current folder (Drive CAS, deduped by content hash)
2. Posts a job: `{ command, snapshot_version_hash, project_id }`
3. The Hub queues it for the next available worker

Output:

```
✓ Snapshot uploaded  hash=a3f8c1d2
✓ Job submitted      job-id=job-abc123  status=QUEUED  position=1
```

That's it. Close your laptop. The GPU server handles the rest.

## What the worker does

When the worker picks up the job:

1. Downloads the exact snapshot (`a3f8c1d2` → `mnist-exp/` contents)
2. Reads `cq.yaml`
3. Pulls `mnist_mini` from Drive → `data/mnist/`
4. Runs `python train.py`
5. Uploads `checkpoints/` → `mnist-checkpoint` in Drive

## Check status

```sh
cq hub status job-abc123
```

Or watch live:

```sh
cq hub watch job-abc123
```

```
job-abc123  RUNNING  [gpu-server-1]
  ▶ epoch 1/10 — loss: 0.4821
  ▶ epoch 2/10 — loss: 0.3102
  ...
```

## Download results

```sh
cq drive dataset download mnist-checkpoint ./results/
```

## Submit multiple jobs (parallel)

Submit variations back-to-back — each runs on a separate worker:

```sh
cq hub submit --run "python train.py --lr 0.01" 
cq hub submit --run "python train.py --lr 0.001"
cq hub submit --run "python train.py --lr 0.0001"
```

All three run in parallel if multiple workers are connected.

---

## Connect more workers

Add GPU capacity by running `c5 worker` on more machines. The Hub distributes jobs automatically — no configuration needed.

```
machine-1 ──┐
machine-2 ──┼── C5 Hub ── job queue ◄── cq hub submit
machine-3 ──┘
  ...
```

Each worker is stateless — just install, log in, and start `c5 worker`. See [Remote Worker Setup](/guide/worker-setup).

## Knowledge loop

Experiment results are automatically recorded in CQ's knowledge base. When you plan the next round, CQ injects relevant past findings into the context — so you don't repeat what didn't work.

# Example: Distributed Experiments

Run ML experiments across multiple machines simultaneously using C5 Hub.

::: info full tier required
This example requires the `full` tier binary and C5 Hub running. `/c4-standby` connects each machine to the job queue.
:::

## Scenario

You need to compare three model backbones to find the best accuracy/efficiency tradeoff.

## Step 1 — Plan the experiments

> **You:** "backbone 3개 비교 실험 돌려야 해. ResNet / EfficientNet / ViT"

```
/c4-plan "backbone ablation: ResNet50 vs EfficientNet-B4 vs ViT-B/16"

  ● Tasks created
    T-020  train ResNet50       (GPU: 1x A100, est. 4h)
    T-021  train EfficientNet   (GPU: 1x A100, est. 3h)
    T-022  train ViT-B/16       (GPU: 1x A100, est. 6h)
    T-023  compare results + report
```

## Step 2 — Register workers on each machine

On each machine, open Claude Code and run:

```
# machine-1
/c4-standby

  ● Registered as worker  [id: worker-m1]
  ◷ Waiting for jobs from C5 Hub...
  ✓ Claimed T-020  →  training ResNet50...
```

```
# machine-2
/c4-standby
  ✓ Claimed T-021  →  training EfficientNet...
```

```
# machine-3
/c4-standby
  ✓ Claimed T-022  →  training ViT-B/16...
```

Workers pull jobs automatically — no manual assignment needed.

## Step 3 — Monitor progress

> **You:** "결과 어때?"

```
/c4-status

  ✓ T-020  ResNet50       MPJPE: 48.3mm  PA-MPJPE: 34.1mm  [worker-m1]
  ✓ T-021  EfficientNet   MPJPE: 44.7mm  PA-MPJPE: 31.8mm  [worker-m2]  ← best so far
  ▶ T-022  ViT-B/16       running 3h 42m / ~6h               [worker-m3]
  ◷ T-023  waiting on T-020, T-021, T-022

  Knowledge: 2 new experiment records saved
```

## Step 4 — Comparison report (auto-generated)

When T-022 completes, T-023 runs and produces:

```
  ✓ T-023  Comparison report

  | Backbone      | MPJPE  | PA-MPJPE | Params | Throughput |
  |---------------|--------|----------|--------|------------|
  | ResNet50      | 48.3mm | 34.1mm   | 25.6M  | 142 img/s  |
  | EfficientNet  | 44.7mm | 31.8mm   | 19.3M  | 98 img/s   |
  | ViT-B/16      | 41.2mm | 29.4mm   | 86.6M  | 47 img/s   |

  Recommendation: EfficientNet-B4 — best accuracy/efficiency ratio
  Knowledge recorded → next experiment will build on these findings
```

## How it scales

Each `/c4-standby` session is an independent worker pulling from the same C5 Hub queue. Add more machines by opening more sessions — the queue distributes automatically.

```
machine-1 ──┐
machine-2 ──┼── C5 Hub ── task queue ── /c4-plan results
machine-3 ──┘
  ...
machine-N ──
```

## Knowledge loop

Results from each experiment are automatically recorded to the knowledge base. When you plan the next round of experiments, CQ injects relevant past findings into each worker's context — so workers already know what didn't work.

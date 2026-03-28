# Researcher End-to-End Workflow

This example walks through a complete ML research cycle using CQ — from initial
idea through planning, implementation, GPU training, evaluation, and iterative
improvement. The Research Loop can run autonomously for multiple cycles without
human intervention.

**Scenario**: Improving a 3D pose estimation model. Starting metric: MPJPE 48mm.
Target: below 25mm.

---

## Phase 1: Idea — /pi

Start with brainstorming before writing any code. `/pi` is a free-form
exploration mode that saves structured notes to `idea.md`.

```
/pi "improve 3D pose estimation accuracy, currently MPJPE 48mm, target <25mm"
```

CQ enters Play Idea mode and asks questions, surfaces related knowledge from
your knowledge base, and helps structure the approach:

```
[pi] Loading related knowledge...
  Found: "exp-038: DINOv2 features improved MPJPE by 8mm on H36M"
  Found: "insight: attention pooling outperforms average pooling for joint prediction"

[pi] Brainstorming directions:
  1. Backbone upgrade — DINOv2 ViT-L instead of ResNet50
  2. Loss function — add bone length consistency loss
  3. Data augmentation — occlusion simulation
  4. Architecture — cross-attention between joint features

[pi] Saving to idea.md...
```

---

## Phase 2: Plan — /c4-plan

Convert the idea into structured requirements and concrete tasks:

```
/c4-plan
```

CQ runs Discovery → Design → Task generation:

```
[plan] DISCOVERY — collecting requirements (EARS format)
  - When training on H36M, system shall achieve MPJPE < 25mm
  - When backbone is DINOv2 ViT-L, system shall load pretrained weights
  - If bone length loss weight > 0.1, system shall apply joint consistency
  - System shall not exceed 200ms inference per frame

[plan] DESIGN — architecture decision
  Option A: Replace backbone only (lower risk, faster iteration)
  Option B: Full architecture redesign (higher ceiling, 2x time)
  Selected: Option A — start minimal, iterate

[plan] TASKS created:
  T-RES-01  Replace ResNet50 backbone with DINOv2 ViT-L
  T-RES-02  Add bone length consistency loss
  T-RES-03  Update training config and data pipeline
  T-RES-04  Run baseline evaluation (exp-043)
  CP-RES    Checkpoint: compare against exp-042 baseline (MPJPE 22.71mm)
```

---

## Phase 3: Implement — /c4-run

Spawn workers to execute tasks in parallel:

```
/c4-run 2
```

Two workers start simultaneously on independent tasks:

```
[worker-1] T-RES-01: Replace backbone with DINOv2 ViT-L
  Editing model/backbone.py ...
  Editing model/feature_extractor.py ...
  Validation: python -m py_compile model/backbone.py ✓

[worker-2] T-RES-02: Add bone length consistency loss
  Editing losses/bone_length.py (new file) ...
  Editing losses/__init__.py ...
  Validation: python -m py_compile losses/bone_length.py ✓
```

After workers finish, T-RES-03 (config updates) runs with full context from
both completed tasks. Worktree branches are merged and cleaned up automatically.

---

## Phase 4: Submit Training Job — Hub

With code ready, submit the experiment to the GPU cluster:

```python
c4_hub_submit(
    name="dinov2-boneloss-exp043",
    workdir="/git/pose-model",
    command="python train.py --config configs/exp043.yaml",
    gpu_count=1,
    exp_id="exp-043"
)
```

Monitor progress:

```
[job-8b2f1e] Epoch 1/80   @loss=2.1432  @bone_loss=0.8821  @mpjpe=45.12
[job-8b2f1e] Epoch 10/80  @loss=1.0341  @bone_loss=0.3214  @mpjpe=35.44
[job-8b2f1e] Epoch 40/80  @loss=0.4821  @bone_loss=0.1102  @mpjpe=26.88
[job-8b2f1e] Epoch 80/80  @loss=0.2941  @bone_loss=0.0634  @mpjpe=21.43
[job-8b2f1e] Status: SUCCEEDED
```

MPJPE dropped from 48mm (ResNet50) to 21.43mm (DINOv2 + bone loss).
Target of 25mm exceeded.

---

## Phase 5: Record Results

```python
c4_knowledge_record(
    type="experiment",
    content="""exp-043: DINOv2 ViT-L + bone length loss
    MPJPE: 21.43mm  PA-MPJPE: 15.92mm
    vs baseline exp-042 (ResNet50): MPJPE 22.71mm
    Key finding: bone loss contribution minor vs backbone upgrade.
    DINOv2 features alone account for ~8mm improvement.
    Next: ablate bone loss weight, try cross-attention head."""
)
```

---

## Phase 6: Research Loop — Autonomous Iteration

For sustained multi-cycle improvement, hand off to the Research Loop:

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
  Hypothesis: cross-attention between joint queries improves local features
  Plan: add CrossAttentionHead to decoder
  Execute: hub submit exp-044 ...
  Result: @mpjpe=19.87mm  (improvement: 1.56mm)
  Knowledge recorded.

[loop] Cycle 2 — best: 19.87mm
  Hypothesis: occlusion augmentation during training reduces test-time errors
  Plan: add OcclusionAug transform with p=0.3
  Execute: hub submit exp-045 ...
  Result: @mpjpe=18.24mm  (improvement: 1.63mm)
  Knowledge recorded.

[loop] Cycle 3 — best: 18.24mm
  Hypothesis: cosine LR schedule with warm restart improves convergence
  Execute: hub submit exp-046 ...
  Result: @mpjpe=17.61mm  (improvement: 0.63mm)
  Goal reached: 17.61mm < 18mm target.

[loop] Goal achieved after 3 cycles. Stopping.
```

Check loop status at any time:

```
/c9-loop status
```

```
Loop: running (cycle 3/5)
Best: 17.61mm (exp-046)
Experiments: exp-043, exp-044, exp-045, exp-046
Budget remaining: 2 cycles
```

---

## Phase 7: Finish and Release

```
/c4-finish
```

CQ runs final checks, generates a summary, and creates a release:

```
[finish] All validations passed
[finish] Generating research summary...

Research Summary — 3D Pose Estimation Improvement
  Starting MPJPE: 48.00mm (exp-042, ResNet50)
  Final MPJPE:    17.61mm (exp-046, DINOv2 + CA + OccAug)
  Improvement:    63% reduction in joint position error
  Experiments:    4 (3 autonomous loop cycles)
  Total GPU time: 6h 14m

Key findings (saved to knowledge base):
  1. DINOv2 backbone provides largest single gain (~8mm)
  2. Cross-attention head adds ~2mm improvement
  3. Occlusion augmentation critical for robustness
  4. Bone length loss: minor effect (< 0.5mm)

[finish] Commit created: feat(model): DINOv2+CA+OccAug — MPJPE 17.61mm
```

---

## Full Workflow in One View

```
/pi          →  Explore idea, surface prior knowledge
/c4-plan     →  EARS requirements + architecture decision + tasks
/c4-run 2    →  Parallel workers implement code changes
hub submit   →  GPU training, @metric streaming, artifact upload
/c9-loop     →  Autonomous multi-cycle improvement
/c4-finish   →  Summary, knowledge capture, commit
```

---

## Research Loop Configuration

```yaml
# .c4/config.yaml
hub:
  enabled: true
  url: https://hub.pilab.kr
  team_id: your-team-id

research_loop:
  max_cycles: 10
  metric: mpjpe
  direction: minimize    # or maximize
  early_stop_patience: 3 # stop if no improvement for 3 cycles
```

---

## Next Steps

- [Distributed ML Experiments](distributed-experiments.md) — Hub and DAG details
- [Usage Guide](../usage-guide.md) — full command reference including /c9-loop

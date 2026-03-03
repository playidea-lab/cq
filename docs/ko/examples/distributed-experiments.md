# 예시: 분산 실험

C5 Hub를 사용하여 여러 머신에서 동시에 ML 실험을 실행합니다.

::: info full 티어 필요
이 예시는 `full` 티어 바이너리와 C5 Hub 실행이 필요합니다. `/c4-standby`로 각 머신을 잡 큐에 연결합니다.
:::

## 시나리오

세 가지 모델 backbone을 비교하여 최적의 정확도/효율성 트레이드오프를 찾습니다.

## 1단계 — 실험 계획

> **You:** "backbone 3개 비교 실험 돌려야 해. ResNet / EfficientNet / ViT"

```
/c4-plan "backbone ablation: ResNet50 vs EfficientNet-B4 vs ViT-B/16"

  ● Tasks created
    T-020  train ResNet50       (GPU: 1x A100, 예상 4h)
    T-021  train EfficientNet   (GPU: 1x A100, 예상 3h)
    T-022  train ViT-B/16       (GPU: 1x A100, 예상 6h)
    T-023  결과 비교 + 리포트
```

## 2단계 — 각 머신에 워커 등록

각 머신에서 Claude Code를 열고 실행합니다:

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

워커가 잡을 자동으로 pull합니다 — 수동 할당 불필요.

## 3단계 — 진행 상황 모니터링

> **You:** "결과 어때?"

```
/c4-status

  ✓ T-020  ResNet50       MPJPE: 48.3mm  PA-MPJPE: 34.1mm  [worker-m1]
  ✓ T-021  EfficientNet   MPJPE: 44.7mm  PA-MPJPE: 31.8mm  [worker-m2]  ← best so far
  ▶ T-022  ViT-B/16       실행 중 3h 42m / ~6h               [worker-m3]
  ◷ T-023  T-020, T-021, T-022 대기 중

  Knowledge: 2 new experiment records saved
```

## 4단계 — 비교 리포트 (자동 생성)

T-022가 완료되면 T-023이 실행되어 다음을 생성합니다:

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

## 확장 방법

각 `/c4-standby` 세션은 동일한 C5 Hub 큐에서 잡을 pull하는 독립적인 워커입니다. 더 많은 세션을 열어 머신을 추가하면 큐가 자동으로 분산됩니다.

```
machine-1 ──┐
machine-2 ──┼── C5 Hub ── task queue ── /c4-plan 결과
machine-3 ──┘
  ...
machine-N ──
```

## 지식 루프

각 실험의 결과가 자동으로 지식 베이스에 기록됩니다. 다음 라운드의 실험을 계획할 때 CQ가 관련 과거 결과를 각 워커의 컨텍스트에 주입합니다 — 워커들이 무엇이 효과가 없었는지 이미 알고 시작합니다.

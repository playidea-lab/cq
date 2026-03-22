---
name: experiment-design
description: |
  딥러닝/ML 실험 설계 가이드. 가설 수립, 대조군, 데이터 분할, 하이퍼파라미터, 통계 검증까지
  체계적으로 실험을 설계합니다. 새 실험을 시작하거나 기존 실험을 개선할 때 이 스킬을 사용하세요.
  "실험 설계", "experiment design", "가설 검증", "ablation study", "베이스라인 비교",
  "하이퍼파라미터 탐색", "실험 계획" 등의 요청에 트리거됩니다.
---

# Experiment Design

딥러닝/ML 실험 설계 가이드.

## 트리거

"실험 설계", "experiment design", "가설 검증", "ablation study", "베이스라인 비교"

## Steps

### 1. 가설 수립

실험 전 반드시 작성:

```markdown
## 가설: [exp_id]
- **주장**: [X를 하면 Y가 개선될 것이다]
- **메트릭**: [측정할 지표 — MPJPE, accuracy, F1, loss 등]
- **기준**: [성공 조건 — baseline 대비 N% 개선, p < 0.05]
- **실패 시**: [가설이 틀리면 어떻게 할 것인가]
```

가설 없는 실험 = 탐색. 탐색도 가치 있지만 명시적으로 구분.

### 2. 베이스라인 설정

- 기존 SOTA 또는 이전 최고 결과를 베이스라인으로
- 베이스라인을 먼저 재현하고 시작 (재현 못 하면 비교 불가)
- 동일 데이터, 동일 전처리, 동일 평가 코드

### 3. 데이터 분할

```
전체 데이터
  ├── Train (70-80%)    ← 학습
  ├── Validation (10-15%) ← 하이퍼파라미터 튜닝
  └── Test (10-15%)      ← 최종 평가 (1회만 사용!)
```

**규칙:**
- Test set은 최종 보고 시 1회만 사용 (반복 접근 = data leakage)
- 시계열: 시간순 분할 (랜덤 분할 금지)
- 의료/소수 데이터: stratified split (클래스 비율 유지)
- Cross-validation: k=5 권장 (소규모 데이터)

### 4. 변수 통제

한 번에 하나만 변경:

| 실험 | 변경 사항 | 나머지 |
|------|----------|--------|
| exp001 | Baseline | - |
| exp002 | Learning rate 변경 | 나머지 동일 |
| exp003 | Augmentation 추가 | 나머지 동일 |

**동시에 2개 이상 변경하면 어떤 것이 효과인지 알 수 없다.**

### 5. 재현성 확보

```python
# 시드 고정
import torch, numpy as np, random
seed = 42
torch.manual_seed(seed)
np.random.seed(seed)
random.seed(seed)
torch.cuda.manual_seed_all(seed)
torch.backends.cudnn.deterministic = True
```

- [ ] 랜덤 시드 고정
- [ ] 데이터 버전 기록 (content hash)
- [ ] 코드 버전 기록 (git commit SHA)
- [ ] 환경 기록 (Python, PyTorch, CUDA 버전)
- [ ] uv.lock 커밋

### 6. 결과 보고

```markdown
## 실험 결과: [exp_id]

| 모델 | Metric1 | Metric2 | 비고 |
|------|---------|---------|------|
| Baseline | 45.2 ± 0.3 | 89.1 ± 0.5 | |
| Ours | **42.1 ± 0.4** | **91.3 ± 0.3** | -3.1mm |

- 실행 횟수: 3회 (mean ± std)
- 학습 시간: N GPU-hours
- 하이퍼파라미터: lr=1e-4, batch=32, epochs=100
```

### 7. Ablation Study

각 컴포넌트의 기여를 분리 검증:

```markdown
| 구성 | Metric | 비고 |
|------|--------|------|
| Full model | 42.1 | |
| - Component A | 44.5 | A가 -2.4 기여 |
| - Component B | 43.2 | B가 -1.1 기여 |
| - A - B | 46.8 | 둘 다 빼면 |
```

## CQ 연동 (CQ 프로젝트인 경우)

| 작업 | CQ 도구 |
|------|--------|
| 실험 결과 기록 | `c4_experiment_record` |
| 실험 루프 자동 실행 | `/c9-loop` |
| 실험 설계 토론 | `/c9-conference` |

## 안티패턴

- Test set으로 하이퍼파라미터 튜닝 (validation set 사용)
- mean만 보고하고 std 생략
- 1회 실행으로 결론 (최소 3회)
- 베이스라인 재현 없이 비교
- 실패한 실험 기록 안 함 (실패도 지식)

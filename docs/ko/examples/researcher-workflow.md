# 연구자 E2E 워크플로우

이 예제는 초기 아이디어부터 계획, 구현, GPU 훈련, 평가, 반복 개선까지 CQ를 사용한 완전한 ML 연구 사이클을 안내합니다. Research Loop는 사람의 개입 없이 여러 사이클을 자율적으로 실행할 수 있습니다.

**시나리오**: 3D 포즈 추정 모델 개선. 시작 메트릭: MPJPE 48mm. 목표: 25mm 미만.

---

## Phase 1: 아이디어 탐색 — /pi

코드 작성 전에 브레인스토밍부터 시작합니다. `/pi`는 구조화된 노트를 `idea.md`에 저장하는 자유 형식 탐색 모드입니다.

```
/pi "3D 포즈 추정 정확도 개선, 현재 MPJPE 48mm, 목표 <25mm"
```

CQ가 Play Idea 모드에 진입하여 질문하고, 지식 베이스에서 관련 지식을 찾아내고, 접근 방식을 구조화합니다:

```
[pi] 관련 지식 로딩 중...
  발견: "exp-038: DINOv2 특징이 H36M에서 MPJPE를 8mm 개선"
  발견: "인사이트: attention pooling이 joint 예측에서 average pooling보다 우수"

[pi] 방향 브레인스토밍:
  1. 백본 업그레이드 — ResNet50 대신 DINOv2 ViT-L
  2. 손실 함수 — 뼈 길이 일관성 손실 추가
  3. 데이터 증강 — 가림 시뮬레이션
  4. 아키텍처 — joint 특징 간 cross-attention

[pi] idea.md에 저장 중...
```

---

## Phase 2: 계획 — /c4-plan

아이디어를 구조화된 요구사항과 구체적인 태스크로 변환합니다:

```
/c4-plan
```

CQ가 Discovery → Design → 태스크 생성을 실행합니다:

```
[plan] DISCOVERY — 요구사항 수집 (EARS 형식)
  - H36M에서 훈련 시, 시스템이 MPJPE < 25mm을 달성해야 함
  - DINOv2 ViT-L 백본 사용 시, 시스템이 사전 훈련된 가중치를 로드해야 함
  - 뼈 길이 손실 가중치 > 0.1이면, 시스템이 joint 일관성을 적용해야 함
  - 시스템이 프레임당 200ms 추론을 초과하지 않아야 함

[plan] DESIGN — 아키텍처 결정
  옵션 A: 백본만 교체 (위험 낮음, 반복 빠름)
  옵션 B: 전체 아키텍처 재설계 (상한 높음, 2배 시간)
  선택: 옵션 A — 최소한으로 시작, 반복

[plan] TASKS 생성됨:
  T-RES-01  ResNet50 백본을 DINOv2 ViT-L로 교체
  T-RES-02  뼈 길이 일관성 손실 추가
  T-RES-03  훈련 설정 및 데이터 파이프라인 업데이트
  T-RES-04  기준선 평가 실행 (exp-043)
  CP-RES    체크포인트: exp-042 기준선(MPJPE 22.71mm) 대비 비교
```

---

## Phase 3: 구현 — /c4-run

태스크를 병렬로 실행할 Worker를 스폰합니다:

```
/c4-run 2
```

두 Worker가 독립적인 태스크를 동시에 시작합니다:

```
[worker-1] T-RES-01: DINOv2 ViT-L로 백본 교체
  model/backbone.py 편집 중...
  model/feature_extractor.py 편집 중...
  유효성 검사: python -m py_compile model/backbone.py ✓

[worker-2] T-RES-02: 뼈 길이 일관성 손실 추가
  losses/bone_length.py (새 파일) 편집 중...
  losses/__init__.py 편집 중...
  유효성 검사: python -m py_compile losses/bone_length.py ✓
```

Worker들이 완료된 후, T-RES-03(설정 업데이트)이 두 완료된 태스크의 전체 컨텍스트로 실행됩니다. 워크트리 브랜치가 자동으로 병합 및 정리됩니다.

---

## Phase 4: 훈련 작업 제출 — Hub

코드가 준비되면 GPU 클러스터에 실험을 제출합니다:

```python
c4_hub_submit(
    name="dinov2-boneloss-exp043",
    workdir="/git/pose-model",
    command="python train.py --config configs/exp043.yaml",
    gpu_count=1,
    exp_id="exp-043"
)
```

진행 상황 모니터링:

```
[job-8b2f1e] Epoch 1/80   @loss=2.1432  @bone_loss=0.8821  @mpjpe=45.12
[job-8b2f1e] Epoch 10/80  @loss=1.0341  @bone_loss=0.3214  @mpjpe=35.44
[job-8b2f1e] Epoch 40/80  @loss=0.4821  @bone_loss=0.1102  @mpjpe=26.88
[job-8b2f1e] Epoch 80/80  @loss=0.2941  @bone_loss=0.0634  @mpjpe=21.43
[job-8b2f1e] Status: SUCCEEDED
```

MPJPE가 48mm (ResNet50)에서 21.43mm (DINOv2 + 뼈 손실)으로 감소. 25mm 목표 달성.

---

## Phase 5: 결과 기록

```python
c4_knowledge_record(
    type="experiment",
    content="""exp-043: DINOv2 ViT-L + 뼈 길이 손실
    MPJPE: 21.43mm  PA-MPJPE: 15.92mm
    기준선 exp-042 (ResNet50) 대비: MPJPE 22.71mm
    핵심 발견: 뼈 손실 기여도는 백본 업그레이드 대비 미미함.
    DINOv2 특징만으로 ~8mm 개선.
    다음: 뼈 손실 가중치 절제 실험, cross-attention head 시도."""
)
```

---

## Phase 6: Research Loop — 자율 반복

지속적인 다중 사이클 개선을 위해 Research Loop에 넘깁니다:

```
/c9-loop start \
  --goal "MPJPE 18mm 미만 달성" \
  --budget 5 \
  --metric mpjpe \
  --direction minimize
```

LoopOrchestrator가 자율적으로 실행합니다:

```
[loop] 사이클 1 — 최적: 21.43mm
  가설: joint 쿼리 간 cross-attention이 로컬 특징 개선
  계획: 디코더에 CrossAttentionHead 추가
  실행: hub submit exp-044 ...
  결과: @mpjpe=19.87mm  (개선: 1.56mm)
  지식 기록됨.

[loop] 사이클 2 — 최적: 19.87mm
  가설: 훈련 중 가림 증강이 테스트 시간 오류 감소
  계획: p=0.3인 OcclusionAug 변환 추가
  실행: hub submit exp-045 ...
  결과: @mpjpe=18.24mm  (개선: 1.63mm)
  지식 기록됨.

[loop] 사이클 3 — 최적: 18.24mm
  가설: 웜 재시작이 있는 코사인 LR 스케줄이 수렴 개선
  실행: hub submit exp-046 ...
  결과: @mpjpe=17.61mm  (개선: 0.63mm)
  목표 달성: 17.61mm < 18mm 목표.

[loop] 3 사이클 후 목표 달성. 중지.
```

언제든지 루프 상태 확인:

```
/c9-loop status
```

```
루프: 실행 중 (사이클 3/5)
최적: 17.61mm (exp-046)
실험: exp-043, exp-044, exp-045, exp-046
남은 예산: 2 사이클
```

---

## Phase 7: 마무리 및 릴리즈

```
/c4-finish
```

CQ가 최종 검사를 실행하고, 요약을 생성하고, 릴리즈를 만듭니다:

```
[finish] 모든 유효성 검사 통과
[finish] 연구 요약 생성 중...

연구 요약 — 3D 포즈 추정 개선
  시작 MPJPE: 48.00mm (exp-042, ResNet50)
  최종 MPJPE:    17.61mm (exp-046, DINOv2 + CA + OccAug)
  개선:    joint 위치 오류 63% 감소
  실험:    4회 (자율 루프 사이클 3회)
  총 GPU 시간: 6시간 14분

핵심 발견 (지식 베이스에 저장):
  1. DINOv2 백본이 가장 큰 단일 성능 향상 제공 (~8mm)
  2. Cross-attention head가 ~2mm 추가 개선
  3. 가림 증강이 견고성에 중요
  4. 뼈 길이 손실: 미미한 효과 (< 0.5mm)

[finish] 커밋 생성됨: feat(model): DINOv2+CA+OccAug — MPJPE 17.61mm
```

---

## 한눈에 보는 전체 워크플로우

```
/pi          →  아이디어 탐색, 이전 지식 활용
/c4-plan     →  EARS 요구사항 + 아키텍처 결정 + 태스크
/c4-run 2    →  병렬 Worker가 코드 변경 구현
hub submit   →  GPU 훈련, @메트릭 스트리밍, 아티팩트 업로드
/c9-loop     →  자율 다중 사이클 개선
/c4-finish   →  요약, 지식 캡처, 커밋
```

---

## Research Loop 설정

```yaml
# .c4/config.yaml
hub:
  enabled: true
  url: https://hub.pilab.kr
  team_id: your-team-id

research_loop:
  max_cycles: 10
  metric: mpjpe
  direction: minimize    # 또는 maximize
  early_stop_patience: 3 # 3 사이클 동안 개선 없으면 중지
```

---

## 다음 단계

- [분산 ML 실험](distributed-experiments.md) — Hub와 DAG 상세
- [사용 가이드](../usage-guide.md) — /c9-loop를 포함한 전체 커맨드 레퍼런스

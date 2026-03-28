# 분산 ML 실험

이 예제는 노트북에서 GPU 훈련 작업을 제출하고, CQ의 relay(NAT 통과 포함)를 통해 원격 Worker가 작업을 처리하고, 메트릭을 실시간으로 스트리밍하고, 결과를 지식 베이스에 수집하는 방법을 보여줍니다.

---

## 사전 요구사항

- CQ 설치 및 로그인 완료 (`cq`)
- `cq serve` 실행 중 (relay 연결 시작)
- `cq worker start`로 연결된 원격 Worker 최소 1개
- 로컬에 훈련 스크립트 준비 (예: `train.py`)

---

## 아키텍처 개요

```
노트북                    Fly.io Relay            원격 GPU 머신
┌──────────────┐          ┌──────────┐          ┌─────────────────┐
│ cq hub submit│──WebSocket►│  Relay   │──WebSocket►│ cq worker start │
│              │          └──────────┘          │                 │
│ cq hub watch │◄──────────── 메트릭 스트림 ─────│ train.py 출력   │
└──────────────┘                                │  @loss=0.312    │
                                                │  @mpjpe=24.3    │
                                                └─────────────────┘
```

Relay가 NAT 통과를 자동으로 처리합니다 — 양쪽 모두 포트 포워딩이 필요 없습니다.

---

## 1단계: Worker 가용성 확인

제출 전에 Worker가 온라인인지 확인합니다:

```bash
cq hub workers
```

```
WORKER ID         STATUS    GPU          LAST SEEN
worker-gpu-a100   online    A100 80GB    2s ago
worker-rtx4090    online    RTX 4090     8s ago
```

---

## 2단계: 훈련 작업 제출

```bash
cq hub submit \
  --name "resnet50-baseline" \
  --workdir /home/user/projects/my-model \
  --command "python train.py --epochs 50 --lr 0.001" \
  --gpu \
  --exp-id exp-042
```

CQ가 사용 가능한 GPU Worker에 작업을 할당합니다:

```
[hub] 작업 제출됨: job-7f3a9c
[hub] 할당됨: worker-gpu-a100
[hub] 상태: queued → running (2s)
[hub] 출력 스트리밍 중...
```

---

## 3단계: 실시간 메트릭 스트리밍

훈련 스크립트가 `@key=value` 규칙으로 메트릭을 출력합니다:

```python
# train.py
for epoch in range(num_epochs):
    loss = train_one_epoch(model, loader)
    mpjpe = evaluate(model, val_loader)
    print(f"Epoch {epoch+1}/{num_epochs}  @loss={loss:.4f}  @mpjpe={mpjpe:.2f}")
```

CQ의 MetricWriter가 stdout을 자동으로 파싱합니다. SDK 변경이 필요 없습니다.
메트릭이 실시간으로 Supabase `experiment_checkpoints`에 저장됩니다.

노트북의 라이브 출력:

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

## 4단계: 아티팩트가 Drive에 자동 업로드

작업이 완료되면 workdir의 `artifacts/`에 기록된 파일이 자동으로 CQ Drive에 업로드됩니다:

```
[hub] 아티팩트 업로드 중...
  checkpoints/epoch_50.pth    (142 MB)  ✓  TUS 재개 가능 업로드
  results/metrics.json        (2 KB)    ✓
  logs/train.log              (88 KB)   ✓
[hub] 아티팩트 저장됨: drive://exp-042/
```

Drive는 내용 주소 지정 스토리지를 사용합니다 — 실험을 재실행해도 같은 체크포인트는 두 번 업로드되지 않습니다.

---

## 5단계: 결과 검토

```bash
cq hub summary job-7f3a9c
```

```
작업:       job-7f3a9c  (resnet50-baseline)
상태:       SUCCEEDED
시간:       1시간 23분
Worker:     worker-gpu-a100 (A100 80GB)

메트릭 (최종 에포크):
  loss:    0.2103
  mpjpe:   22.71 mm
  pa_mpjpe: 16.84 mm

아티팩트:
  checkpoints/epoch_50.pth
  results/metrics.json

Exp ID: exp-042
```

---

## 6단계: 결과를 지식으로 기록

```bash
# Claude Code 세션에서
c4_knowledge_record(
  type="experiment",
  content="exp-042: ResNet50 기준선 — MPJPE 22.71mm / PA-MPJPE 16.84mm.
           LR=0.001, epochs=50, A100 80GB, 1시간 23분.
           아티팩트: drive://exp-042/checkpoints/epoch_50.pth"
)
```

이 지식은 CQ 계정에 연결된 모든 AI 플랫폼의 이후 모든 세션에서 검색 가능합니다.

---

## 다단계 파이프라인 실행 (DAG)

전처리 → 훈련 → 평가 단계가 있는 실험의 경우:

```python
# Claude Code에서
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

그 다음 실행:

```python
c4_hub_dag_execute(dag_id="full-training-pipeline")
```

CQ가 의존성 순서로 단계를 실행합니다. 여러 Worker가 있으면 독립적인 단계가 병렬로 실행됩니다.

---

## @key=value 메트릭 규칙

stdout의 어떤 `@key=value` 패턴이든 자동으로 캡처됩니다.

| 출력 줄 | 캡처된 메트릭 | 값 |
|---------|------------|---|
| `@loss=0.312` | loss | 0.312 |
| `@mpjpe=24.3` | mpjpe | 24.3 |
| `@acc=0.943` | acc | 0.943 |
| `@hd_gt=1.74 @recall=0.88` | hd_gt, recall | 1.74, 0.88 |

한 줄에 여러 메트릭도 모두 캡처됩니다. 특별한 라이브러리가 필요 없습니다.

---

## 다음 단계

- [연구자 E2E 워크플로우](researcher-workflow.md) — 자동 사이클이 있는 전체 Research Loop
- [사용 가이드](../usage-guide.md) — Hub 커맨드 레퍼런스

---
name: experiment-workflow
description: |
  End-to-end experiment lifecycle management: data preparation → code setup →
  training execution → result collection → artifact preservation → knowledge recording.
  Integrates Drive/Dataset for cross-server data and artifact sharing.
  Use PROACTIVELY when the user wants to run ML experiments, train models, prepare
  training data, submit GPU jobs, collect experiment results, or manage experiment
  artifacts across servers. Also triggers on: "실험 실행", "학습 시작", "모델 학습",
  "데이터 준비", "GPU 잡", "experiment", "train model", "run training",
  "prepare dataset", "submit job", "pull data", "push results".
allowed-tools: Bash, Read, Write, Edit, Agent, mcp__cq__*
---

# Experiment Workflow

> 실험의 전체 생명주기를 관리한다.
> 데이터 준비 → 코드 셋업 → 실행 → 결과 수집 → 아티팩트 보존 → 지식 기록.

## 핵심 원칙

1. **서버 독립적**: GPU 서버, 클라우드, 고객 머신 등 어디서든 동일한 워크플로우
2. **Drive가 SSOT**: 데이터셋, 체크포인트, 결과물은 Drive에 보존해야 세션/서버 간 공유 가능
3. **한 번 기록, 어디서든 재사용**: `c4_experiment_record`로 기록하면 Knowledge DB에 영구 보존

---

## Phase 1: 데이터 준비

### 1.1 기존 데이터셋 확인

```
c4_drive_dataset_list()
→ [{name: "h36m-processed", version_hash: "a1b2...", file_count: 1200}, ...]
```

### 1.2 데이터셋 Pull (서버에 데이터 가져오기)

서버에 데이터가 없으면 Drive에서 pull한다.
이미 있는 파일은 해시 비교로 스킵되므로 부담 없이 실행 가능.

```
c4_drive_dataset_pull(
  name: "<dataset_name>",
  dest: "<local_data_path>",    # e.g., "./data" or "/home/pi/data"
  version: "<version_prefix>"    # 생략하면 최신 버전
)
→ {files_downloaded: 5, files_skipped: 1195, version_hash: "a1b2..."}
```

버전 prefix로 특정 스냅샷을 지정할 수 있다 (예: `version: "a1b2"`).

### 1.3 새 데이터셋 등록

전처리한 데이터를 Drive에 올려서 다른 서버에서도 쓸 수 있게 한다.

```
c4_drive_dataset_upload(
  path: "<local_data_dir>",     # 디렉토리 경로
  name: "<dataset_name>"        # 이름 (버전은 자동 생성)
)
→ {version_hash: "c3d4...", files_uploaded: 50, files_skipped: 1150, changed: true}
```

변경이 없으면 `changed: false` — 안전하게 반복 실행 가능.

### 1.4 개별 파일 (설정, 가중치 등)

```
# 다운로드
c4_drive_download(drive_path: "/models/pretrained/dino_v2.pt", local_path: "./weights/dino_v2.pt")

# 업로드
c4_drive_upload(local_path: "./configs/train_v3.yaml", drive_path: "/configs/train_v3.yaml")
```

---

## Phase 2: 코드 준비

### 2.1 메트릭 어노테이션 (필수)

학습 스크립트의 `print()` 출력에 `@key=value` 패턴을 포함해야 한다.
CQ MetricWriter가 stdout에서 자동 파싱하여 실시간 추적한다.

```python
# 학습 루프 안에서
print(f'Epoch {epoch} @loss={loss:.4f} @val_loss={val_loss:.4f} @lr={lr:.6f}')

# 평가 후
print(f'Eval @mpjpe={mpjpe:.2f} @pa_mpjpe={pa_mpjpe:.2f} @pck={pck:.4f}')
```

규칙:
- `@`로 시작, `=` 뒤에 숫자 (정수 또는 소수)
- 한 줄에 여러 메트릭 가능
- key 이름은 영문 소문자 + 언더스코어
- 기존 로깅(wandb, tensorboard 등)과 병행 가능 — print만 추가하면 됨

### 2.2 체크포인트 저장 경로

스크립트가 체크포인트를 저장하는 경로를 기록해둔다.
Phase 4에서 이 경로의 파일을 Drive에 올린다.

```python
CHECKPOINT_DIR = "./checkpoints"
RESULT_DIR = "./results"

# 학습 완료 시
torch.save(model.state_dict(), f"{CHECKPOINT_DIR}/best_model.pt")
# 평가 결과
json.dump(metrics, open(f"{RESULT_DIR}/eval_results.json", "w"))
```

---

## Phase 3: 실험 실행

### 3.0 역할 분담 (v1.38.0+)

| 수단 | 용도 | 비유 |
|------|------|------|
| **relay** (`cq_relay_call`) | 파일 읽기/쓰기, 상태 확인, 짧은 명령 (<30초) | SSH 세션 |
| **Hub** (`cq hub submit`) | 학습, 빌드, 장시간 실험 큐잉 (게시판) | Job Queue |
| **Git** | 코드 버전 관리 | git push/pull |
| **Drive** | 데이터/체크포인트 버전 관리 (DVC 패턴) | S3/DVC |

### 3.1 원격 서버 확인 (relay)

```
cq_workers()
→ [{id: "pi-gpu", status: "connected"}, {id: "desktop", status: "connected"}]

cq_relay_call(worker_id: "pi-gpu", tool: "c4_gpu_status", args: {})
→ {gpus: [{name: "RTX 5080", free_vram_gb: 15.6}]}
```

### 3.2 코드 배포 (Git + relay)

```
# Mac에서 push
git add . && git commit -m "feat: new loss" && git push

# GPU에서 pull (relay 경유)
cq_relay_call(worker_id: "pi-gpu", tool: "c4_execute",
  args: {command: "cd /home/pi/exp-repo && git pull"})
```

### 3.3 데이터 준비 (Drive → GPU)

```
cq_relay_call(worker_id: "pi-gpu", tool: "c4_execute",
  args: {command: "cq drive dataset pull h36m-processed --dest /data/"})
```

### 3.4 실험 실행 — Hub 게시판 패턴 (권장)

```
cq hub submit "cd /home/pi/exp && python train.py --config exp001.yaml" --tag gpu
cq hub submit "cd /home/pi/exp && python train.py --config exp002.yaml" --tag gpu
cq hub submit "cd /home/pi/exp && python train.py --config exp003.yaml" --tag gpu
```

GPU 서버의 `cq serve`가 `hub.auto_worker: true`이면 자동으로 순차 실행.

중간에 확인 (relay):
```
cq_relay_call(worker_id: "pi-gpu", tool: "c4_execute",
  args: {command: "tail -5 /home/pi/exp/train.log"})
```

### 3.5 즉시 실행 (단발, <30초)

```
cq_relay_call(worker_id: "pi-gpu", tool: "c4_execute",
  args: {command: "cd /home/pi/exp && python eval.py --checkpoint best.pt"})
```

### 3.6 잡 상태 확인

```
cq hub list
→ exp001  SUCCEEDED, exp002  FAILED (OOM), exp003  RUNNING
```

---

## Phase 4: 결과 수집 + 아티팩트 보존

실험 완료 후 반드시 수행하는 3단계.

### 4.1 결과 기록 + 아티팩트 업로드 (한 번에)

```
c4_experiment_record(
  title: "exp042: MPJPE=45.2mm, lr=1e-3, batch=64",
  content: |
    ## Config
    - Model: DINOv3-Light
    - LR: 1e-3, Batch: 64, Epochs: 100
    - Dataset: h36m-processed (version a1b2)

    ## Results
    - MPJPE: 45.2mm (baseline 52.1mm, -13.2%)
    - PA-MPJPE: 38.1mm
    - Training time: 4h23m on RTX 5080

    ## Artifacts
    - Model: /experiments/exp042/best_model.pt
    - Results: /experiments/exp042/eval_results.json
  tags: ["paper1", "dinov3", "h36m"],
  artifacts: [
    "./checkpoints/best_model.pt",
    "./results/eval_results.json"
  ]
)
```

`artifacts` 파라미터에 로컬 파일 경로를 넣으면:
- 자동으로 Drive에 업로드 (`/experiments/<title_slug>/<filename>`)
- 응답에 Drive 경로 반환
- Knowledge DB에 실험 기록 + 벡터 임베딩 → 나중에 검색 가능

### 4.2 대량 결과물 (디렉토리 통째로)

체크포인트, 로그, 텐서보드 파일 등 디렉토리 전체를 보존:

```
c4_drive_dataset_upload(
  path: "./experiments/exp042/",
  name: "exp042-outputs"
)
```

다른 서버에서 복원:
```
c4_drive_dataset_pull(name: "exp042-outputs", dest: "./experiments/exp042/")
```

### 4.3 Best model만 별도 보관

```
c4_drive_upload(
  local_path: "./checkpoints/best_model.pt",
  drive_path: "/models/<project>/best_model.pt",
  metadata: {"mpjpe": 45.2, "experiment": "exp042", "dataset": "h36m-processed"}
)
```

---

## Phase 5: 지식 기록

### 5.1 성공/실패 패턴 기록

실험에서 배운 교훈을 기록하면 다음 실험 계획에 자동으로 제공된다.

```
c4_knowledge_record(
  doc_type: "pattern",
  title: "LR warmup이 DINOv3 수렴에 필수",
  content: |
    exp041(warmup 없음)은 발산, exp042(cosine warmup 1000 steps)는 수렴.
    DINOv3 계열에서는 LR warmup을 빼면 안 된다.
  tags: ["dinov3", "lr-schedule", "training-tip"]
)
```

### 5.2 다음 실험 시 과거 지식 활용

```
c4_knowledge_search(query: "DINOv3 training tips")
→ 이전에 기록한 패턴/실험 결과가 자동 추천
```

```
c4_experiment_search(query: "h36m MPJPE baseline")
→ 과거 실험들의 메트릭 비교
```

---

## Quick Reference: 도구 매핑

| 하고 싶은 일 | 도구 |
|------------|------|
| 데이터셋 가져오기 | `c4_drive_dataset_pull(name, dest)` |
| 데이터셋 올리기 | `c4_drive_dataset_upload(path, name)` |
| 파일 하나 받기 | `c4_drive_download(drive_path, local_path)` |
| 파일 하나 올리기 | `c4_drive_upload(local_path, drive_path)` |
| GPU 잡 제출 | `c4_job_submit(command, gpu_count)` |
| 잡 상태 확인 | `c4_job_status(job_id)` |
| 실험 기록 + 파일 업로드 | `c4_experiment_record(title, content, artifacts=[...])` |
| 패턴/인사이트 기록 | `c4_knowledge_record(doc_type, title, content)` |
| 과거 실험 검색 | `c4_experiment_search(query)` |
| 과거 패턴 검색 | `c4_knowledge_search(query)` |
| 데이터셋 목록 | `c4_drive_dataset_list()` |
| Drive 파일 목록 | `c4_drive_list(path)` |

---

## 서브커맨드

| 명령 | 설명 |
|------|------|
| `/experiment-workflow setup` | Phase 1-2: 데이터 pull + 코드 점검 (메트릭 어노테이션 확인) |
| `/experiment-workflow run` | Phase 3: 잡 제출 |
| `/experiment-workflow collect` | Phase 4-5: 결과 수집 + Drive 업로드 + Knowledge 기록 |
| `/experiment-workflow` (인자 없음) | 상황 파악 후 다음 단계 자동 제안 |

### 인자 없음 (자동 감지)

1. `c4_job_list()` → 실행 중인 잡 있으면 상태 보여주고 대기
2. 완료된 잡 있으면 → collect 제안
3. 잡 없으면 → setup부터 시작

### setup

1. 프로젝트 디렉토리 확인 (train 스크립트, config 등)
2. `c4_drive_dataset_list()` → 관련 데이터셋 제안
3. 데이터 없으면 → `c4_drive_dataset_pull` 안내
4. 학습 스크립트에 `@key=value` 메트릭 어노테이션이 있는지 검사
   - 없으면 자동 추가 제안 (print문에 `@loss=`, `@acc=` 등)

### run

1. 실행 커맨드 확인 (또는 config 파일에서 추론)
2. GPU 필요 여부 판단
3. `c4_job_submit` 또는 직접 실행
4. job_id 기록

### collect

1. `c4_job_status` → 완료 확인
2. 결과 파일 탐색 (checkpoints/, results/, logs/)
3. `c4_experiment_record(artifacts=[...])` → 기록 + Drive 업로드
4. 유의미한 패턴 발견 시 `c4_knowledge_record` 제안

# 연구자 워크플로우

아이디어부터 결과까지 — CQ로 ML 실험을 end-to-end로 실행하는 가이드.

::: info full 티어 필요
연구자 워크플로우는 C5 Hub (분산 잡)과 C9 Knowledge (실험 추적)를 사용합니다. 둘 다 `full` 티어가 필요합니다.
:::

## 개요

```
아이디어 → /pi → /c4-plan → /c4-run → cq hub submit → 결과 → c4_experiment_record
```

CQ가 전체 연구 루프를 처리합니다: 아이디어 탐색, 태스크 분해, 분산 실행, 지식 캡처.

---

## 1단계: 아이디어 탐색

코드 작성 전에 `/pi`로 브레인스토밍합니다:

```
/pi
```

`/pi`는 아이디어 탐색 모드입니다 — 발산, 수렴, 리서치, 토론. 준비되면 자동으로 `/c4-plan`을 실행합니다.

---

## 2단계: 실험 계획

연구 목표를 설명합니다:

```
/c4-plan "cosine LR 스케줄로 CIFAR-10에 ResNet-50 학습"
```

CQ가:
1. 명확화 질문을 합니다 (Discovery)
2. 실험 구조를 설계합니다 (Design)
3. DoD가 있는 태스크로 분해합니다
4. 태스크 큐를 생성합니다

---

## 3단계: 로컬 구현

```
/c4-run
```

워커가 격리된 git 워크트리에서 코드를 구현합니다. 완료되면 `/c4-run`이 자동으로 polish → finish를 실행합니다.

---

## 4단계: Hub에 제출 (분산 실행)

코드가 준비되면 GPU 워커에 제출합니다:

```sh
cq hub submit --run "python train.py" --project my-experiment
```

이 명령이:
1. 프로젝트를 Drive CAS에 스냅샷합니다
2. C5 Hub에 잡을 등록합니다
3. GPU 워커가 잡을 pull하고, 스냅샷을 다운로드해 실행하고, 결과를 업로드합니다

`cq.yaml`에 입출력을 선언합니다:

```yaml
run: python train.py

artifacts:
  input:
    - name: cifar10-mini
      local_path: data/cifar10
  output:
    - name: model-checkpoint
      local_path: checkpoints/
```

진행 상황 모니터링:

```sh
cq hub list
cq hub status <job-id>
```

---

## 5단계: 결과 기록

실험이 완료되면 C9 Knowledge에 메트릭을 기록합니다:

```
c4_experiment_record(exp_id="exp001", metrics={"accuracy": 0.923, "loss": 0.21})
```

또는 Claude Code에서:

```
exp001 실험 기록: accuracy=0.923, loss=0.21, epochs=50
```

과거 실험 검색:

```
c4_experiment_search(query="ResNet CIFAR-10 cosine LR")
```

---

## 6단계: 반복

```
/c4-plan "label smoothing과 mixup augmentation 적용"
```

CQ가 모든 반복을 지식 베이스에 추적합니다. 새 실험 시작 전에 `c4_experiment_search`로 최적의 실행을 찾으세요.

---

## 전체 예시: CIFAR-10 학습

```sh
# 1. 프로젝트 초기화
cq claude

# 2. 계획
# Claude Code에서: /c4-plan "CIFAR-10 ResNet-50 베이스라인"

# 3. 구현
# Claude Code에서: /c4-run

# 4. Hub에 제출
cq hub submit --run "python train.py --epochs 100"

# 5. 모니터링
cq hub list

# 6. 결과 기록 (Claude Code에서)
# c4_experiment_record(exp_id="exp001", metrics={"top1_acc": 0.923})
```

---

## 팁

- 새 실험 시작 전에 `c4_knowledge_search`로 유사한 실험이 이미 실행됐는지 확인하세요
- `cq.yaml`을 프로젝트 루트에 유지하세요 — Hub 통합을 위한 코드 변경 불필요
- 워커는 stateless: 모든 자격증명과 설정이 잡 payload로 전달됩니다
- `cq hub watch <job-id>`로 실시간 로그를 스트리밍하세요

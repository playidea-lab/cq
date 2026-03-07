# 예시: 분산 실험

노트북에서 ML 실험을 제출하고 원격 GPU 서버에서 실행합니다 — 학습 중 서버에 SSH 접속 불필요.

::: info full 티어 필요
이 예시는 `full` 티어 바이너리, 실행 중인 C5 Hub, 하나 이상의 연결된 워커가 필요합니다. [원격 워커 설정](/ko/guide/worker-setup) 참고.
:::

## 개요

```
내 노트북                     C5 Hub                  GPU 서버
────────────                  ──────                  ──────────
train.py + cq.yaml 작성 ──►  잡 큐     ◄──────────   c5 worker (실행 중)
cq hub submit                (스냅샷 저장,             잡 pull
                              잡 등록)                 train.py 실행
                                                       결과 업로드
```

GPU 서버는 백그라운드에서 `c5 worker`를 실행합니다. 학습을 시작하기 위해 SSH 접속할 필요가 없습니다 — 그냥 submit하면 됩니다.

---

## 실험 폴더 구성

**노트북에서** 실험 디렉토리를 만듭니다:

```
mnist-exp/
├── train.py       # 학습 스크립트
├── cq.yaml        # CQ 아티팩트 선언
└── requirements.txt
```

**`cq.yaml`** — 실행할 것과 사용할 데이터를 선언합니다:

```yaml
run: python train.py

artifacts:
  input:
    - name: mnist_mini        # Drive 데이터셋 이름
      local_path: data/mnist  # 실행 전 데이터를 놓을 위치
  output:
    - name: mnist-checkpoint  # 결과를 저장할 Drive 데이터셋 이름
      local_path: checkpoints/ # 실행 후 업로드할 로컬 경로
```

**`train.py`** — 순수 Python, CQ 코드 전혀 필요 없음:

```python
import torch
from pathlib import Path

# data/와 checkpoints/는 이미 준비됨 — 워커가 미리 배치
data_path = Path("data/mnist")
# ... 학습 로직 ...
torch.save(model.state_dict(), "checkpoints/model.pt")
print("MPJPE: 42.3")
```

`import cq`, SDK, 기존 스크립트 수정 — 아무것도 필요하지 않습니다.

## 데이터셋 업로드 (최초 1회)

`mnist_mini`를 아직 업로드하지 않았다면:

```sh
cq drive dataset upload ./data/mnist --as mnist_mini -y
```

CQ는 Content-Addressed Storage를 사용합니다 — 같은 데이터는 두 번 업로드되지 않습니다.

## 잡 제출

`mnist-exp/` 폴더 안에서:

```sh
cq hub submit
```

CQ가 수행하는 작업:
1. 현재 폴더를 스냅샷 (Drive CAS, 콘텐츠 해시로 중복 제거)
2. 잡 등록: `{ command, snapshot_version_hash, project_id }`
3. Hub가 다음 가용 워커에게 배분

출력:

```
✓ Snapshot uploaded  hash=a3f8c1d2
✓ Job submitted      job-id=job-abc123  status=QUEUED  position=1
```

이게 전부입니다. 노트북을 닫아도 됩니다. GPU 서버가 나머지를 처리합니다.

## 워커가 하는 일

워커가 잡을 받으면:

1. 정확한 스냅샷 다운로드 (`a3f8c1d2` → `mnist-exp/` 내용)
2. `cq.yaml` 읽기
3. Drive에서 `mnist_mini` pull → `data/mnist/`
4. `python train.py` 실행
5. `checkpoints/` → Drive의 `mnist-checkpoint`에 업로드

## 상태 확인

```sh
cq hub status job-abc123
```

또는 실시간으로:

```sh
cq hub watch job-abc123
```

```
job-abc123  RUNNING  [gpu-server-1]
  ▶ epoch 1/10 — loss: 0.4821
  ▶ epoch 2/10 — loss: 0.3102
  ...
```

## 결과 다운로드

```sh
cq drive dataset download mnist-checkpoint ./results/
```

## 여러 잡 동시 제출 (병렬)

변형 실험을 연달아 제출합니다 — 각각 별도 워커에서 실행됩니다:

```sh
cq hub submit --run "python train.py --lr 0.01" 
cq hub submit --run "python train.py --lr 0.001"
cq hub submit --run "python train.py --lr 0.0001"
```

워커가 여러 개 연결되어 있으면 세 가지 모두 동시에 실행됩니다.

---

## 워커 추가

더 많은 머신에서 `c5 worker`를 실행하여 GPU 용량을 확장합니다. Hub가 자동으로 잡을 분산합니다 — 별도 설정 불필요.

```
machine-1 ──┐
machine-2 ──┼── C5 Hub ── 잡 큐 ◄── cq hub submit
machine-3 ──┘
  ...
```

각 워커는 stateless — 설치, 로그인, `c5 worker` 시작만 하면 됩니다. [원격 워커 설정](/ko/guide/worker-setup) 참고.

## 지식 루프

실험 결과가 자동으로 CQ 지식 베이스에 기록됩니다. 다음 라운드를 계획할 때 CQ가 관련 과거 결과를 컨텍스트에 주입합니다 — 같은 실수를 반복하지 않게 됩니다.

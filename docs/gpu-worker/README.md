# GPU Worker — C5 Hub 연결 가이드

GPU 서버를 C5 Hub 워커로 연결합니다.

## 필수 조건

| 항목 | 필수 여부 | 비고 |
|------|----------|------|
| `cq` 바이너리 | 필수 | [설치 방법](#cq-설치) |
| `nvidia-smi` | 선택 | GPU 없는 환경에서는 CPU-only 워커로 동작 |

## cq 설치

```bash
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
```

## 빠른 시작 (JWT 인증 — 권장)

API key 설정 없이 2단계로 워커를 시작합니다.

```bash
# 1. 로그인 (브라우저에서 인증)
cq auth login --device

# 2. 워커 시작 (JWT 자동 감지, 만료 시 자동 refresh)
cq hub worker start
```

끝입니다. JWT가 만료되면 자동으로 refresh 후 워커가 재시작됩니다.

## API Key 인증 (대안)

JWT 대신 API key를 직접 사용할 수도 있습니다.

```bash
# 방법 A: 환경변수
export C5_API_KEY="your-api-key"
cq hub worker start

# 방법 B: init (영구 저장)
cq hub worker init   # 대화형: Hub URL + API key 입력 → ~/.c5/config.yaml
cq hub worker start
```

## Capability 설정 (선택)

커스텀 capability를 등록하려면 `gpu-caps.yaml`을 작성합니다.

```yaml
capabilities:
  - name: gpu_status
    description: "GPU 상태 조회"
    command: scripts/gpu-status.sh
    input_schema:
      type: object
      properties: {}

  - name: gpu_train
    description: "GPU 학습 실행"
    command: scripts/gpu-train.sh
    input_schema:
      type: object
      properties:
        script:
          type: string
          description: "실행할 Python 스크립트 경로"
        args:
          type: string
          description: "추가 인수"

  - name: gpu_infer
    description: "GPU 추론 실행"
    command: scripts/gpu-infer.sh
    input_schema:
      type: object
      properties:
        script:
          type: string
        args:
          type: string
```

```bash
# capability 파일 지정하여 워커 시작
cq hub worker init
# → Capabilities 경로 입력: gpu-caps.yaml
cq hub worker start
```

## 스크립트 동작 방식

### gpu-status.sh

`nvidia-smi` 결과를 JSON으로 출력합니다.

- **정상**: `{"gpu_count": 2, "gpus": [{"id": 0, "name": "A100", "memory_total": 81920, "memory_free": 80000, "utilization": 0}]}`
- **nvidia-smi 없음**: `{"gpu_count": 0, "gpus": [], "note": "nvidia-smi not found"}`

### gpu-train.sh / gpu-infer.sh

C5 워커 프로토콜을 따릅니다:

- **입력**: `C5_PARAMS` 환경변수 (JSON) — `{"script": "train.py", "args": "--epochs 10"}`
- **출력**: `C5_RESULT_FILE` 경로에 `{"exit_code": 0, "output": "..."}` JSON 저장

## systemd 서비스 등록 (선택)

```bash
# 자동으로 systemd/launchd 서비스 파일 생성
cq hub worker install

# 또는 미리보기만
cq hub worker install --dry-run
```

수동 설정이 필요한 경우:

```ini
# /etc/systemd/system/cq-worker.service
[Unit]
Description=CQ Hub Worker
After=network.target

[Service]
User=ubuntu
WorkingDirectory=/opt/gpu-worker
ExecStart=/usr/local/bin/cq hub worker start
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now cq-worker
sudo journalctl -fu cq-worker
```

> **Note**: `cq hub worker start`는 JWT 또는 `~/.c5/config.yaml`의 API key를 자동으로 사용합니다. systemd 환경에서 JWT를 쓰려면 해당 유저로 `cq auth login --device`를 먼저 실행하세요.

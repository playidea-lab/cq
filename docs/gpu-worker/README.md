# GPU Worker — C5 Hub 연결 가이드

GPU 서버를 C5 Hub 워커로 연결합니다. 1-tier Docker 모델로 동작: 워커가 호스트에서 실행되며, `runtime.image`가 설정된 잡은 `docker run`으로 컨테이너 실행합니다 (DinD 불필요).

## 필수 조건

| 항목 | 필수 여부 | 비고 |
|------|----------|------|
| `cq` 바이너리 | 필수 | [설치 방법](#cq-설치) |
| Docker | 권장 | `cq hub worker install`이 자동 설치 |
| NVIDIA Container Toolkit | 선택 | GPU 잡 실행 시 필요; `cq hub worker install`이 자동 설치 |
| `nvidia-smi` | 선택 | GPU 없는 환경에서는 CPU-only 워커로 동작 |

## cq 설치

```bash
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
```

## 빠른 시작 — `cq hub worker install` (권장)

Docker, NVIDIA toolkit, systemd 서비스를 한 번에 설치합니다.

```bash
# 1. cq 설치
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash

# 2. 워커 서비스 설치 (root 권한 자동 escalation)
cq hub worker install
# → Docker 미설치 시 자동 설치
# → NVIDIA Container Toolkit 자동 감지/설치
# → 현재 사용자를 docker 그룹에 추가
# → systemd 서비스 등록 + 즉시 시작
```

설치 후 `systemctl status cq-worker`로 상태를 확인합니다.

```bash
# 미리보기만 (서비스 파일 출력, 설치하지 않음)
cq hub worker install --dry-run

# 사용자 레벨 systemd unit (Linux only)
cq hub worker install --user
```

## 인증 방법

### JWT 인증 (권장)

API key 없이 브라우저 로그인으로 인증합니다.

```bash
cq auth login --device
cq hub worker start   # JWT 자동 감지, 만료 시 자동 refresh
```

### Scoped API Key 인증

키 접두사로 접근 범위를 제한합니다:

| 접두사 | 범위 | 용도 |
|--------|------|------|
| `sk-user-` | 잡 제출/조회 | Claude Code에서 Hub 연결 시 |
| `sk-worker-` | 잡 폴링/완료 보고 | 워커 인증 시 |
| (없음) | 전체 접근 | 하위 호환, 관리자용 |

```bash
# 워커용 키 설정
export C5_API_KEY="sk-worker-<your-key>"
cq hub worker start

# 또는 init으로 영구 저장
cq hub worker init   # 대화형: Hub URL + API key 입력 → ~/.c5/config.yaml
cq hub worker start
```

## Docker Compose로 실행 (대안)

`cq hub worker install` 대신 Docker Compose로 직접 실행할 수 있습니다.

```bash
# .env 파일 설정
cat > .env <<EOF
C5_HUB_URL=https://<hub-host>:8585
C5_API_KEY=sk-worker-<your-key>
EOF

# GPU 워커 실행
docker compose up -d
docker compose logs -f
```

`docker-compose.yml`과 `Dockerfile`은 이 디렉토리에 포함되어 있습니다.

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

## systemd 서비스 수동 설정

`cq hub worker install`이 자동 생성하지만, 수동 설정이 필요한 경우:

```ini
# /etc/systemd/system/cq-worker.service
[Unit]
Description=CQ Hub Worker
After=network.target docker.service
Wants=docker.service

[Service]
User=ubuntu
SupplementaryGroups=docker
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

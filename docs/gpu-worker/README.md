# GPU Worker — C5 Hub 연결 가이드

고객 GPU 서버에서 `curl` 설치 후 3개 스크립트 실행으로 C5 Hub에 연결합니다.
`cq` 바이너리 불필요 — `bash` + `curl` + `python3`만으로 동작합니다.

## 필수 조건

| 항목 | 필수 여부 | 비고 |
|------|----------|------|
| `bash` | 필수 | |
| `curl` | 필수 | Hub API 통신 |
| `python3` | 필수 | JSON 파싱 |
| `c5` 바이너리 | 필수 | [설치 방법](#c5-설치) |
| `nvidia-smi` | 선택 | GPU 없는 환경에서는 fallback JSON 출력 |

## c5 설치

```bash
# 최신 릴리즈 다운로드 (Linux amd64 예시)
curl -L https://github.com/PlayIdea-Lab/cq/releases/latest/download/c5-linux-amd64 -o ~/.local/bin/c5
chmod +x ~/.local/bin/c5

# 또는 cq 설치 후 내장 c5 사용
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
```

## Hub API Key 인증

C5 Hub는 API Key 인증을 사용합니다. 워커 연결 시 반드시 설정해야 합니다.

```bash
# 환경변수 설정 (권장)
export C5_API_KEY="your-api-key-here"

# 또는 --api-key 플래그 직접 전달
c5 worker --api-key "$C5_API_KEY" --capabilities gpu-caps.yaml --server "$C5_HUB_URL"
```

> **Note**: `c5` 바이너리는 `C5_API_KEY` 환경변수를 사용합니다.

## 빠른 시작

```bash
# 1. 파일 다운로드
git clone https://github.com/PlayIdea-Lab/cq
cd cq/docs/gpu-worker

# 또는 개별 다운로드
curl -O https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker/gpu-caps.yaml
mkdir -p scripts
curl -O --output-dir scripts https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker/scripts/gpu-status.sh
curl -O --output-dir scripts https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker/scripts/gpu-train.sh
curl -O --output-dir scripts https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/docs/gpu-worker/scripts/gpu-infer.sh
chmod +x scripts/*.sh

# 2. 환경변수 설정
export C5_HUB_URL="https://your-hub.example.com"
export C5_API_KEY="your-api-key"

# 3. GPU 상태 확인
bash scripts/gpu-status.sh

# 4. 워커 시작
c5 worker --capabilities gpu-caps.yaml --server "$C5_HUB_URL"
```

## gpu-caps.yaml 설정

기본 제공 `gpu-caps.yaml`을 환경에 맞게 수정하세요:

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

```ini
# /etc/systemd/system/c5-gpu-worker.service
[Unit]
Description=C5 GPU Worker
After=network.target

[Service]
User=ubuntu
Environment=C5_HUB_URL=https://your-hub.example.com
Environment=C5_API_KEY=your-api-key
WorkingDirectory=/opt/gpu-worker
ExecStart=/usr/local/bin/c5 worker \
    --capabilities gpu-caps.yaml \
    --server ${C5_HUB_URL}
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now c5-gpu-worker
sudo journalctl -fu c5-gpu-worker
```

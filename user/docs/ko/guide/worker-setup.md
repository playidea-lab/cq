# GPU Worker 설정

GPU 서버, 클라우드 VM, 또는 로컬 워크스테이션을 CQ Hub에 GPU 작업 Worker로 연결하세요. 설정 없이, 어떤 OS에서도, 종단간 암호화 relay — GPU Anywhere의 토대입니다.

## 아키텍처

```
노트북                  CQ Hub (클라우드)         Worker (GPU/CPU)
───────────              ──────────────          ────────────────
cq hub submit  ────────► 작업 큐         ◄────   cq serve
(코드 스냅샷 +           (분배)                   (큐 폴링,
 작업 명세)                                        작업 실행,
                                                  결과 업로드)
```

Worker는 **무상태**입니다 — Worker 머신에 프로젝트 설정이 필요 없습니다. 작업에 모든 것이 포함됩니다: 코드 스냅샷, 환경 변수, 아티팩트 선언.

## 3단계 빠른 시작

### 1단계: Worker 머신에 CQ 설치

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

Linux (x86_64, ARM64), macOS, Windows/WSL2에서 작동합니다. Docker와 NVIDIA Container Toolkit이 감지되면 자동으로 설정됩니다.

### 2단계: 인증

```sh
cq auth login    # GitHub OAuth — 노트북과 같은 계정 사용
```

브라우저가 없는 헤드리스 머신의 경우:

```sh
cq auth login --device    # 디바이스 코드 플로우 — 다른 디바이스에서 코드 입력
```

### 3단계: 시작

```sh
cq serve    # Hub Worker + MCP + relay + cron을 하나의 프로세스로 시작
```

이제 Worker가 연결되었습니다. 노트북에서 제출한 작업이 자동으로 도착합니다.

## `cq serve`가 시작하는 것

`cq serve`는 올인원 진입점입니다. 개별 컴포넌트를 따로 실행하는 것을 대체합니다.

| 컴포넌트 | 포함 여부 |
|---------|---------|
| Hub Worker (작업 폴링) | 있음 |
| MCP 서버 | 있음 |
| Relay (NAT 통과) | 있음 |
| Cron 스케줄러 | 있음 |
| pg_notify 실시간 | 있음 (`cloud.direct_url` 설정 시) |

## 서비스로 실행

### Linux (systemd) — 권장

```sh
cq serve install    # systemd 서비스 설치, GPU 발견 시 Docker, NVIDIA 툴킷도 설치
systemctl status cq-worker
```

로그 확인:

```sh
journalctl -fu cq-worker
```

수동 systemd 유닛 (직접 작성 선호 시):

```ini
[Unit]
Description=CQ Hub Worker
After=network.target docker.service

[Service]
User=ubuntu
SupplementaryGroups=docker
WorkingDirectory=/opt/gpu-worker
ExecStart=/usr/local/bin/cq serve
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### macOS (launchd)

```sh
cq serve install    # launchd plist 생성 및 로드
```

### Docker Compose

```sh
curl -sSL https://github.com/PlayIdea-Lab/cq/releases/latest/download/gpu-worker.tar.gz | tar xz

cat > .env <<EOF
C5_HUB_URL=https://<hub-host>:8585
C5_API_KEY=sk-worker-<your-key>
EOF

docker compose up -d
docker compose logs -f
```

## 실시간 작업 전달

기본적으로 Worker는 30초마다 작업을 폴링합니다. 1초 미만의 전달을 위해 직접 데이터베이스 연결을 설정하세요:

```yaml
# ~/.c4/config.yaml
cloud:
  direct_url: "postgresql://..."    # 직접 Supabase 연결 문자열
```

`direct_url` 설정 시, Worker는 PostgreSQL `LISTEN 'new_job'`을 사용합니다 — 작업이 즉시 도착합니다.

## 작업 제출

노트북에서, Claude Code 안에서:

```sh
# MCP 도구
c4_hub_submit(command="python train.py")
```

또는 터미널에서:

```sh
cq hub submit --run "python train.py"
```

CQ가 현재 디렉토리를 Drive에 스냅샷(내용 주소 지정, 자동 중복 제거)하고 Hub에 작업을 게시합니다. Git이 필요 없습니다.

## GPU 감지

Worker가 GPU 능력을 자동으로 감지합니다:

- `nvidia-smi`가 발견되면 Worker가 GPU 지원으로 등록
- `requires_gpu: true` 작업은 GPU Worker에만 라우팅
- `nvidia-smi`가 없으면 Worker가 CPU 전용 모드로 시작 (별도 조치 불필요)

## 특정 Worker로 라우팅

### Worker ID로

```sh
cq hub submit --target worker-abc123 python train.py
```

### 능력으로

```sh
cq hub submit --capability cuda python train.py
```

### 태그로

```sh
cq hub submit --tags gpu,a100 python train.py
```

Worker의 `caps.yaml`에 태그 선언:

```yaml
tags:
  - gpu
  - a100
  - datacenter-us
```

## 모니터링

```sh
cq hub workers              # 활성 Worker
cq hub workers --all        # 오프라인 Worker 포함
cq hub list                 # 최근 작업
cq hub status <job_id>      # 작업 상태
cq hub watch <job_id>       # 작업 출력 실시간 보기
cq hub log <job_id>         # 작업 로그
cq hub summary              # Hub 통계
```

## 유지보수

### 좀비 Worker 제거

24시간 이상 오프라인인 Worker는 자동으로 정리됩니다. 수동 정리:

```sh
cq hub workers prune              # 오프라인 Worker 제거
cq hub workers prune --dry-run    # 미리보기
```

### 버전 게이트

Hub가 최소 Worker 버전을 요구하는 경우:

```sh
cq update               # 바이너리 업데이트
cq hub worker start     # Worker 재시작
```

## 인증 레퍼런스

| 방법 | 방법 |
|------|------|
| JWT (권장) | `cq auth login` — `~/.c4/session.json`에서 자동 주입 |
| API 키 | `export C5_API_KEY=sk-worker-<key>` |
| 디바이스 코드 | `cq auth login --device` — 헤드리스 머신용 |

키 접두사:

| 접두사 | 범위 |
|-------|------|
| `sk-worker-*` | 작업 폴링 및 완료만 |
| `sk-user-*` | 작업 제출 및 조회만 |
| (없음) | 전체 접근 |

## 문제 해결

| 증상 | 해결 방법 |
|------|---------|
| `nvidia-smi not found` | Worker가 자동으로 CPU 전용 모드로 실행 — 별도 조치 불필요 |
| 인증 오류 | `cq auth login` 또는 `cq auth login --device` 재실행 |
| Worker가 오프라인으로 표시 | `ps aux | grep cq` 및 `curl -s "$C5_HUB_URL/v1/health"` 실행 |
| 작업 멈춤 | `cq hub log <job_id>` 및 Worker 로그 확인 |
| CI에서 `--non-interactive` 필요 | `cq hub worker init`에 `--non-interactive` 플래그 전달 |
| WSL2 relay 끊김 | CQ가 `SO_KEEPALIVE`를 자동으로 설정 — 설정 불필요. keepalive 인식 relay 컴포넌트가 포함된 `cq serve` 사용 (`cq hub worker start` 아님) |

## 다음 단계

- [Knowledge Loop](growth-loop.md) — 실험 결과를 재사용 가능한 AI 지식으로 축적
- [티어](tiers.md) — Free/Pro/Team 기능 이해

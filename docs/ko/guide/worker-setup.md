# 원격 워커 설정

GPU 서버(또는 어떤 머신이든)를 CQ Hub의 stateless 잡 워커로 연결합니다.

::: info full 티어 필요
워커 모드는 `full` 티어 바이너리가 필요합니다. [`--tier full`로 설치](/ko/guide/install#특정-티어-설치).
:::

## 권장 시작 방법

3개 명령으로 GPU 서버를 연결합니다:

```sh
# GPU 서버에서:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
cq auth login
cq hub worker start
```

이게 전부입니다. 이제 서버가 `cq hub submit`으로 전송된 잡을 받을 준비가 됐습니다.

---

## 동작 원리

```
내 노트북              Supabase (클라우드)      GPU 서버
────────────          ────────────────        ──────────
cq hub submit  ──►   jobs 테이블        ◄──   cq hub worker start
(코드 스냅샷 +        LISTEN/NOTIFY            (pgx로 잡 pull,
 잡 등록)            (NAT-safe)               실행,
                                              결과 업로드)
```

1. 노트북에서 `cq hub submit` 실행 — CQ가 현재 폴더를 Drive CAS에 스냅샷하고 Supabase에 잡 행을 삽입합니다.
2. 워커는 `pgx LISTEN/NOTIFY`로 리슨합니다 (Supabase 포트 5432로 아웃바운드 TCP — NAT-safe, 인바운드 포트 불필요).
3. 잡을 가져간 워커가 정확한 스냅샷을 다운로드해 실행하고, 결과를 Drive에 업로드합니다.
4. 워커는 **stateless** — 서버에 프로젝트 설정이 없어도 됩니다. 잡 payload에 모든 정보가 포함됩니다.

---

## 1단계 — 서버에 CQ 설치

GPU 서버에 SSH 접속 후 실행:

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
```

새 셸을 열거나 RC 파일을 소스하여 PATH 활성화:

```sh
source ~/.bashrc   # 또는 ~/.zshrc
cq --version
```

## 2단계 — 로그인

```sh
cq auth login
```

디바이스 코드가 표시됩니다. 노트북 등 아무 브라우저에서 URL을 열고 코드를 입력하면 됩니다. 서버에 확인 메시지가 출력됩니다:

```
✓ Logged in as user@example.com
```

## 3단계 — 워커 시작

```sh
cq hub worker start
```

워커가 Supabase에 연결되고 잡을 기다립니다:

```
cq: registered worker  id=worker-abc123  host=gpu-server-1
cq: listening for jobs via NOTIFY...
```

이게 전부입니다. 서버는 이제 stateless 워커입니다 — 프로젝트 설정, `cq project use`, 로컬 데이터 없이 동작합니다.

Hub URL 설정이 필요 없습니다. `C5_HUB_URL`이나 `C5_API_KEY` 환경변수도 없습니다. 워커는 `~/.c4/config.yaml`의 기존 `cloud.url`을 사용합니다.

---

## 영구 서비스로 실행

`cq`가 OS 서비스를 자동으로 설치합니다 (macOS LaunchAgent / Linux systemd):

```sh
cq              # 자동 설치 및 시작
cq serve status # 서비스 상태 확인
cq stop         # 서비스 중지
```

설치 후 워커는 **모든 장애 상황에서 자동 복구**됩니다:

| 장애 | 복구 |
|------|------|
| 프로세스 크래시 | systemd `Restart=always` (5초 후) / macOS `KeepAlive` |
| 토큰 만료 | `TokenProvider` 만료 5분 전 자동 갱신 |
| 네트워크 끊김 | Relay `reconnectLoop` (지수 백오프) |
| OS 재부팅 | systemd `enable` / macOS `RunAtLoad` |
| `.mcp.json` 만료 | 10분 주기로 `session.json`에서 자동 동기화 |

로그:
- **macOS**: `~/Library/Logs/cq-serve.{out,err}.log`
- **Linux**: `~/.local/state/cq/cq-serve.{out,err}.log`

### WSL2 지원 *(v1.32.1+)*

CQ가 WSL2를 자동 감지하여 추가 보강:

- **Windows Task Scheduler** — 윈도우 부팅 시 WSL + cq serve 자동 시작
- **nvidia-smi fallback** — `/usr/lib/wsl/lib/nvidia-smi` 자동 탐색 (GPU 패스스루)
- **systemd 확인** — `/etc/wsl.conf`에 `[boot] systemd=true` 없으면 경고

```sh
cq serve install   # WSL2에서: systemd + Windows Task Scheduler 이중 등록
cq serve uninstall # 양쪽 모두 제거
```

::: details 레거시 수동 systemd 설정
별도 유닛 파일이 필요한 경우:

```sh
cat > ~/.config/systemd/user/cq-worker.service << 'EOF'
[Unit]
Description=CQ Hub Worker
After=network-online.target

[Service]
ExecStart=%h/.local/bin/cq hub worker start
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now cq-worker
systemctl --user status cq-worker
```

로그 확인:

```sh
journalctl --user -u cq-worker -f
```

---

## 버전 게이트 (자동 업그레이드)

Hub 운영자가 최소 버전 요구사항을 설정하면, 해당 버전 미만의 워커는 잡 대신 `upgrade` 제어 메시지를 받습니다. 워커가 자동으로 `cq upgrade`를 실행하고 재시작합니다 — 수동 개입 불필요.

버전 정보 없이 빌드된 워커(`version="unknown"`)는 업그레이드 루프 방지를 위해 게이트를 bypass합니다.

---

## 잡이 도착하면 일어나는 일

1. **스냅샷 pull** — 코드 스냅샷 다운로드 (Drive CAS, 정확한 버전 해시)
2. **`cq.yaml` 파싱** — `run`, `artifacts.input`, `artifacts.output` 읽기
3. **입력 아티팩트** — Drive에서 선언된 데이터셋/파일 pull
4. **실행** — `C4_PROJECT_ID`를 주입하여 명령 실행
5. **출력 push** — 선언된 출력 경로를 Drive에 업로드

워커는 프로젝트 이름이나 credentials를 미리 알 필요가 없습니다 — 모든 정보가 잡 payload에 포함됩니다.

---

## 유지 관리

### 좀비 워커 GC *(v0.91.0+)*

24시간 이상 오프라인인 워커는 Hub가 자동으로 정리합니다. 수동 정리:

```sh
cq hub workers prune              # 좀비 워커 제거
cq hub workers prune --dry-run    # 미리보기만
cq hub workers                    # 활성 워커 (기본값)
cq hub workers --all              # 오프라인/정리된 워커 포함
```

### 캐퍼빌리티 폴백 체인 *(v0.91.0+)*

잡 실행 시 워커는 3단계 폴백으로 명령을 찾습니다:
1. 디스크의 `capabilities/<name>` 파일
2. `caps.yaml`의 `command` 필드
3. 잡 payload의 `C5_PARAMS.command`

`caps.yaml`에 `command:`가 정의되어 있으면 캐퍼빌리티 파일이 불필요합니다.

## 워커 어피니티 *(v1.5.0+)*

워커는 잘 수행하는 프로젝트를 자동으로 학습합니다. 특정 프로젝트의 잡을 성공할수록 어피니티 점수가 높아지고, 해당 프로젝트의 잡은 우선적으로 그 워커에 라우팅됩니다.

### 동작 방식

```
1회:  HMR 잡 → 아무 유휴 워커 (기록 없음)
2회:  HMR 잡 → 같은 워커 선호 (affinity: hmr 1✓)
10회: HMR 잡 → 강하게 선호 (affinity: hmr 10✓)
```

점수 계산식: `project_match×10 + tag_overlap×3 + recency×2 + success_rate×5`

### 어피니티 확인

```sh
cq hub workers
  ID               STATUS   AFFINITY              TAGS
  gpu-server       idle     hmr(10✓) cq(2✓)       [gpu, a100]
  build-server     idle     cq(15✓)               [cpu, linux]
  nas              idle     (none)                [storage]
```

### 태그 기반 라우팅

올바른 하드웨어로 잡을 보내려면 태그를 사용하세요:

```sh
# GPU 필요 잡 → 'gpu' 태그가 있는 워커만
cq hub submit --project hmr --tags gpu "train backbone"

# CPU 잡 → 아무 워커
cq hub submit --project cq "go build test"
```

태그가 먼저 후보를 필터링하고, 어피니티가 순위를 매깁니다.

### 수동 지정

특정 워커에 잡을 고정합니다:

```sh
cq hub submit --worker gpu-server "urgent training"
```

어피니티 점수를 완전히 무시합니다.

---

## 노트북에서 잡 제출

`cq hub submit`과 `cq.yaml`을 사용한 전체 제출 워크플로우는 [분산 실험 예시](/ko/examples/distributed-experiments)를 참고하세요.

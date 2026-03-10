# 원격 워커 설정

GPU 서버(또는 어떤 머신이든)를 CQ Hub의 stateless 잡 워커로 연결합니다.

::: info full 티어 필요
워커 모드는 `full` 티어 바이너리가 필요합니다. [`--tier full`로 설치](/ko/guide/install#특정-티어-설치).
:::

## 권장 시작 방법

4개 명령으로 GPU 서버를 연결합니다:

```sh
# GPU 서버에서:
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier full
cq auth login
cq config set hub.url https://your-hub.fly.dev
cq hub worker start
```

이게 전부입니다. 이제 서버가 `cq hub submit`으로 전송된 잡을 받을 준비가 됐습니다.

---

## 동작 원리

```
내 노트북              C5 Hub (클라우드)      GPU 서버
────────────          ─────────────         ──────────
cq hub submit  ──►   잡 큐          ◄──    c5 worker
(코드 스냅샷 +        (분배)                 (잡 pull,
 잡 등록)                                    실행,
                                            결과 업로드)
```

1. 노트북에서 `cq hub submit` 실행 — CQ가 현재 폴더를 Drive CAS에 스냅샷하고 잡을 등록합니다.
2. 연결된 워커가 잡을 pull하고, 정확한 스냅샷을 다운로드해 실행하고, 결과를 Drive에 올립니다.
3. 워커는 **stateless** — 서버에 프로젝트 설정이 없어도 됩니다. 잡 payload에 모든 정보가 포함됩니다.

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

## 3단계 — Hub URL 설정

Hub 엔드포인트를 한 번 설정합니다:

```sh
cq config set hub.url https://your-hub.fly.dev
cq config set hub.api_key YOUR_API_KEY   # Hub에 인증이 필요한 경우
```

또는 환경변수로 설정 (systemd / Docker에 유용):

```sh
export C5_HUB_URL=https://your-hub.fly.dev
export C5_API_KEY=YOUR_API_KEY
```

## 4단계 — 워커 시작

```sh
c5 worker
```

워커가 Hub에 등록되고 잡을 기다립니다:

```
c5: registered worker  id=worker-abc123  host=gpu-server-1
c5: waiting for jobs...
```

이게 전부입니다. 서버는 이제 stateless 워커입니다 — 프로젝트 설정, `cq project use`, 로컬 데이터 없이 동작합니다.

---

## 영구 서비스로 실행 (systemd)

SSH 종료 후에도 워커를 유지하려면:

```sh
cat > ~/.config/systemd/user/c5-worker.service << 'EOF'
[Unit]
Description=CQ C5 Worker
After=network-online.target

[Service]
ExecStart=%h/.local/bin/c5 worker
Environment=C5_HUB_URL=https://your-hub.fly.dev
Environment=C5_API_KEY=YOUR_API_KEY
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now c5-worker
systemctl --user status c5-worker
```

로그 확인:

```sh
journalctl --user -u c5-worker -f
```

---

## 버전 게이트 (자동 업그레이드)

Hub 운영자가 `C5_MIN_VERSION`을 설정하면, 해당 버전 미만의 워커는 잡 대신 `upgrade` 제어 메시지를 받습니다. 워커가 자동으로 `cq upgrade`를 실행하고 재시작합니다 — 수동 개입 불필요.

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

## 노트북에서 잡 제출

`cq hub submit`과 `cq.yaml`을 사용한 전체 제출 워크플로우는 [분산 실험 예시](/ko/examples/distributed-experiments)를 참고하세요.

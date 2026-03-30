# Hub Worker 가이드

GPU 서버, 클라우드 VM, 로컬 워크스테이션 — 어떤 머신이든 Hub Worker로 연결합니다.

## 개요

```
노트북               Hub (클라우드)          Worker (GPU/CPU)
────────────         ─────────────         ────────────────
cq hub submit   ──►  작업 큐          ◄──  cq serve
(코드 업로드 +       (분배)                 (작업 받아서
 작업 등록)                                 실행, 결과 업로드)
```

Worker는 **무상태** — 서버에 프로젝트 설정 불필요. 작업 페이로드가 모든 것을 담고 있습니다.

---

## 빠른 시작

```sh
# 1. 설치
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash

# 2. 인증
cq auth login

# 3. 시작 (Hub Worker + MCP + Relay + Cron 전부 포함)
cq serve
```

`cq serve`가 모든 것을 하나의 프로세스로 시작합니다.

---

## 작업 라우팅

### 특정 Worker 지정

```sh
cq hub submit --target worker-id python train.py
```

### 기능으로 라우팅

```sh
cq hub submit --capability cuda python train.py
```

### 태그로 라우팅

```sh
cq hub submit --tags gpu,a100 python train.py
```

---

## 작업 제출

```sh
# CLI — 현재 폴더를 작업으로 제출
cq hub submit --run "python train.py"

# MCP (Claude Code에서)
c4_hub_submit(command="python train.py")
```

CQ가 현재 폴더를 Drive CAS에 스냅샷하고 Hub에 작업을 등록합니다.

---

## 모니터링

```sh
cq hub workers              # 활성 Worker 목록
cq hub status <job_id>      # 작업 상태
cq hub list                 # 작업 목록
cq hub watch <job_id>       # 작업 진행 실시간 확인
cq hub log <job_id>         # 작업 로그
```

---

## 서비스 등록

### systemd (Linux)

```sh
cq hub worker install       # Docker + systemd 자동 설정
systemctl status cq-worker  # 확인
```

### macOS

```sh
cq hub worker install       # launchd plist 자동 생성
```

---

## 문제 해결

| 증상 | 해결 |
|------|------|
| nvidia-smi 없음 | CPU 모드로 자동 전환 — 조치 불필요 |
| 인증 에러 | `cq auth login` 재실행 |
| Worker 오프라인 | `cq serve` 실행 중인지 확인, Hub 접근 가능한지 확인 |
| 작업 타임아웃 | `cq hub log <job_id>`로 로그 확인 |

<div align="center">

[English](README.md)

# CQ — GPU Anywhere, Anytime, Anything.

**당신의 GPU는 70%의 시간 동안 유휴 상태입니다. CQ가 GPU를 AI에 연결합니다 — 설정 없이, 어떤 OS에서도, 암호화된 상태로.**

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)
![Version](https://img.shields.io/badge/version-v1.47-blue)
![MCP Tools](https://img.shields.io/badge/MCP_Tools-217-blueviolet)
![License](https://img.shields.io/badge/License-Personal_Study-orange)

![Demo](docs/demo.gif)

</div>

---

## 왜 CQ인가?

| | CQ 없이 | CQ 사용 시 |
|---|---|---|
| 새벽 2시의 GPU | 유휴 상태, 전기 낭비 | 실험 실행 중 |
| 기기 간 AI | 매 세션마다 컨텍스트 소실 | 어디서나 영속적 메모리 |
| 코드 품질 | 운에 맡기기 | 6축 자동 리뷰 |

---

## 활용 사례

### 🖥️ 어디서나 집의 GPU로 AI 실험 실행

노트북, 휴대폰, 또는 어떤 AI 어시스턴트에서든 학습 작업을 시작하세요. CQ의 Hub가 사용 가능한 GPU에 태스크를 분배하고, 실시간으로 메트릭을 스트리밍하며, 아티팩트를 자동으로 저장합니다.

```sh
cq job submit --image pytorch --gpu 1 -- python train.py
```

### 🧠 AI 플랫폼을 넘나드는 외장 두뇌

모든 대화가 지식 베이스에 누적됩니다. ChatGPT가 버그 근본 원인을 찾으면 — Claude가 다음 세션에서 이어받습니다. 결정, 패턴, 발견이 도구와 세션과 기기를 넘어 영속됩니다.

```sh
cq serve   # MCP 브릿지 시작. 끝.
```

### 🔒 암호화된 P2P — 포트 포워딩도, VPN도 없이

CQ는 NAT traversal을 위해 relay 서버를 사용합니다. 기기 간 트래픽은 엔드-투-엔드 암호화됩니다. 기업 방화벽, WSL2, 동적 IP 환경에서도 설정 없이 동작합니다.

---

## 빠른 시작

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq               # 로그인 + 서비스 시작 (최초 1회)
cq claude        # 개발 시작
```

언제든 업데이트: `cq update`

---

## 연동 지원

| AI 도구 | 연동 방식 |
|---|---|
| **Claude Code** | Native MCP — 전체 도구 접근 |
| **ChatGPT** | OAuth 2.1 remote MCP proxy |
| **Cursor** | `mcp.pilab.kr`를 통한 remote MCP |
| **Gemini CLI** | MCP 호환 연결 |

---

## 가격

| | Free | Pro | Team |
|---|---|---|---|
| **가격** | $0 | $5–10/월 | 문의 |
| **모드** | solo | connected | full |
| **Knowledge (AI 자동 수집)** | 로컬 SQLite | 클라우드 (pgvector) | 클라우드 + 공유 |
| **Hub GPU 작업** | — | 월 100회 | 무제한 |
| **Relay (P2P)** | — | 포함 | 포함 |
| **Research Loop** | — | 포함 | 포함 |
| **팀 지식 베이스** | — | — | 포함 |

---

## 핵심 구성요소

| 구성요소 | 설명 |
|-----------|-------------|
| **Go MCP Server** | 217개 도구 (코어 + Hub + 조건부), Registry 기반 |
| **Knowledge** | FTS5 + pgvector (OpenAI 1536d) + 3-way RRF + 자동 증류 |
| **Hub** | 분산 작업 큐, DAG 엔진, 아티팩트 저장소, cron, watchdog |
| **Session** | LLM을 통한 자동 요약, 시작 시 컨텍스트 주입 |
| **Research Loop** | 자율 ML 실험 사이클 (계획→학습→평가→반복) |
| **Paper Mode** | knowledge DB 연동 구조화 논문/문서 학습 |
| **70가지 Skills** | Claude Code 슬래시 커맨드 (/plan, /run, /finish, /pi, /paper 등) |

---

<details>
<summary>아키텍처</summary>

```
┌──────────────────┐          ┌────────────────────────────┐
│ Local (Thin Agent)│  JWT    │ Cloud (Supabase)            │
│                   │◄───────►│                             │
│ 손발:             │         │ 두뇌:                       │
│  ├ Files / Git    │         │  ├ Tasks (Postgres)         │
│  ├ Build / Test   │         │  ├ Knowledge (pgvector)     │
│  ├ LSP analysis   │         │  ├ LLM Proxy (Edge Fn)     │
│  └ MCP bridge     │         │  ├ Quality Gates            │
│                   │         │  └ Hub (분산 작업)          │
│ Service (cq serve)│   WSS   │                             │
│  ├ Relay ─────────┼────────►│  Relay (Fly.io)             │
│  ├ EventBus       │         │  └ NAT traversal            │
│  └ Token refresh  │         │                             │
└──────────────────┘          │ External Brain (CF Worker)  │
                              │  ├ OAuth 2.1 MCP proxy      │
Any AI (ChatGPT,   ── MCP ──►│  ├ Knowledge record/search  │
 Claude, Gemini)              │  └ Session summary          │
                              └────────────────────────────┘

solo:       모든 것이 로컬 (SQLite + 개인 API 키)
connected:  클라우드 두뇌 + relay (로그인 + serve)
full:       Connected + GPU 워커 + research loop
```

</details>

---

## 개발

```bash
cd c4-core && make install                             # 빌드 + 설치
cd c4-core && go build ./... && go test -p 1 ./...    # Go 테스트
uv run pytest tests/                                   # Python 테스트
cq doctor                                              # 헬스 체크
```

[문서](https://cq.pilab.kr) | [설치](https://cq.pilab.kr/guide/install) | [빠른 시작](https://cq.pilab.kr/guide/quickstart) | [아키텍처](https://cq.pilab.kr/reference/architecture)

---

## 라이선스

Personal Study & Research License (비상업적). [LICENSE.md](./LICENSE.md) 참조. Copyright (c) 2026 PlayIdeaLab.

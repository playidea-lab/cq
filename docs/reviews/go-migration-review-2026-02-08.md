# C4 Go 전환 검토 보고서

> **작성일**: 2026-02-08
> **작성자**: Architecture Review
> **상태**: Draft - 의사결정 대기
> **범위**: C4 Core Backend를 Go Hybrid 아키텍처로 전환하는 타당성 검토

---

## 1. Executive Summary

C4(110K LOC Python)의 코어 백엔드를 Go로 전환하는 Hybrid 아키텍처를 검토했다.
Go, Rust, 현행 Python 유지 세 가지 옵션을 비교한 결과, **Go Hybrid가 현 단계에서 최적**이라는 결론이다.

| 항목 | 현행 Python | Go Hybrid | Rust Hybrid |
|------|------------|-----------|-------------|
| 가중 점수 | 5.85/10 | **7.00/10** | 5.60/10 |
| 개발 기간 | 0 (현행) | 3-4개월 | 6-8개월 |
| 배포 복잡도 | 높음 (venv) | 중간 (바이너리+Python sidecar) | 중간 |
| MCP SDK 안정성 | v1.0 (Python) | **v1.0 (Go)** | v0.14 (pre-1.0) |

**권장**: Go Hybrid 전환을 Python v1.0 안정화와 **병렬 진행**

---

## 2. 현재 시스템 완성도

### 2.1 코드베이스 현황

| 항목 | 수치 |
|------|------|
| Production 코드 | 324 파일, 110,579 LOC |
| 테스트 코드 | 274 파일, 96,768 LOC |
| 테스트:코드 비율 | 0.87:1 |
| 서브패키지 | 44개 |
| MCP 도구 핸들러 | 24개 |
| 지원 IDE | 4개 (Claude Code, Cursor, Gemini, VSCode) |

### 2.2 주요 모듈 규모

| 모듈 | 파일 | LOC | 역할 |
|------|------|-----|------|
| supervisor | 33 | 12,059 | AI 리뷰, 에이전트 라우팅, 체크포인트 |
| daemon | 19 | 11,918 | 태스크 오케스트레이션, Worker 관리 |
| api | 29 | 10,772 | REST API (FastAPI) |
| services | 21 | 7,652 | 비즈니스 로직 |
| mcp | 24 | 6,946 | MCP 서버, 도구 핸들러 |
| lsp | 13 | 6,392 | Language Server Protocol |
| memory | 12 | 5,820 | 벡터 스토어, 임베딩 |
| store | 8 | 3,644 | SQLite, Supabase |
| config | 5 | 1,045 | 설정 관리 |
| validation | 1 | 387 | 검증 러너 |

### 2.3 완성도 평가: 75-80%

**완성된 부분:**
- State Machine (9 상태, 20+ 전이 규칙)
- Task Lifecycle (Review-as-Task, 자동 리뷰 생성)
- Worker/Direct 두 가지 실행 모드
- MCP 서버 (24개 도구)
- LSP 통합 (심볼 검색, 코드 분석)
- Economic Mode (모델별 비용 최적화)
- Knowledge Store v2 (Hybrid Search)
- Git 통합 (hooks, worktree, 브랜치 전략)

**미완성:**
- Cloud 백엔드 (Supabase 통합 부분적)
- Billing, Auth 모듈 (스텁)
- Realtime 모듈 (WebSocket)
- 일부 모니터링/텔레메트리

---

## 3. 현행 시스템의 문제점

### 3.1 배포 마찰

```
현재 설치 과정:
1. curl 스크립트 다운로드
2. uv 자동 설치 (없는 경우)
3. Python 3.11+ 확인
4. uv sync (의존성 30+개)
5. 가상환경 활성화
6. 글로벌 명령어 등록
```

- uv.lock 795KB, 의존성 해결에 수십 초
- Python 버전 충돌 시 설치 실패
- 사용자별 환경 차이로 디버깅 어려움

### 3.2 성능 한계

| 항목 | 현재 | 문제 |
|------|------|------|
| MCP 서버 시작 | 2-3초 | import chain (30+개 패키지) |
| Worker 생성 | ~200ms/개 | subprocess 스폰 비용 |
| 메모리 | ~50-100MB | Python 인터프리터 오버헤드 |
| 동시 Worker | GIL 제한 | asyncio 단일 스레드 |

### 3.3 동시성 안정성

- Zombie Worker 버그 (2026-02-06, 6개 근본 원인 수정)
- asyncio lock 관리의 복잡성
- Worker 상태와 Task 상태 간 비동기화 발생 이력
- 인메모리 캐시와 DB 간 정합성 문제

---

## 4. 언어 비교 분석

### 4.1 Go vs Rust vs Python (Hybrid 시나리오)

#### MCP SDK

| 항목 | Python | Go | Rust |
|------|--------|-----|------|
| 공식 SDK | mcp (v1.0) | go-sdk (**v1.0**) | rmcp (v0.14) |
| 안정성 보장 | 있음 | **있음** | 없음 |
| Stars | - | ~3.5K | ~3.0K |
| Transport | stdio, SSE | stdio, SSE, WS, gRPC | stdio, SSE, child |

#### 동시성

| 항목 | Python | Go | Rust |
|------|--------|-----|------|
| 모델 | asyncio (cooperative) | goroutine (**preemptive**) | tokio (cooperative) |
| 복잡도 | 중간 | **낮음** | 높음 |
| 병렬성 | GIL 제한 | **진정한 병렬** | 진정한 병렬 |
| Worker 생성 | ~200ms (subprocess) | **~1us** (goroutine) | ~1us (tokio spawn) |
| 학습 비용 | 0 (현행) | **2-4주** | 8-12주 |

#### 빌드 & 배포

| 항목 | Python | Go | Rust |
|------|--------|-----|------|
| 빌드 시간 | 0 (인터프리터) | **1-3초** | 30-60초 |
| 증분 빌드 | 0 | **<1초** | 5-10초 |
| 바이너리 | venv ~50MB | **단일 ~15MB** | 단일 ~5-8MB |
| 크로스컴파일 | 어려움 | **GOOS=... 한 줄** | 가능 (복잡) |

#### SQLite

| 항목 | Python | Go | Rust |
|------|--------|-----|------|
| 라이브러리 | sqlite3 (내장) | modernc (**pure Go**) | rusqlite (C 바인딩) |
| Async | asyncio wrapper | goroutine blocking OK | sqlx (복잡) |
| 벡터 검색 | sqlite-vec | sqlite-vec (CGo) | sqlite-vec (C) |

#### Python 연동

| 항목 | Go | Rust |
|------|-----|------|
| gRPC | 동등 | 동등 |
| In-process FFI | CGo (비권장) | **PyO3** (우수) |
| subprocess | 동등 | 동등 |

#### 원시 성능

| 항목 | Go | Rust |
|------|-----|------|
| JSON 파싱 | 1x | ~2x |
| HTTP 처리 | 1x | ~1.5x |
| 메모리 효율 | ~2KB/goroutine | ~수백 bytes |

> **주의**: C4의 실제 병목은 LLM API 호출(2-30초)이므로 언어간 raw 성능 차이(ms 단위)는 체감에 영향 없음

### 4.2 가중 점수 비교

| 평가 항목 | 가중치 | Python | Go | Rust |
|----------|--------|--------|-----|------|
| MCP SDK 안정성 | 20% | 9 | **9** | 6 |
| 동시성 단순함 | 20% | 5 | **9** | 5 |
| 빌드/개발 속도 | 15% | 8 | **9** | 4 |
| 학습 곡선 | 15% | 10 | **8** | 4 |
| 배포 용이성 | 10% | 3 | **7** | 7 |
| 원시 성능 | 5% | 3 | 7 | **10** |
| Python 연동 | 5% | 10 | 6 | **8** |
| 타입 안전성 | 5% | 5 | 6 | **10** |
| 바이너리 크기 | 5% | 2 | 7 | **9** |
| **가중 합계** | 100% | 5.85 | **7.00** | 5.60 |

### 4.3 결론: Go 선택 근거

1. **MCP SDK v1.0** — 프로덕션 안정성 보장, Rust는 pre-1.0
2. **goroutine 단순함** — Worker 관리, State Machine에 최적 (Zombie Worker류 버그 구조적 방지)
3. **빌드 30x 빠름** — 일상 개발 생산성에 직접 영향
4. **학습 4x 빠름** — 1인 개발 환경에서 결정적
5. **실제 병목은 LLM** — Rust의 raw 성능 이점이 C4에서는 무의미

---

## 5. Hybrid 아키텍처 설계

### 5.1 전체 구조

```
┌─────────────────────────────────────┐
│           Go Core (~20K LOC)         │
│                                      │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ MCP      │  │ State Machine    │ │
│  │ Server   │  │ + Task Store     │ │
│  │ (go-sdk) │  │ (modernc/sqlite) │ │
│  └──────────┘  └──────────────────┘ │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ Worker   │  │ CLI              │ │
│  │ Manager  │  │ (cobra)          │ │
│  │ (gorout.)│  │                  │ │
│  └──────────┘  └──────────────────┘ │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ Git Ops  │  │ Validation       │ │
│  │ (go-git) │  │ Runner (os/exec) │ │
│  └──────────┘  └──────────────────┘ │
├──────────────────────────────────────┤
│           gRPC Bridge (proto)        │
├──────────────────────────────────────┤
│        Python Services (유지)         │
│                                      │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ LSP      │  │ Supervisor /     │ │
│  │ Server   │  │ Agent Router     │ │
│  │ (pygls)  │  │ (litellm)        │ │
│  └──────────┘  └──────────────────┘ │
│  ┌──────────┐  ┌──────────────────┐ │
│  │ Knowledge│  │ Embeddings /     │ │
│  │ Store    │  │ Vector Search    │ │
│  └──────────┘  └──────────────────┘ │
└──────────────────────────────────────┘
```

### 5.2 Go Core 전환 대상

| 컴포넌트 | 현재 Python LOC | Go 예상 LOC | 이유 |
|----------|----------------|-------------|------|
| MCP Server | 6,946 | ~4,000 | go-sdk v1.0 활용 |
| State Machine | 400 | ~300 | select/channel 자연스러움 |
| Task Store | 3,644 | ~2,500 | modernc/sqlite |
| Worker Manager | ~5,000 (daemon 일부) | ~3,000 | goroutine 기반 |
| CLI | 3,500 | ~2,500 | cobra |
| Git Ops | ~2,000 (services 일부) | ~1,500 | go-git |
| Validation Runner | 387 | ~300 | os/exec |
| Config | 1,045 | ~800 | viper |
| Event Logger | ~500 | ~400 | 단순 JSON append |
| **합계** | **~23,400** | **~15,300** | |

### 5.3 Python 유지 대상

| 컴포넌트 | LOC | 유지 이유 |
|----------|-----|----------|
| Supervisor / Agent Router | 12,059 | litellm 의존, LLM 프롬프트 로직 |
| LSP Server | 6,392 | multilspy, pygls 의존 |
| Knowledge Store | 885 + 임베딩 | sentence-transformers, sqlite-vec |
| Memory / Vector | 5,820 | ML 라이브러리 의존 |
| Discovery | 1,979 | LLM 기반 분석 |
| API (REST) | 10,772 | FastAPI, 점진적 이전 가능 |
| Templates | ~2,000 | ML 파이프라인 |
| **합계** | **~40,000** | |

### 5.4 gRPC Interface 설계 (초안)

```protobuf
// c4_bridge.proto

service SupervisorService {
  rpc ReviewTask(ReviewRequest) returns (ReviewResult);
  rpc RouteAgent(RouteRequest) returns (AgentInfo);
  rpc RunCheckpoint(CheckpointRequest) returns (CheckpointResult);
}

service KnowledgeService {
  rpc Search(SearchRequest) returns (SearchResults);
  rpc RecordExperiment(ExperimentData) returns (RecordResult);
  rpc GetEmbedding(EmbeddingRequest) returns (EmbeddingVector);
}

service LSPService {
  rpc FindSymbol(SymbolQuery) returns (SymbolResults);
  rpc GetOverview(FileRequest) returns (SymbolOverview);
}
```

### 5.5 클라우드(Supabase) 연결 토폴로지

#### 현재 상태 분석

현재 C4의 Store 추상화는 Strategy + Factory 패턴으로 잘 설계되어 있다:

```
protocol.py  → StateStore(ABC), LockStore(ABC)
factory.py   → create_state_store(), create_lock_store(), create_task_store()
supabase.py  → SupabaseStateStore(StateStore, LockStore)  # 양쪽 구현
```

**기술 부채**: `create_task_store()`는 SQLite 전용 — Supabase 백엔드에서 `ValueError` raise.

#### 클라우드 토폴로지 옵션

| 옵션 | 구조 | gRPC 영향 | 권장 |
|------|------|-----------|------|
| **A: Python이 Supabase 담당** | Go→gRPC→Python→Supabase | gRPC에 프록시 메서드 추가 (복잡↑) | 비추천 |
| **B: Go가 Supabase 직접 연결** | Go→Supabase (직접), Go→gRPC→Python (LSP/Knowledge만) | **gRPC 변경 없음** | **권장** |
| **C: 이중 연결** | Go→Supabase (State), Python→Supabase (Knowledge) | gRPC 변경 없음 | 상황에 따라 |

#### Option B 상세 (권장)

```
                                    ┌─────────────────┐
Claude Code ──stdin/stdout──→       │    Go Core       │──HTTP──→ Supabase
                                    │  (MCP + State    │         (State, Lock,
                                    │   + Tasks + Lock)│          Task, Realtime)
                                    └───────┬─────────┘
                                            │ gRPC (Python 전용만)
                                            ▼
                                    ┌─────────────────┐
                                    │  Python Sidecar  │
                                    │  (LSP, Knowledge,│
                                    │   Embeddings)    │
                                    └─────────────────┘
```

**gRPC 서비스는 Python 전용 4-5개 RPC로 고정** — 클라우드 추가 시에도 변경 없음:

```protobuf
service C4Python {
  rpc AnalyzeSymbols(SymbolRequest) returns (SymbolResponse);
  rpc SearchKnowledge(KnowledgeQuery) returns (KnowledgeResults);
  rpc GenerateEmbeddings(EmbedRequest) returns (EmbedResponse);
  rpc RunLSP(LSPRequest) returns (LSPResponse);
}
// Supabase는 Go가 직접 연결 — gRPC 범위 밖
```

#### 클라우드 전환 시 Go가 해결하는 기존 문제

| 현재 Python 문제 | Go Hybrid 해결 |
|-----------------|---------------|
| TaskStore 클라우드 미지원 (`factory.py:219`) | Go에서 Supabase TaskStore 신규 구현 |
| asyncio 이벤트 루프 충돌 (Realtime + MCP + Daemon) | goroutine이 독립 관리 |
| `atomic_modify` 낙관적 잠금 한계 (`supabase.py:646`) | Go `database/sql` 트랜잭션 개선 |
| Realtime WebSocket asyncio 의존 | Go native WebSocket + goroutine |
| GIL로 동시 Realtime 구독 제한 | goroutine 무제한 병렬 |

#### Phase 계획 반영

- **Phase 0**: supabase-go SDK PoC (Go에서 Supabase State 읽기/쓰기)
- **Phase 1**: Go StateStore + LockStore + **TaskStore** (Supabase 포함) 구현
- **Phase 2**: Realtime 구독 Go 이전, Python sidecar에서 Supabase 의존성 제거
- **Phase 3**: 팀 격리 (RLS) + Auth 토큰 관리 Go 이전

### 5.6 c4cloud 서버 아키텍처 (팀 협업 모드)

#### 배포 토폴로지: CLI 직접 연결 + 선택적 c4cloud

```
                         ┌───────────┐
                         │ Supabase  │
                         │ (DB+Auth+ │
                         │  Realtime)│
                         └─────┬─────┘
                ┌──────────────┼──────────────────┐
                ▼              ▼                  ▼
          ┌──────────┐   ┌──────────┐      ┌──────────────┐
          │ dev1     │   │ dev2     │      │ c4cloud      │
          │ Go Core  │   │ Go Core  │      │ (Go Server)  │
          │ (CLI)    │   │ (CLI)    │      │              │
          │          │   │          │      │ ┌──────────┐ │
          │ SQLite   │   │ SQLite   │      │ │Chat/LLM  │ │
          │ (offline │   │ (offline │      │ │Workers   │ │
          │  cache)  │   │  cache)  │      │ │Webhook   │ │
          │          │   │          │      │ └──────────┘ │
          │ gRPC→Py  │   │ gRPC→Py  │      │              │
          └──────────┘   └──────────┘      │ Web Dashboard│
                                           │ (Next.js)    │
                                           └──────────────┘
```

- **CLI 사용자** (dev1, dev2): Go Core가 Supabase에 직접 연결. c4cloud 불필요.
- **웹 사용자**: 브라우저 → c4cloud (Chat, Worker 관리) + Supabase (Auth, DB 직접).
- **오프라인**: SQLite로 로컬 동작, 온라인 시 Supabase에 동기화.

#### 3계층 분리

| 계층 | 기술 | 담당 |
|------|------|------|
| **Layer 1: Supabase** | 매니지드 | Auth, DB (Teams/Members/State/Tasks), RLS, Realtime, Storage |
| **Layer 2: Go Server** | `c4 serve` | Chat/LLM 프록시, Cloud Worker(Fly.io), Webhook, C4 오케스트레이션 |
| **Layer 3: Frontend** | Next.js | Web Dashboard, Supabase JS 직접 연결 |

#### Supabase가 대체하는 범위

현재 FastAPI 10K LOC 중:

| 현재 Python 코드 | LOC | Go 전환 후 |
|-----------------|-----|-----------|
| SaaS routes (Teams/SSO/Branding/Reports) | ~4K | **삭제** — Supabase Auth + RLS + Edge Fn |
| Chat (LLM proxy, SSE) | ~1.5K | Go 서버 (~800 LOC) |
| Workspace/Workers (Fly.io) | ~1.5K | Go 서버 (~1K LOC) |
| C4 프록시 routes | ~2K | Go Core에 내장 |
| Auth/Models/Middleware | ~1.5K | **삭제** — Supabase Auth |
| **합계** | ~10K | **Go ~2K + Supabase 설정** |

#### 왜 Go 서버인가

| 대안 | 평가 |
|------|------|
| FastAPI 유지 | Go Core와 별도 Python 런타임 필요 — 배포 복잡도 ↑ |
| Supabase Edge Functions 전용 | Chat/Worker 로직이 Edge Function 한계(실행시간, 메모리) 초과 |
| Next.js API Routes | Node.js 런타임 추가 — 런타임 3개(Go+Python+Node) |
| **Go 서버** | Go Core와 **동일 바이너리** (`c4 serve` 서브커맨드), 런타임 추가 없음 |

#### 통신 흐름

| 시나리오 | 경로 |
|----------|------|
| dev1이 태스크 생성 | Go Core → Supabase 직접 → Realtime → dev2에 알림 |
| 웹 대시보드 조회 | Browser → Supabase JS 직접 → DB 쿼리 |
| 웹에서 Chat | Browser → c4cloud (Go) → Claude API → SSE 응답 |
| Cloud Worker 스폰 | c4cloud (Go) → Fly.io API → Worker 시작 |
| GitHub Webhook | GitHub → c4cloud (Go) → 이벤트 처리 → Supabase |

---

## 6. 장단점 종합

### 6.1 Go 전환의 장점

| 영역 | 현재 | 전환 후 | 개선 |
|------|------|---------|------|
| **설치** | curl + uv + venv | 단일 바이너리 (Core) | 설치 시간 90% 감소 |
| **MCP 시작** | 2-3초 | <100ms | 20-30x |
| **Worker 생성** | ~200ms | ~1us | 200,000x |
| **메모리** | ~50-100MB | ~10-20MB (Core) | 5x |
| **동시성 버그** | asyncio lock 복잡 | goroutine/channel | 구조적 방지 |
| **타입 안전** | Pydantic 런타임 | 컴파일 타임 | 배포 전 검출 |
| **크로스 플랫폼** | Python 버전 의존 | GOOS 한 줄 | macOS/Linux/Windows |

### 6.2 Go 전환의 단점

| 영역 | 비용 | 설명 |
|------|------|------|
| **개발 기간** | 3-4개월 | Go Core 15K LOC + 테스트 ~12K LOC + gRPC ~3K |
| **2개 프로세스** | 운영 복잡도 | Go가 Python sidecar 시작/감시 |
| **gRPC 스키마** | 유지비용 | proto 변경 시 양쪽 재생성 |
| **디버깅** | 추가 노력 | Go/Python/gRPC 중 어디인지 판단 |
| **litellm 부재** | Go에서 LLM | 각 provider SDK 개별 관리 |
| **MCP 도구 추가** | 추가 비용 | Python만 수정 → Go + proto + Python |
| **"단일 바이너리" 반감** | 부분적 | Python 서비스가 살아있는 한 uv도 필요 |

---

## 7. 병렬 진행 계획

### 7.1 전략: Python v1.0 안정화 + Go Core 프로토타입 병렬

```
2026-02        2026-03        2026-04        2026-05        2026-06
────┬──────────┬──────────────┬──────────────┬──────────────┬───→
    │          │              │              │              │
    │ ◆ Phase 0│  ◆ Phase 1   │  ◆ Phase 2   │  ◆ Phase 3   │ ◆ Phase 4
    │ 준비     │  Go Core     │  통합        │  확장        │ 컷오버
    │          │              │              │              │
    ├──────────┼──────────────┼──────────────┼──────────────┤
    │ Python   │ Python 안정화│ Python 안정화│   Python     │ Python
    │ 유지     │ (미완성 20%) │ (버그 수정)  │   Sidecar    │ Sidecar
    │          │              │   전환       │   안정화     │ 운영
    ├──────────┼──────────────┼──────────────┼──────────────┤
    │ Go       │ Go Core      │ Go+Python    │   Go Core    │ Go Core
    │ 프로젝트 │ 프로토타입   │ gRPC 통합    │   기능       │ 프로덕션
    │ 셋업     │ 개발         │ + 테스트     │   완성       │
    └──────────┴──────────────┴──────────────┴──────────────┘
```

### 7.2 Phase 0: 준비 (2주, 2026-02 중순)

**목표**: Go 프로젝트 초기화, 인터페이스 설계

| 작업 | 산출물 | 예상 기간 |
|------|--------|----------|
| Go 모듈 초기화 | `c4-core/go.mod` | 1일 |
| gRPC proto 정의 (초안) | `proto/c4_bridge.proto` | 2일 |
| CI 파이프라인 구성 | GitHub Actions (Go + Python) | 1일 |
| MCP SDK PoC | go-sdk로 hello world MCP 서버 | 2일 |
| SQLite PoC | modernc.org/sqlite로 Task CRUD | 2일|
| State Machine 설계 | Go interface + 전이 테이블 | 2일 |
| 코드 구조 결정 | 패키지 레이아웃 문서 | 1일 |

**동시 진행 (Python):**
- Billing/Auth 모듈 스텁 완성
- Cloud 백엔드 (Supabase) 연동 마무리

### 7.3 Phase 1: Go Core 프로토타입 (6주, 2026-03~04 초)

**목표**: Go Core가 Python MCP 서버를 대체할 수 있는 최소 기능

| 주차 | Go Core 작업 | Python 안정화 작업 |
|------|-------------|-------------------|
| W1-2 | State Machine + Task Store (SQLite) | Realtime 모듈 완성 |
| W3-4 | MCP Server (go-sdk, 핵심 도구 10개) | 모니터링/텔레메트리 마무리 |
| W5 | Worker Manager (goroutine 기반) | 통합 테스트 보강 |
| W6 | CLI (cobra, 기본 명령어) | E2E 테스트 시나리오 |

**Phase 1 완료 기준:**
- [ ] Go MCP 서버가 `c4_status`, `c4_add_todo`, `c4_get_task`, `c4_submit` 처리
- [ ] State Machine 전체 전이 규칙 구현 + 테스트
- [ ] SQLite Task Store CRUD + WAL 모드
- [ ] Worker 3개 동시 실행 가능
- [ ] `c4 status` CLI 명령어 동작

### 7.4 Phase 2: 통합 (4주, 2026-04)

**목표**: Go Core + Python Services가 gRPC로 통신

| 주차 | 작업 | 상세 |
|------|------|------|
| W1 | gRPC 서버 (Python 측) 구현 | Supervisor, Knowledge, LSP 서비스 노출 |
| W2 | gRPC 클라이언트 (Go 측) 구현 | Go Core에서 Python 서비스 호출 |
| W3 | 프로세스 관리 | Go가 Python sidecar 시작/감시/재시작 |
| W4 | 통합 테스트 + tap-compare | 동일 입력에 Python/Go 응답 비교 |

**Phase 2 완료 기준:**
- [ ] Go Core → Python Supervisor로 리뷰 요청 정상 동작
- [ ] Go Core → Python Knowledge로 검색 정상 동작
- [ ] Python 프로세스 crash 시 Go가 자동 재시작
- [ ] tap-compare 테스트 100% 통과 (기존 Python 대비)

### 7.5 Phase 3: 기능 확장 (4주, 2026-05)

**목표**: 나머지 MCP 도구 구현, 성능 최적화

| 주차 | 작업 |
|------|------|
| W1-2 | 나머지 MCP 도구 14개 구현 (코드 분석, 디자인, 디스커버리) |
| W3 | Git 연동 (worktree, hooks, 브랜치 전략) |
| W4 | 성능 튜닝 + 벤치마크 (Python 대비 비교) |

**Phase 3 완료 기준:**
- [ ] 24개 MCP 도구 전체 Go Core에서 처리 (Python 위임 포함)
- [ ] Git worktree 병렬 관리
- [ ] MCP 응답 시간 < 50ms (Python 위임 제외)
- [ ] 메모리 사용량 < 30MB (Go Core 단독)

### 7.6 Phase 4: 컷오버 (2주, 2026-06 초)

**목표**: Go Core를 기본 실행 바이너리로 전환

| 작업 | 상세 |
|------|------|
| 설치 스크립트 업데이트 | Go 바이너리 다운로드 + Python sidecar 설치 |
| 문서 업데이트 | README, 설치 가이드, 아키텍처 문서 |
| 크로스컴파일 빌드 | macOS (arm64, amd64), Linux (amd64) |
| Fallback 메커니즘 | Go Core 실패 시 Python 단독 모드 |
| 릴리스 | v2.0.0-beta |

### 7.7 리스크 관리

| 리스크 | 확률 | 영향 | 대응 |
|--------|------|------|------|
| Go SDK 호환성 변경 | 낮음 (v1.0) | 높음 | v1.x 고정, 테스트 자동화 |
| gRPC 브릿지 성능 병목 | 중간 | 중간 | 벤치마크 Phase 2에서 조기 검증 |
| Python sidecar 불안정 | 중간 | 높음 | Health check + 자동 재시작 |
| 기능 패리티 달성 지연 | 높음 | 중간 | Phase 1 범위를 핵심 도구 10개로 제한 |
| 개발 리소스 부족 | 높음 | 높음 | Phase별 Go/No-Go 판단, 중단 가능 설계 |

### 7.8 Go/No-Go 체크포인트

| 시점 | 판단 기준 | Go 조건 | No-Go 시 |
|------|----------|---------|----------|
| Phase 0 끝 | PoC 성공 여부 | MCP + SQLite PoC 동작 | Python 유지, Go 계획 보류 |
| Phase 1 끝 | 핵심 기능 완성 | 4개 MCP 도구 + State Machine | Phase 1 연장 또는 Python 복귀 |
| Phase 2 끝 | 통합 안정성 | tap-compare 95%+ 통과 | gRPC 대신 REST 전환 검토 |
| Phase 3 끝 | 성능 목표 달성 | MCP < 50ms, 메모리 < 30MB | 최적화 연장 |

---

## 8. 디렉토리 구조 (안)

```
c4/                          # 기존 Python 패키지 (유지)
c4-core/                     # Go Core (신규)
├── go.mod
├── go.sum
├── cmd/
│   └── c4/
│       └── main.go          # CLI 진입점
├── internal/
│   ├── mcp/                 # MCP 서버 (go-sdk)
│   │   ├── server.go
│   │   ├── handlers/        # 도구 핸들러
│   │   └── registry.go
│   ├── state/               # State Machine
│   │   ├── machine.go
│   │   ├── transitions.go
│   │   └── machine_test.go
│   ├── task/                # Task Store
│   │   ├── store.go         # SQLite CRUD
│   │   ├── models.go        # Task, Review, Checkpoint
│   │   └── store_test.go
│   ├── worker/              # Worker Manager
│   │   ├── manager.go
│   │   ├── pool.go
│   │   └── manager_test.go
│   ├── git/                 # Git Operations
│   │   ├── ops.go
│   │   └── worktree.go
│   ├── bridge/              # gRPC 클라이언트 (Python 서비스 호출)
│   │   ├── supervisor.go
│   │   ├── knowledge.go
│   │   └── lsp.go
│   └── config/              # 설정 관리
│       └── config.go
├── proto/                   # gRPC proto 정의
│   └── c4_bridge.proto
└── test/                    # 통합 테스트
    ├── mcp_test.go
    └── e2e_test.go
```

---

## 9. 성공 지표

| 지표 | 현재 (Python) | 목표 (Go Hybrid) | 측정 방법 |
|------|-------------|-----------------|----------|
| MCP 응답 시간 (p50) | ~500ms | **<50ms** | 벤치마크 스크립트 |
| MCP 서버 시작 | 2-3초 | **<100ms** | time 명령 |
| 설치 시간 (신규) | ~2분 | **<10초** (Core) | 신규 머신 테스트 |
| 동시 Worker | 5-10개 | **50+개** | 부하 테스트 |
| 메모리 사용 (Core) | ~80MB | **<30MB** | pprof |
| 바이너리 크기 | ~50MB (venv) | **<20MB** | ls -la |
| 기능 패리티 | 100% | **100%** | tap-compare |

---

## 10. 비용 추정

### 10.1 개발 비용

| 항목 | 예상 규모 | 비고 |
|------|----------|------|
| Go Core 신규 코드 | ~15K LOC | 23K Python → 15K Go |
| Go 테스트 | ~12K LOC | 테스트:코드 0.8:1 |
| gRPC proto + 양쪽 코드 | ~3K LOC | proto 정의 + Go client + Python server |
| 문서 업데이트 | ~2K LOC | README, 설치 가이드, 아키텍처 |
| **총 신규 코드** | **~32K LOC** | |
| **예상 기간** | **16주** (4개월) | 1인 풀타임 |

### 10.2 유지 비용 변화

| 항목 | 현재 | 전환 후 |
|------|------|---------|
| 언어 | Python 1개 | Go + Python 2개 |
| CI 파이프라인 | Python only | Go build + Python test + 통합 |
| 디버깅 복잡도 | 단일 프로세스 | 2 프로세스 + gRPC |
| 의존성 관리 | pyproject.toml | go.mod + pyproject.toml + proto |

---

## 11. 의사결정 요청

### Option A: Go Hybrid 병렬 진행 (권장)

- Python v1.0 안정화와 Go Core 프로토타입을 동시 진행
- Phase 0 (2주) 후 Go/No-Go 판단
- 총 16주, 각 Phase 끝에 체크포인트

### Option B: Python 안정화 우선 → Go 순차 진행

- Python v1.0 완성 (2-3개월) 후 Go 전환 시작
- 총 7-8개월
- 리스크 낮지만 시간 길어짐

### Option C: Python 유지 (Go 전환 보류)

- 현행 유지, 배포 마찰은 Docker 이미지로 우회
- Go 전환은 사용자 수 증가 시 재검토

---

## Appendix A: 참고 자료

- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) (Official Go MCP SDK v1.0)
- [modelcontextprotocol/rust-sdk](https://github.com/modelcontextprotocol/rust-sdk) (Official Rust MCP SDK v0.14)
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (Pure Go SQLite)
- [supabase-go](https://github.com/supabase-community/supabase-go) (Go Supabase Client)
- [go-sqlite-bench](https://github.com/cvilsmeier/go-sqlite-bench) (SQLite 벤치마크)
- [Three Dots Labs: Durable Execution with Go + SQLite](https://threedots.tech/post/sqlite-durable-execution/)
- [Reddit Python→Go Migration (InfoQ)](https://www.infoq.com/news/2025/11/reddit-comments-go-migration/)
- [Google ADK for Go](https://developers.googleblog.com/announcing-the-agent-development-kit-for-go-build-powerful-ai-agents-with-your-favorite-languages/)
- [rig.rs](https://rig.rs/) (Rust LLM Framework)
- [Rust Compiler Performance Survey 2025](https://blog.rust-lang.org/2025/09/10/rust-compiler-performance-survey-2025-results/)
- [Bitfield: Rust vs Go in 2026](https://bitfieldconsulting.com/posts/rust-vs-go)

## Appendix B: 용어

| 용어 | 설명 |
|------|------|
| MCP | Model Context Protocol - LLM과 외부 도구 간 통신 프로토콜 |
| tap-compare | 동일 입력을 Python/Go 양쪽에 보내 응답 동일성 검증 |
| Sidecar | Go Core 옆에서 실행되는 Python 서비스 프로세스 |
| gRPC | Google의 RPC 프레임워크, protobuf 기반 타입 안전 통신 |
| Hybrid | Go Core + Python Services 조합 아키텍처 |

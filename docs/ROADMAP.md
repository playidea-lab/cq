# C4 Roadmap

## Current Version: v0.16.3 (Phase 10.5 — C1 Context Hub + C3 EventBus v4 + Tier 1 Go Native)

현재 버전은 **Go MCP Primary(112 tools: Base 86 + Hub 26), LLM Gateway (4개 Provider 실제 구현), CDP Runner (브라우저 자동화), Cloud Foundation (Supabase), Knowledge Bidirectional Sync, c4 daemon (로컬 작업 스케줄러), C0 Drive (파일 관리), C1 Context Hub (메시징 + 문서 + Context Keeper), C3 EventBus v4 (gRPC daemon + WebSocket bridge + DLQ + Filter v2)**을 포함합니다.

### 핵심 구조

- **Go MCP Server (Primary)** - 112 도구 (Base 86: state/task/file/git/discovery/artifact/lsp/knowledge/research/gpu/soul/team/twin/onboard/lighthouse/llm/cdp/c2/drive/c1), Registry-based, SQLite Store, JSON-RPC Bridge, LLM Gateway, CDP Runner, Hub Client
- **C0 Drive** - Supabase 파일 저장소, metadata JSONB, c4_drive_mkdir 6개 도구, PostgREST URL 인코딩, server-side filtering
- **C1 Context Hub** - Supabase 4 테이블 (channels/messages/participants/summaries), Go MCP 3 도구 (search/mentions/briefing), Context Keeper (LLM 요약), Agent 통합 (notifyKeeper 4-param), participant_id 추적
- **C3 EventBus v4** - gRPC daemon (UDS) + WebSocket bridge + Python sidecar piggyback + CLI + Embedded auto-start + Event Replay + DLQ (16+ event types, 5 default rules, correlation_id, Filter v2)
- **C1 Desktop App** - Tauri 2.x, 4개 프로바이더, Realtime WebSocket, 6-탭 UI (Sessions/Dashboard/Config/Documents/Channels/Events)
- **C1 Views** - SessionsView (provider 자동감지), ChannelsView (메시징 + Realtime + count 로직), DocumentsView (파일+마크다운 편집)
- **Daemon Scheduler** - 로컬 작업 스케줄러, 13 REST API, GPU 할당, 소요시간 예측 (PiQ 대체)
- **LLM Gateway** - 4개 Provider (Anthropic/OpenAI/Gemini/Ollama), 5단계 라우팅, CostTracker, 모델 카탈로그 9종
- **Cloud Layer** - Go PostgREST client (Auth + CloudStore + HybridStore + KnowledgeCloudClient)
- **Python Sidecar** - LSP(Multilspy→Jedi→Tree-sitter), Knowledge Store v2, GPU Scheduler
- **Infra** - Supabase PostgreSQL (13 migrations, RLS, tsvector FTS)

### 지원 기능

- State Machine (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → COMPLETE)
- Multi-Worker (SQLite WAL 모드, race-condition free)
- Direct Mode (c4_claim/c4_report) + Worker Mode (c4_get_task/c4_submit)
- EARS Requirements + ADR (Architecture Decision Records)
- Validation Runner (lint, unit tests)
- Checkpoint System (APPROVE, REQUEST_CHANGES, REPLAN, REDESIGN)
- **Code Analysis Engine** - Multilspy → Jedi → Tree-sitter 3단계 fallback, LSP 7개 도구
- **Knowledge Store v2** - Obsidian Markdown SSOT + FTS5 + Vector hybrid search (RRF)
- **GPU/ML Native** - GPU 감지, 스케줄링, DAG→Task 변환
- **Experiment Tracker** - @c4_track 데코레이터, 메트릭 자동 캡처
- **Artifact Store** - Content-addressable 로컬 저장소
- **Team Collaboration** - Supabase 기반 팀 상태 공유 + Realtime WebSocket
- **C1 Multi-Provider** - Claude Code, Codex CLI, Cursor, Gemini CLI 4개 프로바이더
- **C0 Drive** - 클라우드 파일 저장소 (metadata, URL 인코딩, 보안)
- **C1 Context Hub** - 채널 메시징, Context Keeper (LLM 요약), Agent 통합 (notifyKeeper 4-param)
- **C1 Documents** - 마크다운 파일 편집기, 지속성 (persona/skill/spec/config)
- **C3 EventBus v4** - gRPC daemon (UDS) + WebSocket bridge + DLQ + Filter v2, Python sidecar piggyback, task lifecycle events
- **코드베이스**: Go ~19K + Python 24K + C1 ~13K + Tests ~26K = **~82K LOC**
- **테스트**: Go 937+ (eventbus 87+, c2 29, research 34, gpu 4) + Python 735+ + C1 (Rust 73 + Frontend 81) = **~1,839 tests** (+10 this session)

---

## 최신 추가사항 (2026-02-15)

### C3 EventBus Full Code Review + Tier 1 Go Native Migration Complete ✅

**목표**: C3 EventBus v4 최종 검토 완료, Python sidecar 13개 도구 Go native 마이그레이션 완료

- **C3 v4 Review 이슈 7개 수정**:
  - **Bounded Dispatch**: buffered channel 크기 제한 (1000), queue full 시 oldest event 자동 폐기
  - **gRPC Client Upgrade**: v1.60+ grpc.NewClient() 마이그레이션 (deprecated dial 제거)
  - **DLQ Config**: max_retries (기본 3) + retention_days (기본 7) 명시적 설정
  - **Nested Template Expressions**: $if/$for 중첩 조건식 지원
  - **Template Escaping**: 표현식 파싱 강화
  - **Tests**: dispatcher_test +5, server_test +5 (eventbus 안정성)

- **Tier 1 Go Native Migration 완료**:
  - **13개 도구 마이그레이션**: Research (5) + C2 (6) + GPU (2) → Go native
  - **Python sidecar 도구 축소**: 30 → 17 tools (LSP, Knowledge Store만 남음)
  - **새 패키지**: `internal/research/` (565 LOC) + `internal/c2/` (1,067 LOC)
  - **테스트**: +67개 신규 (research 34 + c2 29 + gpu 4 + migration 18)

- **코드 변경**: +248/-25 LOC (20 files changed)
- **커밋**: 5e589fb, 8f9d7a9, 6f3918c
- **결과**: Python 의존성 축소, Go MCP toolset 강화, sidecar 초기화 시간 단축

**Architecture 영향**:
```
Before:  Go MCP → Python Sidecar (30 proxy tools: LSP 7 + Knowledge 6 + Research 5 + C2 6 + GPU 2 + Review 4)
After:   Go MCP → Go Native (Research/C2/GPU) + Python Sidecar (17 proxy tools: LSP 7 + Knowledge 6 + Review 4)
```

### Python Sidecar Tier 1 Migration to Go Native ✅ (상세 - 최근 완료됨)

**목표**: 13개 Python proxy 도구를 Go native로 마이그레이션 (성능 + 의존성 축소)

- **Research Store (5 tools)**: `internal/research/store.go` — SQLite CRUD 구현 (565 LOC)
  - c4_research_start, c4_research_status, c4_research_record, c4_research_approve, c4_research_next
  - axis_scores, experiments, gaps 메타데이터 저장
  - 테스트: 19개 (CRUD, iteration, gap tracking)
- **C2 Workspace/Profile/Persona (6 tools)**: `internal/c2/` — 문서 생명주기 (1,067 LOC)
  - c4_workspace_create, c4_workspace_load, c4_workspace_save
  - c4_profile_load, c4_profile_save
  - c4_persona_learn (draft/final 비교 → 패턴 추출)
  - 테스트: 29개 (YAML parsing, line-diff, profile versioning)
- **GPU Native (2 tools)**: `internal/gpu/handlers.go` — daemon GPU monitor 활용 (123 LOC)
  - c4_gpu_status, c4_job_submit (기존 proxy 제거, 직접 구현)
  - 테스트: 4개 (GPU detection, job submission)
- **Handler 통합**: `handlers/register.go` — NativeOpts wiring (research, c2, gpu handlers 등록)
- **Proxy 감소**: 30 → 17 tools (13개 Go 대체)
- **코드**: +3,667 LOC (c2 1,067 + research 907 + gpu 123 + tests 1,570)
- **테스트**: Go 687 → 754 (+67)
- **결과**: Python 의존성 제거 (pytorch, tensorflow 등), sidecar 초기화 시간 단축

### Python Sidecar Tier 2 Migration — Knowledge Go Native ✅

- **7개 도구 이동**: knowledge_record, knowledge_get, knowledge_search, experiment_record, experiment_search, pattern_suggest, knowledge_pull
- **새 패키지**: `internal/knowledge/` — Store (SQLite CRUD+FTS5+Markdown SSOT), VectorStore (BLOB cosine), Searcher (RRF hybrid), Sync (Cloud pull)
- **NativeOpts 확장**: KnowledgeStore, KnowledgeSearcher, KnowledgeCloud 필드 추가 (register.go)
- **mcp.go 와이어링**: NewStore → NewVectorStore → NewSearcher 초기화, shutdown cleanup
- **Proxy fallback 유지**: KnowledgeStore 초기화 실패 시 Python proxy로 자동 전환
- **Proxy 감소**: 17 → 10 tools (LSP 7 + Onboard 1 + C2 Doc 2)
- **테스트**: knowledge store 26 + vector 9 + search 13 + sync 8 + handlers 16 = ~72개
- **커밋**: 825cf12, c6f28ab, cd692a7, 01c9244

**Tier 3 다음 계획** (예정 — LSP Go native):
- LSP tools (7): find_symbol, get_symbols_overview, replace/insert/rename_symbol (tree-sitter Go 활용)
- Review tool (1): c2_review (LLM 기반)
- C2 Doc tools (2): parse_document, extract_text (Go PDF/DOCX 라이브러리)
- **결과**: Python sidecar 10 → 0 tools (완전 Go MCP native)

### C3 EventBus v4 — WS Bridge + DLQ + Filter v2 + C1 Events 탭 ✅

**목표**: 실시간 이벤트 스트리밍 + 신뢰성 (DLQ) + 고급 필터링 + Desktop 이벤트 모니터링

- **correlation_id**: Proto Event 신규 field 7, Store/Server/Client 전체 반영 (사건-추적)
- **DLQ (Dead Letter Queue)**:
  - `c4_event_dlq` 테이블 (id, event, error, retry_count, created_at, updated_at)
  - 5개 Store 메서드: InsertDLQ, ListDLQ, IncrementDLQRetry, RemoveDLQ, PurgeDLQ
  - Dispatcher에서 처리 실패 시 자동 DLQ 삽입 (with correlation_id)
  - autoPurge: 7일 이상 재시도 실패한 이벤트 자동 삭제
- **Filter Engine v2**:
  - 7개 연산자: `$eq`, `$ne`, `$gt`, `$lt`, `$in`, `$regex`, `$exists`
  - dot notation 중첩 필드 지원 (e.g., `task.priority > 2`)
  - 기존 1.0 필터와 하위 호환 (기본 `$eq`)
- **Go WebSocket Bridge** (`wsbridge.go`):
  - gobwas/ws 라이브러리 (12KB, production-grade)
  - `/ws/events?pattern=task.*` 엔드포인트 (GET → WebSocket 업그레이드)
  - Embedded config: `WSPort` (기본 9222), message chunking
  - Per-client filter 관리 (goroutine-safe)
- **C1 Desktop Events 탭** (6번째 탭):
  - **Rust WS Client** (`eventbus.rs`): WebSocket 연결 + 재연결 로직
  - **React Hook** (`useEventBus.ts`): events list, filter pattern, real-time updates
  - **EventBusView.tsx + eventbus.css**: 이벤트 스트림 시각화 (타입별 색상, correlation_id 표시)
  - **DLQ 모니터**: Failed events 섹션, retry 버튼, manual purge
- **Webhook HMAC-SHA256**:
  - `X-C4-Signature: sha256=<hex>` 헤더 자동 생성 (dispatcher)
  - `X-C4-Timestamp: <unix_ms>` 타임스탐프 검증 (5분 유효)
  - 외부 webhook consumer 검증용
- **CLI DLQ Subcommand**:
  - `c4 eventbus dlq list [--filter=error] [--limit=100]` — DLQ 조회
  - `c4 eventbus dlq retry <id>` — 재시도
  - `c4 eventbus dlq purge [--before=7d]` — 정리
  - `c4 eventbus monitor` — correlation_id + error 표시
- **내부 개선**:
  - dispatcher Close() 메서드: graceful shutdown
  - publishSidecarEvents: correlation_id 자동 설정
  - eventbus.proto: field 7 추가 (backward-compat)
- **테스트**:
  - Go +22: store (6 DLQ 메서드), dispatcher (11 DLQ 처리/filter/ws), wsbridge (3)
  - Rust +4: eventbus.rs (2), useEventBus.ts (2)
  - 기존 eventbus 테스트 호환성 유지
- **코드**: +1277/-119 LOC (19 files modified + 5 new: wsbridge.go, wsbridge_test.go, eventbus.rs, EventBusView.tsx, eventbus.css, useEventBus.ts)
- **의존성**: gobwas/ws v1.4.0 추가 (direct), 기타 신규 없음
- **결과**: Go eventbus 테스트 65→87 (+22), Rust 69→73 (+4), C1 Desktop 탭 5→6

### Security Fixes + Sidecar Improvements ✅

**목표**: 보안 취약점 수정 및 Sidecar 안정성 강화

- **보안 수정 12개**:
  - SSRF 방지: EventBus dispatcher에서 URL validation 강화 (로컬호스트만 허용)
  - TOCTOU 취약점: c1/documents.rs — stat → read 사이의 경쟁 조건 제거
  - Path Traversal: documents.rs — 파일 경로 프로젝트 루트 검증
  - JWT 보안: messaging.rs — 토큰 검증 강화, 만료시간 체크
  - SQL LIKE escape: c1.go — wildcards 이스케이프 처리
  - RLS 정책: migration 00014 (c1_channels, c1_messages, c1_participants, c1_channel_summaries)
  - json.Unmarshal 에러: 7개 핸들러에서 에러 로깅 추가
  - GPU requires_gpu 단순화
  - path traversal 프로젝트 루트 검증
  - daemon GPU indices 검증

- **Sidecar 안정성**:
  - Health Check with exponential backoff: sidecar.go에서 최대 5회 재시도 (2s→32s)
  - Lazy Start: 첫 proxy 도구 호출까지 sidecar 지연 시작 (startup latency 감소)
  - bridge 패키지 확장: lazy.go (108줄), lazy_test.go (225줄) 신규

- **EventBus 개선**:
  - dispatcher.go: AutoPost 에러 처리, Close() 메서드, logging 표준화
  - Printf → os.Stderr 변경, sync.Once Stop 추가

- **C1 Desktop UI**:
  - ChannelSidebar.tsx: Keyboard accessibility (arrow keys, Enter, Escape)
  - useMessages.ts: Realtime validation 개선

- **코드 정리**:
  - .gitignore에 *.test 패턴 추가
  - accidental test binary 제거

- **테스트**: Rust 44 → 69 (+25), Go all packages pass, +940/-82 LOC
- **결과**: 총 ~1,737 tests, 20 files changed

### C3 EventBus v3 ✅

**목표**: 완전한 이벤트 기반 아키텍처 — CLI + Replay + Rules 관리 + Auto-start

- **gRPC RPC 4개 신규**
  - `ToggleRule(rule_id)` — 규칙 활성화/비활성화
  - `ListLogs(filter)` — 이벤트 로그 조회 (시간/타입 필터)
  - `GetStats()` — 통계 (이벤트 유형별, 규칙별)
  - `ReplayEvents(start_time, end_time)` — 이벤트 재생 (디버깅)
- **Store 메서드 7개 신규**
  - `Purge(before)` — 오래된 로그 삭제
  - `GetStats()` — 이벤트 유형별 통계
- **Embedded auto-start**
  - EventBus daemon UDS 자동 시작 (MCP 서버에 내장)
  - `EnsureServer()` 호출 시 기존 서버 재사용
- **CLI 전면 재작성** (`internal/eventbus/cmd/cli.go`)
  - `logs [--filter="type:task.*"] [--limit=100]` — 로그 조회
  - `rules [--list|--toggle rule_id]` — 규칙 관리
  - `monitor --watch=5s` — 실시간 감시 (ticker)
  - `status` — daemon 상태
  - `replay --start=2h` — 이벤트 재생 + 재처리
- **신규 이벤트 6종** (총 16→16 동일, 기존 10 + 신규 6)
  - `checkpoint.approved`, `checkpoint.rejected`
  - `review.changes_requested`
  - `validation.passed`, `validation.failed`
  - `knowledge.searched`
- **Default rules YAML** (embedded)
  - task.completed → #task-updates
  - checkpoint.approved → #milestones
  - review.changes_requested → #blocked
  - validation.failed → #errors
  - knowledge.searched → #insights
- **테스트**: Go eventbus 34개 (RPC 테스트 포함) + handlers proxy 20개 = **~20개 추가**
- **결과**: Go 819→839+, eventbus 테스트 52→70+, 이벤트 10→16종

### C3 EventBus v1+v2 (이전)

**요약**: gRPC daemon (UDS) + Python sidecar piggyback, task lifecycle wiring
- **테스트**: Go eventbus 25 + proxy 14 + Python events 9 + piggyback 4 = 52개
- **결과**: Go 767→819+, Python 735→748+

---

## 이전 추가사항 (2026-02-14)

### C0 Drive: Supabase 파일 저장소 ✅

**목표**: Supabase 기반 클라우드 파일 저장소 (모바일-퀵스타트)

- **6개 도구**: c4_drive_upload, c4_drive_download, c4_drive_list, c4_drive_delete, c4_drive_info, c4_drive_mkdir
- **메타데이터**: JSONB 지원 (색상, 태그, 사용자 정보)
- **보안**: PostgREST URL-encode, uploaded_by 필드, RLS
- **DB**: migration 00012 (`drive_files` 테이블, metadata JSONB)
- **E2E 테스트**: Supabase 통합 검증 완료

### C1 Documents: 마크다운 편집기 ✅

**목표**: C1 내 마크다운 파일 브라우저 및 편집기

- **DocumentsView**: 4개 탭 (persona/skill/spec/config) + 파일 목록 + 검색
- **DocumentEditor**: 뷰/수정 토글, MarkdownViewer, textarea
- **useDocuments 훅**: list_documents/get_document/save_document IPC
- **연동**: C1 Tauri commands (`list_documents`, `get_document`, `save_document`)
- **지속성**: .c4/documents/ 디렉토리

### C1 Channels + Context Keeper ✅

**목표**: C1 내 채널 기반 메시징 및 자동 요약

- **ChannelsView**: 5개 컴포넌트 (List, Thread, Compose, UserTyping, Picker)
- **Realtime**: c1_messages, c1_channels Supabase subscription
- **Context Keeper**: sqlite_store notifyKeeper hook, 채널 요약 자동 생성
- **테스트**: upsert/trigger 8개

---

## 완료된 Phase

### Phase 1: Core Foundation ✅

**목표**: 기본 상태 머신 및 MCP 서버

- State Machine (INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- MCP 도구 (c4_status, c4_get_task, c4_submit 등)
- LocalFile StateStore
- 기본 Validation Runner

### Phase 2: Multi-Worker Support ✅

**목표**: 동시 작업 지원

- SQLite 기반 StateStore (WAL 모드)
- Scope Lock (동시 작업 충돌 방지)
- Worker Manager (stale recovery)
- Atomic 태스크 할당

### Phase 3: Auto Supervisor ✅

**목표**: 자동화된 체크포인트 처리

- Supervisor Loop (백그라운드 실행)
- Claude CLI Backend
- Stop Hook (작업 완료까지 유지)
- Checkpoint Queue / Repair Queue

### Phase 4: Agent Routing ✅

**목표**: 도메인별 특화 에이전트 자동 선택

- `c4/supervisor/agent_router.py` - 도메인 → 에이전트 매핑
- Agent Chaining (frontend → test → reviewer)
- Task Type Overrides (debug → debugger)
- Handoff Instructions

**구현된 도메인**:

| Domain | Primary Agent | Chain |
|--------|--------------|-------|
| web-frontend | frontend-developer | frontend → test → reviewer |
| web-backend | backend-architect | architect → python → test → reviewer |
| fullstack | backend-architect | backend → frontend → test → reviewer |
| ml-dl | ml-engineer | ml → python → test |
| mobile-app | mobile-developer | mobile → test → reviewer |
| infra | cloud-architect | cloud → deployment |
| library | python-pro | python → docs → test → reviewer |
| unknown | general-purpose | general → reviewer |

### Phase 5: Enhanced Discovery & Design ✅ (Current)

**목표**: 자동화된 요구사항 수집, 아키텍처 설계, 런타임 검증

**구현 완료**:
- **EARS Requirements** (`c4/discovery/specs.py`)
  - 5가지 패턴: Ubiquitous, State-driven, Event-driven, Optional, Unwanted
  - SpecStore로 영속화
- **ADR (Architecture Decision Records)** (`c4/discovery/design.py`)
  - DesignDecision, ArchitectureOption 모델
  - DesignStore로 영속화
- **Component Specification**
  - ComponentDesign, DataFlowStep 모델
- **Verification System** (`c4/supervisor/verifier.py`)
  - 6가지 Verifier: HTTP, CLI, Browser, Visual, Metrics, Dryrun
  - VerifierRegistry로 플러그인 관리
  - 도메인별 기본 검증 자동 설정
- **MCP 도구** (10개 추가)
  - `c4_save_spec`, `c4_list_specs`, `c4_get_spec`
  - `c4_add_verification`, `c4_get_feature_verifications`
  - `c4_discovery_complete`, `c4_design_complete`
  - `c4_save_design`, `c4_get_design`, `c4_list_designs`

**EARS 패턴 예시**:

```yaml
# .c4/specs/{feature}/requirements.yaml
feature: user-authentication
requirements:
  - type: ubiquitous
    text: "The system shall hash all passwords using bcrypt"
  - type: event-driven
    text: "When a user fails login 3 times, the system shall lock the account"
  - type: state-driven
    text: "While the user is logged in, the system shall refresh tokens every 15 minutes"
  - type: optional
    text: "Where 2FA is enabled, the system shall require verification code"
  - type: unwanted
    text: "The system shall not store plain-text passwords"
```

**Verification 예시**:

```yaml
# config.yaml
verifications:
  enabled: true
  items:
    - type: http
      name: "API Health"
      config:
        url: "http://localhost:8000/health"
        expected_status: 200
    - type: cli
      name: "Unit Tests"
      config:
        command: "uv run pytest tests/unit"
        expected_exit_code: 0
    - type: metrics
      name: "Model Performance"
      config:
        metrics: {"accuracy": 0.95}
        thresholds: {"accuracy": {"min": 0.90}}
```

**테스트**:
```bash
uv run pytest tests/unit/test_discovery.py tests/unit/test_verifier.py -v
# 76개 테스트 통과
```

### Phase 5.5: Skill System Enhancement ✅

**목표**: 확장 가능한 스킬 시스템으로 에이전트 라우팅 고도화

**구현 완료**:
- **스킬 스키마 V2** (`c4/supervisor/agent_graph/schema/skill.schema.yaml`)
  - Impact 우선순위: critical, high, medium, low
  - 다중 도메인 지원: `domains`, `domain_specific`
  - 메타데이터: version, author, license, tags
  - 임베디드 규칙: rules with example_bad/example_good
  - 의존성: dependencies (required, optional)
- **Impact 기반 스코어링** (`c4/supervisor/agent_graph/skill_matcher.py`)
  - 공식: `score = impact_weight × (1 + domain_boost) + rule_bonus`
  - impact_weight: critical=2.0, high=1.5, medium=1.0, low=0.5
- **도메인별 스킬 (18개)**
  - 범용: debugging, testing, code-review, error-handling, security-scanning
  - ML/DL: experiment-tracking, model-optimization
  - Data Science: data-analysis, feature-engineering, statistical-testing
  - Frontend: react-optimization, accessibility
  - Backend: api-design, database-optimization, authentication, caching-strategy
  - Infra: deployment, monitoring, container-orchestration
- **스킬 관리 CLI**
  - `c4 skill list` - 스킬 목록
  - `c4 skill validate` - 스킬 검증
  - `c4 skill info` - 스킬 상세
- **외부 스킬 호환**
  - SKILL.md 파서 (Vercel 포맷 호환)
  - 다중 소스 로더 (built-in → project → external)
  - 충돌 해결 전략

**디렉토리 구조**:
```
c4/supervisor/agent_graph/skills/
├── _meta/           # 범용 스킬
├── frontend/        # 프론트엔드 도메인
├── backend/         # 백엔드 도메인
├── ml-dl/           # ML/DL 도메인
├── data-science/    # 데이터 사이언스
├── infra/           # 인프라
└── _groups.yaml     # 스킬 그룹 정의
```

**테스트**:
```bash
uv run pytest tests/unit/test_skill_matcher.py tests/unit/test_agent_graph_loader.py -v
# 61개 테스트 통과
```

---

### Phase 6: Team Collaboration ✅

**목표**: 팀원 간 협업 지원

**구현 완료**:
- **Supabase State Store** (`c4/store/supabase.py`)
  - `SupabaseStateStore` - 프로젝트 상태 저장/조회
  - `SupabaseLockStore` - 분산 잠금 관리
  - RLS (Row Level Security) 적용
- **Team Management** (`c4/services/teams.py`)
  - 팀 생성/수정/삭제
  - 멤버 초대/권한 관리 (RBAC)
  - 팀 설정 관리
- **Cloud Supervisor** (`c4/supervisor/cloud_supervisor.py`)
  - 팀 전체 리뷰 관리
  - 분산 체크포인트 처리
- **Task Dispatcher** (`c4/daemon/task_dispatcher.py`)
  - 우선순위 기반 태스크 분배
  - Peer Review 워크플로우
- **GitHub Integration** (`c4/integrations/github.py`)
  - 팀 권한 동기화
  - 자동 PR/Issue 생성
- **Branding Middleware** (`c4/api/middleware/branding.py`)
  - 커스텀 도메인 브랜딩
  - TTL 캐시 (60초 기본)

**DB 스키마** (`infra/supabase/migrations/`):
- `00001_c4_projects.sql` - 프로젝트 테이블
- `00002_c4_tasks.sql` - 태스크 테이블
- `00003_c4_events.sql` - 이벤트 로그
- `00004_teams_and_members.sql` - 팀/멤버 테이블
- `00005_team_settings.sql` - 팀 설정
- `00006_team_branding.sql` - 브랜딩 설정

**아키텍처**:

```text
┌─────────────┐        ┌─────────────┐
│ Claude Code │        │ Claude Code │
│ + C4 Daemon │        │ + C4 Daemon │
└──────┬──────┘        └──────┬──────┘
       │                      │
       └──────────┬───────────┘
                  ▼
           ┌────────────┐
           │  Supabase  │
           │  (State)   │
           └────────────┘
```

### Phase 6.5: MCP Advanced Tools ✅

**목표**: 코드 분석 및 문서화 자동화 (완료)

... (생략) ...

### Phase 6.6: UX & Platform Excellence ✅

... (생략) ...

### Phase 6.7: Reliability & Observability ✅

**목표**: 시스템 안정성 강화 및 운영 가시성 확보

**구현 완료**:
- **OpenTelemetry Tracing** (`c4/monitoring/tracing.py`)
  - 분산 트레이싱 (워커 간 호출 추적)
  - OTLP Exporter 지원
  - FastAPI 자동 계측
- **Prometheus Metrics** (`c4/monitoring/prometheus.py`)
  - 6개 핵심 메트릭: API 요청, 활성 워크스페이스, LLM 토큰, 태스크 처리 시간
  - Counter, Gauge, Histogram 타입 지원
- **Self-Healing Queue** (`c4/daemon/repair_analyzer.py`)
  - AI 기반 실패 원인 분석 (8개 카테고리)
  - 자동 수정 제안 및 auto_fixable 판단
  - RepairQueueItem으로 복구 추적
- **Cost-Aware Routing** (`c4/supervisor/cost_optimizer.py`)
  - 복잡도 기반 모델 선택 (haiku/sonnet/opus)
  - 예산 관리 및 알림 (BudgetWarning, BudgetExceeded)
  - 사용량 추적 (`c4/supervisor/usage_tracker.py`)
- **Context Slimmer** (`c4/utils/slimmer.py`)
  - 로그 슬리밍 (에러 패턴 추출)
  - JSON 압축 (배열/깊이 제한)
  - 코드 시그니처 추출

**테스트**: 80 passed (cost_optimizer 38, tracing 2, usage_tracker 40)

### Phase 6.8: LSP Server ✅

**목표**: 에디터에서 직접 코드 인텔리전스 제공 (MCP와 독립)

**구현 완료 (Phase 1 & 2)**:
- **pygls 기반 LSP 서버** (`c4/lsp/server.py`)
  - `C4LSPServer` 클래스 - CodeAnalyzer 통합
  - stdio 및 TCP 모드 지원
- **핵심 LSP 기능**:
  - `textDocument/hover` - 심볼 정보 (docstring, signature)
  - `textDocument/definition` - 정의로 이동
  - `textDocument/references` - 참조 찾기
  - `textDocument/documentSymbol` - 파일 아웃라인
  - `workspace/symbol` - 전역 심볼 검색
  - `textDocument/completion` - 자동 완성 (Phase 2)
  - `completionItem/resolve` - 완성 항목 상세 정보 (Phase 2)
- **MCP 통합 도구** (Phase 2):
  - `c4_lsp_start` - LSP 서버 시작 (TCP 모드, 백그라운드 스레드)
  - `c4_lsp_status` - LSP 서버 상태 조회
  - `c4_lsp_stop` - LSP 서버 중지
- **CLI 엔트리포인트**: `uv run c4-lsp` 또는 `uv run python -m c4.lsp`

**아키텍처**:

```text
┌─────────────────────────────────────┐
│         c4d (single process)        │
│                                     │
│  ┌─────────────┐  ┌──────────────┐  │
│  │ LSP Server  │  │  MCP Server  │  │
│  │  (pygls)    │  │   (기존)     │  │
│  └──────┬──────┘  └──────┬───────┘  │
│         │                │          │
│         └───────┬────────┘          │
│                 ▼                   │
│        ┌──────────────┐             │
│        │ CodeAnalyzer │             │
│        │ (tree-sitter)│             │
│        └──────────────┘             │
└─────────────────────────────────────┘
          │               │
          ▼               ▼
     VS Code/IDE     Claude Code
```

**사용법**:

```bash
# stdio 모드 (에디터 연동)
uv run python -m c4.lsp

# TCP 모드 (디버깅용)
uv run c4-lsp --tcp --port 2087
```

**테스트**:

```bash
uv run pytest tests/unit/lsp/ -v
# 22개 테스트 통과
```

**향후 계획 (Phase 2-3)**:
- `textDocument/completion` - 자동 완성
- MCP 통합 (`c4_lsp_start`, `c4_lsp_status`)
- Go, Rust 언어 확장 (tree-sitter 플러그인)

### Phase 6.9: PiQ 완전 흡수 - Native GPU/ML Support ✅

**목표**: piq 프로젝트를 C4에 완전 흡수하여 GPU/ML 워크로드를 네이티브 지원

**흡수 범위**: ~14,750줄 흡수, ~58,520줄 폐기 (Hub, PiDrive, Data, Templates)

**구현 완료**:
- **GPU Monitor** (`c4/gpu/monitor.py`) - CUDA/MPS/CPU 백엔드 감지, VRAM 기반 할당
- **GPU Scheduler** (`c4/gpu/scheduler.py`) - Multi-GPU 스케줄링 (DDP/FSDP)
- **DAG→Task 변환** (`c4/gpu/dag.py`) - DAG 정의, 검증, 위상정렬, C4 태스크 변환
- **Experiment Tracker** (`c4/tracker/`) - `@c4_track` 데코레이터
  - stdout 메트릭 파싱 (regex), AST 코드 분석, 데이터 프로파일링
  - Git 컨텍스트, 실행 환경 캡처
- **Local Artifact Store** (`c4/artifacts/`) - Content-addressable (SHA256) 로컬 저장소
  - 3-Tier 분류 (SOURCE/DATA/OUTPUT), 자동 감지 (*.pt, *.pkl 등)
- **Knowledge Store v2** (`c4/knowledge/`) - Obsidian-style Markdown SSOT + Hybrid Search
  - **DocumentStore**: `.c4/knowledge/docs/*.md` YAML frontmatter + body
  - **4가지 문서 유형**: experiment(`exp-`), pattern(`pat-`), insight(`ins-`), hypothesis(`hyp-`)
  - **Hybrid Search**: sqlite-vec(Vector) + FTS5(Keyword), RRF merge
  - **Backlink**: `[[doc-id]]` 참조로 지식 그래프 구성
  - **MCP Tools**: `c4_knowledge_search`/`record`/`get` (v2) + legacy 위임(`c4_experiment_*`, `c4_pattern_suggest`)
  - **c4/memory/ 완전 삭제**: -12,878 LOC, `c4/analysis/git/`로 git 분석 분리
  - **P0/P1 수정**: 필터 O(m+n) 최적화, 트랜잭션 보호, asyncio crash 제거, 메타데이터 검증
- **Hook Registry** (`c4/hooks/`) - 생명주기 훅 (BEFORE_SUBMIT, AFTER_COMPLETE, ON_FAILURE)
  - 빌트인: KnowledgeHook, ArtifactHook
- **Task 모델 확장** - `GpuTaskConfig`, `ExecutionStats`, `ArtifactSpec` 필드 추가
- **Config 확장** - `gpu`, `tracker`, `artifacts`, `experiments` 섹션 추가
- **MCP Tools** - `c4_gpu_status`, `c4_job_submit`, `c4_knowledge_search`/`record`/`get`, `c4_artifact_list` 등
- **Worker GPU 타입** - GPU 요구사항 매칭, `is_piq_project` 자동 활성화
- **ABC 인터페이스** - Cloud 확장 포인트 (ArtifactStore, KnowledgeStore, GpuScheduler, ExperimentTracker)
- **Legacy 완전 정리** - store.py/aggregator.py/miner.py 삭제 (-1,158 LOC), DocumentStore→KnowledgeStore ABC 상속

**테스트**: 358+ tests (knowledge 92 + search/embeddings 25 + mcp 20 + hooks 8 + migration 18 + e2e 10 + gpu/tracker/artifacts 185)

```
c4/
├── gpu/          # GPU 감지, 스케줄링, DAG 변환
├── tracker/      # @c4_track 데코레이터, 메트릭 캡처
├── artifacts/    # 로컬 아티팩트 저장소
├── knowledge/    # Obsidian Markdown + FTS5 + Vector hybrid search
├── analysis/git/ # Git 분석 (commit_analyzer, story_builder, dependency_inferrer)
└── hooks/        # 생명주기 훅 레지스트리
```

### Phase 6.11: C1 (See) — Multi-LLM Project Explorer ✅

**목표**: C1 데스크톱 앱을 Claude Code 전용 뷰어에서 모든 LLM 코딩 도구 통합 탐색기로 확장

**구현 완료**:
- **Provider Trait 추상화** (`c1/src-tauri/src/providers/mod.rs`)
  - `ProviderKind` enum + `SessionProvider` trait (enum dispatch)
  - `detect_providers()` 자동 감지, `get_provider()` 팩토리
- **4개 프로바이더**:
  - Claude Code (`providers/claude_code.rs`) — 기존 로직 추출
  - Codex CLI (`providers/codex_cli.rs`) — `~/.codex/sessions/` JSONL 파싱
  - Cursor (`providers/cursor.rs`) — `state.vscdb` SQLite READONLY (composerData/bubbleId)
  - Gemini CLI (`providers/gemini_cli.rs`) — 스텁 (설치 감지만)
- **IPC 커맨드**: `list_providers`, `list_sessions_for_provider`, `get_provider_session_messages`
- **프론트엔드**: ProviderTabs, OverviewPanel (프로바이더 카드), UsagePanel (CSS-only 바 차트)
- **리뷰**: R-CVR-001~013 전체 APPROVE

**테스트**: Rust 16/16, Vitest 29/29

### Phase 6.12: Go Core Foundation ✅

**목표**: 성능 크리티컬 컴포넌트를 Go로 마이그레이션 (기반 구축)

**구현 완료**:
- **c4-core/** — Go 기반 코어 (14 packages)
- **State Machine** (Go) — Python과 동일 상태 전이
- **SQLite TaskStore** (Go) — Python DB 호환 (동일 스키마)
- **MCP Server** (Go) — stdio transport, 10 핸들러 (초기)
- **CLI** (cobra) — run, status, stop, add-task, mcp

**테스트**: 275 passing (`go test ./...`)

### Phase 6.13: Go MCP Primary + Cloud Cleanup ✅

**목표**: Go를 MCP Primary 서버로 전환, Python Cloud 코드 대폭 정리

**5-Phase 실행 완료**:

#### Phase 1: Python Cloud 코드 삭제 (-73,000+ LOC)
- **17+ 모듈 삭제**: `c4/api/`, `c4/billing/`, `c4/monitoring/`, `c4/realtime/`, `c4/cloud/`, `c4/web/`, `c4/ui/`, `c4/telemetry/`, `c4/templates/`, `c4/sandbox/`, `c4/web_worker/`, `c4/connection/`, `c4/workspace/`, `c4/store/supabase.py` 등
- **pyproject.toml** 정리: fastapi, uvicorn, supabase, opentelemetry, prometheus, stripe 등 삭제
- **관련 테스트** 정리: Cloud 전용 테스트 디렉토리 삭제

#### Phase 2: JSON-RPC Bridge 구현
- **Python Bridge Server** (`c4/bridge/rpc_server.py`): JSON-RPC over TCP
  - 11 methods: find_symbol, get_symbols_overview, replace_symbol_body, insert_before/after_symbol, rename_symbol, knowledge_search/record/get, gpu_status, job_submit
- **Go Bridge Client** (`c4-core/internal/bridge/`): sidecar 자동 관리, health check
- **Sidecar Lifecycle**: `C4_BRIDGE_PORT=<port>` stdout 프로토콜, graceful shutdown

#### Phase 3: Go MCP 도구 확장 (10 → 47개)
- **Go Native (21개)**: 상태, 태스크, 파일(find/read/replace/create), git(worktree/history/commits), validation
- **Go + SQLite (13개)**: spec(save/get/list), design(save/get/list), discovery/design_complete, artifact(save/get/list), checkpoint, ensure_supervisor
- **JSON-RPC Proxy (16개)**: LSP 7개(find/get_symbols/replace/insert/rename) + Knowledge 6개(search/record/get/experiment/pattern) + GPU 2개(status/submit)

#### Phase 4: Go MCP Primary 전환
- **Registry-based**: `mcp.Registry` + `handlers.RegisterAllHandlers()`
- **SQLiteStore**: `handlers.Store` 인터페이스 SQLite 구현 (11 methods)
- **Sidecar 자동 관리**: Go 시작 시 Python sidecar 자동 spawn
- **Fallback**: Go MCP 실패 시 Python MCP fallback
- **바이너리**: `c4-core/bin/c4` (12MB)

#### Phase 5: canvas-app → c1 리네임
- `canvas-app/` → `c1/` 디렉토리 이동

**아키텍처**:
```
Claude Code → Go MCP Server (stdio, 47 tools)
                ├→ Go native (21개)
                ├→ Go + SQLite (13개)
                └→ JSON-RPC proxy (16개) → Python Sidecar
                                            ├→ LSP (multilspy, Jedi, tree-sitter)
                                            ├→ Knowledge Store (FTS5 + Vector)
                                            └→ GPU Scheduler
```

**테스트**: Go 15 packages pass, Python 2269/2270 pass

### Phase 6.10: Worker Lifecycle Hardening ✅

**목표**: 좀비 워커 버그 수정 및 워커 생명주기 강화

**근본 원인 6가지 수정**:
1. **`_sync_merged_tasks()` worker 상태 미업데이트** → merge 시 worker idle 전환 추가
2. **`_sync_state_consistency()` done queue 누락** → busy worker 중 task 완료/누락 감지 추가
3. **`cleanup_stale()` 미호출** → `c4_get_task` 경로에서 주기적 호출 추가
4. **`max_idle_minutes` 기본값 0** → 60분으로 변경
5. **MCP 세션 종료 시 정리 없음** → TTL 30분 기반 자동 제거
6. **수동 정리 방법 부재** → `c4_cleanup_workers` MCP tool 추가

**테스트**: 10 passed (`tests/unit/test_zombie_worker_fix.py`)

---

### Phase 6.14: Go Gap Fix + Persona Evolution ✅

**목표**: Go 번역 갭 수정 (5 HIGH) + Persona 기반 팀 도구 비전 첫 단계

**Part A: Go 번역 갭 수정**:
- **A1: Active Claim File** — `ClaimTask()` → `.c4/active_claim.json` 생성, `ReportTask()` → 삭제
- **A2: State Machine 연결** — `TransitionState()`가 23-rule 전이 테이블 검증 사용 (단순 from/to 덮어쓰기 제거)
- **A3: Config 연결** — `config.Manager` 로드 → MCP 서버에 주입 (경제 모드, model 힌트 등)
- **A4: Worker Loop** — **DEFERRED** (현재 MCP 도구 경로로 충분, CLI 단독 사용 시나리오 미존재)
- **A5: c4_find_referencing_symbols** — proxy 도구 등록 (Python sidecar 위임)

**Part B: Persona Evolution Foundation**:
- **B1: persona_stats 테이블** — 태스크 완료 시 페르소나별 성과 자동 추적
- **B2: Knowledge Auto-Record** — `SubmitTask()`/`ReportTask()` 완료 시 experiment 문서 자동 생성 (sidecar proxy, best-effort)
- **B3: Persona MCP 도구** — `c4_persona_stats` (성과 통계), `c4_persona_evolve` (진화 제안)
- **B4: Team Identity** — `c4_whoami` 도구 + `.c4/team.yaml` 팀 구성 관리

**결과**: 47 → **51 MCP 도구** (+4: find_referencing_symbols, persona_stats, persona_evolve, whoami)
**스키마 자동 초기화**: MCP 서버 시작 시 c4_tasks, c4_state, c4_checkpoints, persona_stats 테이블 자동 생성

**테스트**: Go 9 packages pass, Python 402 pass

### Phase 6.15: Sidecar Stability + LSP Onboarding ✅

**목표**: Python Sidecar 안정성 강화, 프로젝트 자동 분석

**구현 완료**:
- **Sidecar Rename**: `grpc_server.py` → `rpc_server.py`, `C4_GRPC_PORT` → `C4_BRIDGE_PORT`
- **Ping Health Check**: Python `Ping` 메서드 + Go `Sidecar.Ping()` JSON-RPC ping
- **Auto-Restart**: `Sidecar.Restart()` (max 3회), `BridgeProxy` conn 실패 시 자동 재시작+retry
- **c4_onboard MCP 도구**: 프로젝트 구조 자동 분석 → `pat-project-map.md` 생성
  - 언어/프레임워크 감지, 심볼 추출 (top 100), 타입 계층, 엔트리포인트, 모듈 의존성 그래프

**결과**: 51 → **52 MCP 도구** (+1: c4_onboard)

### Phase 6.16: Review-as-Task + Soul System ✅

**목표**: 리뷰 태스크 자동 생성, 사용자별 판단 시뮬레이터

**Review-as-Task**:
- **c4_add_todo**: T- 접두사 + `review_required=true`(기본) → R-XXX 자동 생성
- **c4_request_changes**: R-task 거부 → T-XXX-(N+1) + R-XXX-(N+1) 자동 생성
- **AssignTask 확장**: R-task 할당 시 parent T의 commit_sha/files를 ReviewContext로 주입
- **Config**: `max_revision: 3` (REQUEST_CHANGES 횟수 제한), dead config 4필드 삭제

**Soul System (3-Layer 아키텍처)**:
- **Persona** (팀 기본, `.c4/personas/`) + **Soul** (개인 override, `.c4/souls/{user}/`) → **Merged** 판단 기준
- **MCP 도구 3개**: `c4_soul_get` (CRUD), `c4_soul_set` (atomic write), `c4_soul_resolve` (병합)
- **Workflow-Soul 연동**: 워크플로우 단계별 활성 역할 자동 전환
- **Learn Loop**: SubmitTask/ReportTask → autoLearn → Soul Learned 섹션 자동 축적
- **c4_whoami 확장**: 복수 roles, soul_files 표시

**결과**: 52 → **56 MCP 도구** (+4: c4_request_changes, c4_soul_get, c4_soul_set, c4_soul_resolve)
**테스트**: Go 9 packages pass (soul_test.go 22개 포함), Python 428 pass

### Phase 6.17: c4-swarm Agent Teams ✅

**목표**: deprecated Worker 스폰 → Agent Teams 기반 협업형 병렬 실행

- **3가지 모드**: standard (구현), `--review` (읽기전용 3명), `--investigate` (가설 경쟁)
- **차별화**: `/c4-run` = fire-and-forget 독립 Worker, `/c4-swarm` = TeamCreate+SendMessage 협업
- **흐름**: c4_status → TeamCreate → TaskCreate 매핑 → Task(team_name=...) 스폰 → coordinator 모니터링 → shutdown → TeamDelete
- **Review 모드**: Security/Performance/TestCoverage 3명, plan 모드(읽기전용)

### Phase 7: Digital Twin — 토론형 성장 시스템 ✅

**목표**: C4를 거울이 아닌 토론 상대로 — 패턴 감지, 도전, 성장 추적

**구현 완료**:
- **Pattern Detection Engine** (`twin.go`) — 6가지 SQL 기반 패턴 자동 감지
  - Domain variance, Trend shift, Repeated failures, Checkpoint rejection, Feedback keywords, Speed change
- **Contextual Enrichment** — 기존 도구 응답 자동 강화
  - `c4_claim` → `twin_context` (패턴 + Soul reminder)
  - `c4_checkpoint` → `twin_review` (히스토리 + growth note)
  - `c4_submit`/`c4_report` → `twin_growth` 주간 스냅샷 자동 기록
- **c4_reflect MCP 도구** (#57) — Digital Twin 대화 인터페이스
  - Focus: patterns, growth, challenges, all
  - Identity, patterns, growth, challenges, soul_summary 반환
- **Project-as-Persona** — 프로젝트도 하나의 역할
  - `SetProjectRoleForStage()` → 모든 stage에 project role 동적 추가
  - `injectSoulContext()` → 3-way merge (role + personal + project)
- **Growth Dashboard** — 주간 메트릭 추적
  - `twin_growth` 테이블: approval_rate, avg_review_score, tasks_completed
  - Milestones: 승인률 80%/90%, 태스크 20/50/100 달성 감지

**결과**: 56 → **57 MCP 도구** (+1: c4_reflect)
**테스트**: Go 9 packages pass (twin_test.go 23개 포함), Python 446 pass

### Phase 7.5: PDF/Cursor 가이드 고도화 + Lighthouse Pattern ✅

**목표**: Claude Code 70팁 PDF + Cursor Self-Driving 블로그 기반 시스템 고도화, Spec-as-MCP 패턴

**PDF/Cursor 가이드 고도화**:
- **Go PostToolUse hook**: `.go` 수정 시 자동 `go vet` (hooks.json)
- **승인 명령어 감사**: `auditApprovedCommands()` → c4_reflect permission_audit 섹션
- **HANDOFF.md 자동 생성**: c4-finish Step 4.5 — 세션 간 컨텍스트 압축
- **Worktree 자동 생성**: AssignTask에서 `git worktree add` 실제 실행
- **c4-swarm 고도화**: domain→agent 매핑 12개, sub-planner 모드, 핸드오프, anti-fragility
- **Agent trace logging**: c4_agent_traces 테이블, c4_reflect recent_traces

**Lighthouse Pattern (Spec-as-MCP)**:
- **c4_lighthouse MCP 도구** (#58): register/list/get/promote/update/remove 6개 action
- **Registry 확장**: `Replace()`, `Unregister()` 메서드 — stub→live 교체
- **Stub 팩토리**: 호출 시 spec/contract 반환 (TDD와 동일 원리, API 계약 수준)
- **Startup Loader**: DB의 stub lighthouse를 서버 시작 시 자동 로드
- **Status 흐름**: stub → implemented (promote) / deprecated (remove)
- **충돌 방지**: core 도구 이름 거부, 중복 lighthouse 거부

**결과**: 57 → **58 MCP 도구** (+1: c4_lighthouse)
**테스트**: Go 9 packages pass (lighthouse_test.go 11개 포함)

---

## Phase 8: C4 Cloud (v0.13.0)

**목표**: Go 기반 Cloud 재구축 — Local-first + Supabase 동기화
**참고**: Phase 6.13에서 Python Cloud 코드(-73K LOC) 삭제됨. Go 기반으로 재설계 완료.

### Phase 8.1: Cloud Foundation ✅

**목표**: Go 기반 인증, 클라우드 스토어, 하이브리드 동기화

- **Authentication**: GitHub OAuth (Go), token 저장/refresh, CLI `c4 auth login/logout/status`
- **CloudStore**: PostgREST 기반 handlers.Store 완전 구현 (13 메서드)
- **HybridStore**: Local-first (SQLite 즉시 + Cloud 비동기 push)
- **SQL Migrations**: 8개 (projects, tasks, state, checkpoints, personas, growth, traces, lighthouses)
- **RLS**: 모든 테이블에 Row-Level Security + `c4_is_project_member()` 헬퍼
- **테스트**: Auth 15 + CloudStore 20 + Hybrid 7 = 50개 신규

### Phase 8.2: C1 Cloud Dashboard ✅

**목표**: 실시간 팀 대시보드, 양방향 동기화

- **Realtime WebSocket**: Supabase Realtime v2 (Phoenix Channels), auto-reconnect, heartbeat
- **Cloud Pull**: PostgREST GET + row_version 충돌 해결 (last-write-wins)
- **TeamView 확장**: 4-탭 상세 뷰 (Overview/Checkpoints/Growth/Traces)
- **React Hook**: `useRealtimeSync.ts` — cloud-update/realtime-status 이벤트 구독
- **테스트**: Rust 36→42, Frontend 80→81

### Phase 8.3: Knowledge Store Cloud ✅

**목표**: 로컬 knowledge → 클라우드 자동 동기화, 팀 검색

- **SQL**: `c4_documents` 테이블 (tsvector FTS, weighted A/B/C, GIN index, RLS)
- **Go Client**: KnowledgeCloudClient (5 methods: Sync/Search/Get/List/Delete)
- **Proxy Interceptor**: knowledge_record → async cloud push, knowledge_search → cloud merge
- **C1 React**: TeamView 5번째 Knowledge 탭 (검색, 문서 목록, type badges)
- **테스트**: Go 10 + Rust 2 + Frontend 1 = 13개 신규

### Phase 8.4: Knowledge Bidirectional Sync ✅

**목표**: Cloud → Local 양방향 knowledge 동기화

- **c4_knowledge_pull MCP 도구** (#59): cloud → local sync (version 비교, force 옵션)
- **KnowledgeSyncer 확장**: 2→4 메서드 (ListDocuments, GetDocument 추가)
- **content_hash 변경 감지**: ListDocuments select에 content_hash 필드 추가
- **Re-push 방지**: cloud → local은 BridgeProxy.Call("KnowledgeRecord") 직접 호출 (MCP handler 우회)
- **테스트**: Pull 핸들러 9개 + content_hash 1개 = 10개 신규

---

## Phase 9: LLM Gateway (v0.14.0)

**목표**: 멀티 LLM 오케스트레이션 프레임워크 — 인터페이스 + 라우팅 + 비용 추적

### Phase 9.1: Gateway Framework ✅

- **Provider 인터페이스**: `Chat(ctx, *ChatRequest) → *ChatResponse` 표준 인터페이스
- **모델 카탈로그**: 9개 모델 가격/스펙 (Claude Opus/Sonnet/Haiku, GPT-4 Turbo/4o/4o-mini, Gemini Flash/Pro, Llama 70B) + 9개 Aliases
- **Gateway 코어**: 5단계 라우팅 (direct provider/model → alias → taskType route → default route → default provider)
- **CostTracker**: 인메모리 비용 집계 (provider별, model별, 세션 누적)
- **MockProvider**: 테스트용 고정 응답 프로바이더
- **Config**: `LLMGatewayConfig` (enabled, default, providers map with api_key_env, base_url)
- **MCP 도구 3개** (#60-62): `c4_llm_call`, `c4_llm_providers`, `c4_llm_costs`
- **서버 와이어링**: `config.LLMGateway.Enabled` → Gateway 자동 초기화 + 핸들러 등록
- **테스트**: Gateway 19 + Handler 10 + Config 2 = **31개 신규** (전체 회귀 없음)
- **외부 의존성**: 추가 없음 (stdlib + 기존 의존성만)

**결과**: 59 → **62 MCP 도구** (+3)

### Phase 9.2: Provider Implementations ✅

**목표**: 실제 API 연결

- **4개 Provider**: stdlib `net/http` + `encoding/json`만 사용 (외부 의존성 0)
- **Anthropic**: `x-api-key` 헤더, `system` 별도 필드, `anthropic-version: 2023-06-01`
- **OpenAI**: `Authorization: Bearer` 헤더, system prompt → system role message prepend
- **Gemini**: API key → URL query param, `systemInstruction` 필드, role 매핑 (assistant→model)
- **Ollama**: 로컬 전용, API key 불필요, stream: false, 300s timeout
- **Factory**: `NewGatewayFromConfig(cfg)` — config 기반 Provider 자동 생성 + 등록
- **테스트**: httptest 기반 25개 신규 (총 44개 LLM 패키지)

---

### Phase 10: CDP Runner (v0.15.0) ✅

**목표**: 브라우저 자동화 — Chromium 앱에 CDP 연결하여 JS 배치 실행

- **신규 패키지**: `internal/cdp/` — chromedp 기반 범용 CDP Runner
- **Runner.Execute()**: 기존 Chromium 앱에 CDP 연결 → JS 배치 실행 (per-request, stateless)
- **Runner.ListTargets()**: 브라우저 탭/타겟 목록 조회
- **MCP 도구 2개** (#63-64): `c4_cdp_run` (JS 실행), `c4_cdp_list` (탭 목록)
- **보안**: localhost only 기본값, `validateURL()` 강제
- **timeout**: 기본 30초, 최대 300초
- **테스트**: Runner unit 5 + handler 9 = 14개
- **핵심 철학**: "11번 tool call → 1번 스크립트 실행" 패턴으로 토큰 80% 절감

**결과**: 62 → **64 MCP 도구** (+2)

### Phase 10.1: SQLite Hardening ✅

**목표**: SQLITE_BUSY_SNAPSHOT(517) 방지 및 deadlock 수정

- **openDB()**: 중앙 DB 열기 함수 — `MaxOpenConns(1)` + `PRAGMA busy_timeout=5000` + `PRAGMA journal_mode=WAL`
- **Deadlock 수정**: `AssignTask`에서 `logTrace()`를 `tx.Commit()` 이후로 이동
- 6개 CLI 파일(`mcp.go`, `status.go`, `add_task.go`, `run.go`, `stop.go`, `root.go`) 통일

### Phase 10.2: R-task Cascade + Orphan GC ✅

**목표**: T-task 완료 시 연관 R-task 자동 정리, 고아 리뷰 감지

- **completeReviewTask() 헬퍼**: `sqlite_store.go`에서 parent T-task done → 연관 R-task 자동 done 처리
- **SubmitTask 통합**: Worker mode에서 T-task complete 후 cascade 호출
- **ReportTask 통합**: Direct mode에서 T-task complete 후 cascade 호출
- **GetStatus() 메트릭**: `orphan_reviews` 필드 추가 (parent T done인데 R-task pending 건수)
- **ProjectStatus 구조체**: `OrphanReviews` 필드 추가
- **테스트**: 5개 신규 테스트 (cascade 정상, no R-task, already done, report cascade, orphan count)
- **결과**: 64 → **64 MCP 도구** (변화 없음, 내부 정리)

### Phase 10.3: c4 daemon — 로컬 작업 스케줄러 (2026-02-14) ✅

**목표**: PiQ daemon을 Go 기반으로 재작성하여 로컬 환경에서 작업 스케줄링

**구현 완료**:
- **신규 패키지**: `internal/daemon/` — 6개 모듈 (~2.5K LOC)
  - `models.go` — JobStatus enum, Job/Task struct, Request/Response types
  - `store.go` — SQLite 저장소 (MaxOpenConns(1), WAL, atomic ID gen)
  - `scheduler.go` — 프로세스 실행, GPU 할당, Setpgid 프로세스 그룹
  - `gpu.go` — nvidia-smi CSV 파싱
  - `server.go` — PiQ 호환 REST API (13개 엔드포인트)
  - `estimator.go` — 명령어 정규화, 4단계 소요시간 예측

- **CLI**: `c4 daemon [--port 7123] [--data-dir ~/.c4/daemon] [--max-jobs 4]`
  - `c4 daemon stop` — graceful shutdown

- **REST API 13개 엔드포인트**:
  - `/health` — 헬스 체크
  - `/jobs/submit` — 작업 제출
  - `/jobs` — 작업 목록
  - `/jobs/{id}` — 작업 상세
  - `/jobs/{id}/logs` — 작업 로그
  - `/jobs/{id}/cancel` — 작업 취소
  - `/jobs/{id}/complete` — 작업 완료
  - `/jobs/{id}/summary` — 작업 요약
  - `/jobs/{id}/estimate` — 소요시간 예측
  - `/jobs/{id}/retry` — 작업 재시도
  - `/stats/queue` — 큐 통계
  - `/gpu/status` — GPU 상태
  - `/daemon/stop` — daemon 중지

- **Hub 호환성**: `hub.Client(apiPrefix="")` → daemon REST API 완전 호환
  - submit_job, get_job, list_jobs, cancel_job 기존 메서드 변경 없음

- **Features**:
  - Job 상태: QUEUED → RUNNING → SUCCEEDED|FAILED|CANCELLED
  - GPU 할당: nvidia-smi 기반 VRAM 자동 할당
  - 소요시간 예측: 4단계 (compile/test/train/eval) 학습 모델
  - 원자적 ID: sync/atomic.Int64 카운터로 밀리초 내 중복 방지
  - Graceful shutdown: SIGTERM 처리, 진행 중 작업 안전 종료

- **테스트**:
  - store_test.go (21개): DB CRUD, transaction, 동시성
  - scheduler_test.go (10개): 프로세스 실행, GPU 할당
  - gpu_test.go (11개): nvidia-smi 파싱, VRAM 계산
  - server_test.go (21개): REST API, 요청 검증
  - estimator_test.go (11개): 명령어 분류, 예측 모델
  - integration_test.go (23개): End-to-end 워크플로우

- **결과**: Go 300 → **400+ 테스트** (+97)

**아키텍처**:
```text
Claude Code → Go MCP Server
              ├─→ Handler: c4_hub_submit → Hub Client
              └─→ Hub Client ──HTTP──→ Daemon Server
                                          ├─→ Scheduler → Process Manager
                                          ├─→ GPU Monitor → nvidia-smi
                                          └─→ SQLite Store
```

### Phase 10.4: Codebase Refactoring + Security Fixes (2026-02-14) ✅

**목표**: 커넥션 재사용, 핸들러 중복 제거, 보안/안정성 픽스

**구현 완료**:
- **Python Store Connection Reuse** (T-S05)
  - documents.py, research/store.py — 단일 `self._conn` 재사용, `close()`/`__enter__`/`__exit__` 구현
  - PRAGMA busy_timeout=5000 + WAL 통일
  - rpc_server.py — DocumentStore/ResearchStore 인스턴스 캐싱
- **daemon store PRAGMA Error Handling** (T-S08)
  - 2개 PRAGMA 실행 + 6개 json.Unmarshal 에러 로깅 추가 (fmt.Fprintf(os.Stderr))
- **Scanner Interface** (T-S09)
  - scanJob/scanJobRow 95% 중복 제거
  - type scanner 인터페이스 + populateJob() 공통 함수
- **hub.go 분할** (T-S10)
  - hub.go 1219→14줄 (struct + 4개 helper 정의만)
  - hub_jobs.go (473), hub_dag.go (362), hub_infra.go (152), hub_edge.go (251) 분할
  - 가독성 + 유지보수성 향상, 자동 테스트 용이
- **신규 테스트 17개** (T-S11)
  - validation_test.go (10): JSON validation, file encoding, request format
  - artifacts_test.go (7): Store CRUD, versioning, metadata
- **보안 & 안정성 픽스 5개**
  - PostgREST 필터값 URL-encode (462d8a4)
  - json.Unmarshal 에러 처리 (773620f) — 7개 핸들러
  - GPU requires_gpu 로직 단순화 (2616aed)
  - 파일 경로 프로젝트 루트 검증 (de7b1ee)
  - daemon GPU indices 워커 전달 (99c29f6)

**결과**: Go 400+ → **620+ 테스트** (+17), Python 492+ → **735** (세션 중 업데이트), 파일 29개 변경 (+1,798/-1,332)

### Phase 10.5: C1 Context Hub (2026-02-14) ✅

**목표**: C1 채널 기반 메시징 + Context Keeper (LLM 요약) + Agent 통합 + Desktop UI

**구현 완료**:

#### Phase 1: 데이터 레이어 — Go MCP + Supabase
- **Supabase Migration 00013**: 4 테이블 (c1_channels, c1_messages, c1_participants, c1_channel_summaries) + RLS + tsvector FTS + participant_id 필드
- **Go C1Handler**: PostgREST HTTP client (setHeaders, httpGet, httpPost, resolveChannelID)
- **MCP 도구 3개**: `c1_search` (FTS), `c1_check_mentions` (agent mentions), `c1_get_briefing` (채널 요약 + 최근 메시지)
- **Helper 메서드**: ListChannels, CreateChannel, PostMessage, GetContext (4개 추가 메서드, MCP 미등록)

#### Phase 2: C1 Desktop 메시징 UI
- **Rust messaging.rs**: 7개 IPC commands (list_channels, create_channel, get_channel_messages, send_message, search_messages, get_briefing, check_mentions)
- **Rust realtime.rs**: c1_messages + c1_channels Supabase Realtime 구독
- **React 컴포넌트**: ChannelsList, MessageThread, ComposeBox, UserTyping, ChannelPicker
- **useChannels 훅**: 채널 목록/메시지/전송/검색 상태 관리

#### Phase 3: Context Keeper
- **c1_keeper.go**: ContextKeeper struct — AutoPost (시스템 메시지 자동 전송), UpdateChannelSummary (LLM haiku 요약)
- **sqlite_store 연동**: notifyKeeper() — SubmitTask/ReportTask 완료 시 #updates 채널 자동 게시
- **SetKeeper()**: Post-construction 주입 패턴 (store가 C1Handler보다 먼저 생성되므로)
- **mcp.go 와이어링**: C1Handler → ContextKeeper → sqliteStore.SetKeeper(keeper) + optional LLM Gateway

#### Phase 4: 문서 관리 + Agent 통합
- **Documents UI**: DocumentsView (탭 사이드바), DocumentEditor (뷰/수정 토글), MarkdownViewer
- **Agent 통합**: mcp.go에서 ContextKeeper 자동 와이어링 (Cloud 활성화 시)

#### Phase 5: 코드 리뷰 & 픽스 (R-CVR-002~008)
- **Migration 00013 확정**: participant_id 필드 추가 (참여자 추적)
- **Count Logic**: channel.json 구조 수정 (메시지 수 정확도)
- **URL 인코딩**: Drive client 필터 (PostgREST and()/not.like())
- **Code Cleanup**: intOr() 미사용 제거, Drive server-side filtering
- **테스트**: Go 22개 신규 (c1_test.go, c1_keeper_test.go)

#### Phase 6: Task Lifecycle → C1 Channels + godotenv
- **notifyKeeper 4-param**: `notifyKeeper(eventType, taskID, title, workerID)` — AssignTask/SubmitTask/ReportTask/MarkBlocked 4곳 연결
- **EnsureChannel**: idempotent resolve-or-create 패턴, NotifyTaskEvent async 포스팅
- **godotenv**: `mcp.go`에서 `.env` 자동 로딩 (monorepo root 지원), `.mcp.json` 하드코딩 키 제거
- **cloud auto-enable**: `config.go`에서 SUPABASE_URL+KEY 발견 시 cloud.Enabled=true 자동 설정
- **C3 EventBus**: gRPC UDS daemon (`internal/eventbus/`) + Python sidecar response piggyback
- **Keeper 테스트**: 11개 (EnsureChannel, NotifyTaskEvent, AutoPost 등)

#### Phase 7: C3 EventBus v1+v2 (2026-02-15)
- **EventBus v1**: `internal/eventbus/` — server/client/store/dispatcher/publisher (gRPC UDS)
- **EventBus v2**: Python sidecar response piggyback — EventCollector (grpcio 의존성 제거)
- **Proxy 통합**: BridgeProxy.SetEventBus + publishSidecarEvents (14개 테스트 추가)
- **C1 wiring**: notifyKeeper 4곳에서 EventBus emit (channel updates, task events)
- **테스트**: Go eventbus 25 + proxy 14 + Python events 9 + piggyback 4 = 52개 신규

**결과**: 112 MCP 도구 (Base 86 + Hub 26), ~1,706 tests (Go 819+ + Python 748+), 13 migrations

---

### Phase 9.3: Cost Dashboard 📋 Future

- C1 앱에 비용 대시보드 뷰 추가
- 예산 알림 및 제한
- 비용 분석 리포트

### Phase 7.3: Hosted Workers

**목표**: 서버에서 Worker 실행

- **Sandbox Execution**
  - gVisor 기반 격리 실행 환경
  - Ephemeral Workspace (세션별 일회성)
- **Auto Scaling**
  - 수요 기반 Worker 자동 확장
  - 리소스 할당 제한 (Quotas)
- **Billing**
  - 사용량 기반 과금
  - Stripe 연동

### 수익 모델

```
┌─────────────────────────────────────────────────────────────┐
│  Tier        │ 가격      │ 기능                             │
├─────────────────────────────────────────────────────────────┤
│  Free        │ $0        │ 자기 API 키, 로컬 Worker, 대시보드│
│  Pro         │ $20/월    │ + 호스팅 Worker, C4 API 크레딧,  │
│              │           │   멀티 LLM 라우팅, 팀 기능       │
│  Enterprise  │ 문의      │ + On-premise, 커스텀 모델, SLA   │
└─────────────────────────────────────────────────────────────┘
```

### 아키텍처

```text
┌─────────────────────────────────────────────────────────────────┐
│                         C4 Cloud                                 │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐     │
│  │ Web Console  │  │ API Gateway  │  │  LLM Gateway       │     │
│  │ (React/Next) │  │ (FastAPI)    │  │  ┌──────────────┐  │     │
│  └──────────────┘  └──────────────┘  │  │ Claude API   │  │     │
│                                       │  │ Gemini API   │  │     │
│                                       │  │ OpenAI API   │  │     │
│                                       │  │ Ollama       │  │     │
│                                       │  └──────────────┘  │     │
│                                       └────────────────────┘     │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   Worker Orchestrator                     │   │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐     │   │
│  │  │ Worker  │  │ Worker  │  │ Worker  │  │ Worker  │     │   │
│  │  │ (gVisor)│  │ (gVisor)│  │ (gVisor)│  │ (gVisor)│     │   │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘     │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌────────────┐  ┌────────────┐  ┌────────────┐                 │
│  │ Supabase   │  │   Redis    │  │  Stripe    │                 │
│  │ (State/DB) │  │  (Locks)   │  │ (Billing)  │                 │
│  └────────────┘  └────────────┘  └────────────┘                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Migration Path

```text
v0.1-0.3        v0.4           v0.5           v0.6          v0.6.10         v0.7.0 (현재)     v0.8+
    │               │               │               │               │               │               │
    │  Multi-Worker │  Agent Routing│  Discovery    │  Team + LSP   │  GPU/ML +     │  Go Primary + │
    │  SQLite       │  + Chaining   │  + Verifier   │  + Observ.    │  Worker Fix   │  Cloud삭제    │
    ▼               ▼               ▼               ▼               ▼               ▼               ▼
┌─────────┐   ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌────────────────┐
│ Local   │──▶│ Agent   │───▶│ EARS +  │───▶│ Supabase│───▶│ PiQ     │───▶│ Go MCP  │───▶│ C4 Cloud       │
│ Files   │   │ Routing │    │ ADR     │    │ + Code  │    │ Absorb  │    │ Primary │    │ (Go 재설계)    │
└─────────┘   └─────────┘    └─────────┘    └─────────┘    └─────────┘    └─────────┘    └────────────────┘
```

---

## 우선순위

| 기능 | 우선순위 | 상태 |
|------|----------|------|
| 단일 사용자 완성 | P0 | ✅ 완료 |
| Multi-Worker | P0 | ✅ 완료 |
| Auto Supervisor | P0 | ✅ 완료 |
| Agent Routing | P0 | ✅ 완료 |
| EARS Requirements | P0 | ✅ 완료 |
| ADR Generator | P0 | ✅ 완료 |
| Verification System | P0 | ✅ 완료 |
| Skill System Enhancement | P0 | ✅ 완료 |
| 문서화 | P0 | ✅ 완료 |
| Team Collaboration | P0 | ✅ 완료 |
| Branding Middleware | P0 | ✅ 완료 |
| Code Analysis Engine | P0 | ✅ 완료 |
| Documentation API | P0 | ✅ 완료 |
| Gap Analyzer | P0 | ✅ 완료 |
| Semantic Search Engine | P0 | ✅ 완료 |
| Call Graph Analyzer | P0 | ✅ 완료 |
| Long-Running Worker Detection | P0 | ✅ 완료 |
| LSP Server | P0 | ✅ 완료 |
| **Reliability & Observability** | P0 | ✅ 완료 |
| **PiQ 완전 흡수 (GPU/ML)** | P0 | ✅ 완료 |
| **Worker Lifecycle Hardening** | P0 | ✅ 완료 |
| **C1 Multi-LLM Explorer** | P0 | ✅ 완료 |
| **Go Core Foundation** | P0 | ✅ 완료 |
| **Go MCP Primary + Cloud Cleanup** | P0 | ✅ 완료 |
| **Go Gap Fix + Persona Evolution** | P0 | ✅ 완료 |
| **Sidecar Stability + LSP Onboarding** | P0 | ✅ 완료 |
| **Review-as-Task + Soul System** | P0 | ✅ 완료 |
| **Cloud Foundation (8.1)** | P0 | ✅ 완료 |
| **C1 Cloud Dashboard (8.2)** | P0 | ✅ 완료 |
| **Knowledge Store Cloud (8.3)** | P0 | ✅ 완료 |
| **Knowledge Bidirectional Sync (8.4)** | P0 | ✅ 완료 |
| **LLM Gateway Framework (9.1)** | P1 | ✅ 완료 |
| **LLM Provider Implementations (9.2)** | P1 | ✅ 완료 |
| **CDP Runner (10)** | P1 | ✅ 완료 |
| **SQLite Hardening (10.1)** | P0 | ✅ 완료 |
| **c4 daemon (10.3)** | P0 | ✅ 완료 |
| **Codebase Refactoring + Security Fixes (10.4)** | P0 | ✅ 완료 |
| **C1 Context Hub (10.5)** | P0 | ✅ 완료 |
| **Security Fixes + Sidecar Improvements** | P0 | ✅ 완료 |
| LLM Cost Dashboard (9.3) | P2 | 📋 Future |
| Worker Loop (CLI `c4 run`) | P2 | 📋 Deferred |
| Hosted Workers | P2 | 📋 Future |

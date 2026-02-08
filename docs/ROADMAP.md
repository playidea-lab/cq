# C4 Roadmap

## Current Version: v0.7.0 (Go MCP Primary + Cloud Cleanup)

현재 버전은 **Go MCP Primary 아키텍처, Python Cloud 코드 정리(-73K LOC), JSON-RPC Bridge, Multi-LLM 탐색기**를 포함합니다.

### 지원 기능

- **Go MCP Server (Primary)** - 47 도구, Registry-based, Python sidecar 자동 관리
- **LSP Server** (VS Code 등 에디터 통합) - pygls 기반
- State Machine (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ↔ CHECKPOINT → COMPLETE)
- Multi-Worker (SQLite WAL 모드, race-condition free)
- Agent Routing (Phase 4) - 도메인별 에이전트 자동 선택 및 체이닝
- **EARS Requirements** - 5가지 패턴 기반 요구사항 수집
- **ADR (Architecture Decision Records)** - 설계 결정 관리
- **Verification System** - 6가지 Verifier (HTTP, CLI, Browser, Visual, Metrics, Dryrun)
- Validation Runner (lint, unit tests)
- Checkpoint System (APPROVE, REQUEST_CHANGES, REPLAN, REDESIGN)
- Slash Commands (11개)
- Stop Hook (자동 실행 유지)
- Auto Supervisor Loop
- **Team Collaboration** - Supabase 기반 팀 상태 공유
- **Branding Middleware** - 화이트라벨 커스텀 도메인 지원
- **Code Analysis Engine** - Python/TypeScript AST 분석
- **Documentation Snapshots** - Context7 스타일 문서 API
- **Gap Analyzer** - EARS 요구사항 ↔ 구현 매핑
- **GPU/ML Native** - GPU 감지, 스케줄링, DAG→Task 변환
- **Experiment Tracker** - @c4_track 데코레이터, 메트릭 자동 캡처
- **Artifact Store** - Content-addressable 로컬 저장소
- **Knowledge Store v2** - Obsidian Markdown SSOT + FTS5 + Vector hybrid search
- **Hook Registry** - 태스크 생명주기 훅 시스템
- **Worker Lifecycle** - 좀비 워커 자동 감지/정리, TTL 기반 제거
- **Go MCP Primary** (c4-core/) - 47 도구, Registry-based, SQLite Store, Python sidecar 자동 관리
- **JSON-RPC Bridge** - Go ↔ Python 통신 (LSP, Knowledge, GPU 위임)
- **C1 Multi-LLM Explorer** - Claude Code, Codex CLI, Cursor, Gemini CLI 4개 프로바이더 통합
- **Python Cloud 삭제** - ~39K LOC 삭제 (API, Billing, SSO, Monitoring, Realtime 등)

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
- **Python Bridge Server** (`c4/bridge/grpc_server.py`): JSON-RPC over TCP
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

## Phase 7: C4 Cloud (v0.8.0) 📋 Next

**목표**: LLM 오케스트레이션 플랫폼으로서의 완전 관리형 SaaS
**참고**: Phase 6.13에서 Python Cloud 코드(-73K LOC) 삭제됨. Cloud 재구축 시 Go 기반으로 설계.

### Phase 7.1: Cloud Foundation

**목표**: 웹 기반 프로젝트 관리 및 모니터링

- **Web Console**
  - 실시간 워크플로우 대시보드
  - 프로젝트/팀 관리 UI
  - 태스크 상태 시각화
- **Remote State Sync**
  - 로컬 Worker + Supabase 상태 동기화 (이미 구현됨)
  - 웹에서 실시간 모니터링
- **Authentication**
  - Supabase Auth 연동
  - GitHub OAuth

### Phase 7.2: LLM Gateway ⭐ 핵심 차별화

**목표**: 멀티 LLM 오케스트레이션 플랫폼

- **Multi-LLM Support**
  - Claude, Gemini, GPT, Ollama 연결
  - 사용자 API 키 관리 (Vault)
  - C4 호스팅 API 옵션 (마진 모델)
- **Smart Routing**
  - 태스크별 최적 모델 자동 선택 (Economic Mode 확장)
  - 모델 간 협업 (Claude 계획 → Gemini 실행 → Claude 리뷰)
  - 비용/성능 트레이드오프 설정
- **Cost Dashboard**
  - 모델별 사용량 추적
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
| Cloud Foundation (7.1) | P1 | 📋 Next (Go 재설계) |
| LLM Gateway (7.2) | P1 | 📋 Phase 7 |
| Hosted Workers (7.3) | P2 | 📋 Phase 7 |

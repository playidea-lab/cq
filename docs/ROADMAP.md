# C4 Roadmap

## Current Version: v0.6.0 (Team Collaboration)

현재 버전은 **팀 협업, 화이트라벨 브랜딩, 코드 분석 엔진**을 지원합니다.

### 지원 기능

- MCP Server (Claude Code 통합) - 25+ 도구
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

**목표**: 코드 분석 및 문서화 자동화

**구현 완료**:
- **Code Analysis Engine** (`c4/services/code_analysis/`)
  - `PythonParser` - Python AST 분석
  - `TypeScriptParser` - TypeScript 구문 분석
  - 심볼 테이블, 의존성 그래프
- **Semantic Search Engine** (`c4/docs/semantic_search.py`)
  - TF-IDF 기반 자연어 코드 검색
  - 프로그래밍 동의어 확장 (auth → authentication 등)
  - 범위 지정 검색 (symbols, docs, code, files)
- **Call Graph Analyzer** (`c4/docs/call_graph.py`)
  - 호출자/피호출자 분석
  - 함수 간 호출 경로 찾기
  - 호출 그래프 통계 및 Mermaid 다이어그램 생성
- **Long-Running Worker Detection** (`c4/daemon/workers.py`)
  - Worker heartbeat 모니터링
  - 장기 실행 태스크 자동 감지
  - Stale worker 복구 메커니즘
- **Documentation Server** (`c4/mcp/docs_server.py`)
  - `query_docs` - 문서 쿼리
  - `create_snapshot` - 스냅샷 생성
  - `get_usage_examples` - 사용 예시 추출
- **Gap Analyzer** (`c4/mcp/gap_analyzer.py`)
  - `analyze_spec_gaps` - 명세-구현 갭 분석
  - `generate_tests_from_spec` - 명세→테스트 생성
  - `link_impl_to_spec` - 구현-명세 연결
- **12개 신규 MCP 도구** (`c4/mcp/code_tools.py`)
  - `c4_semantic_search`, `c4_find_related_symbols`, `c4_search_by_type`
  - `c4_get_callers`, `c4_get_callees`, `c4_find_call_paths`
  - `c4_call_graph_stats`, `c4_call_graph_diagram`
  - `c4_find_definition`, `c4_find_references`, `c4_analyze_file`, `c4_get_dependencies`
- **Public Docs API** (`c4/api/routes/docs.py`)
  - Context7 스타일 REST API
  - 스냅샷 생성/조회
  - 검색 기능

---

## Phase 7: C4 Cloud (장기 계획)

**목표**: 완전 관리형 SaaS 버전

### 주요 기능

- Web Dashboard
- 원격 Worker Pool
- GitHub 통합 (Auto PR)
- 사용량 기반 과금
- 팀/조직 관리

### 아키텍처

```text
┌─────────────────────────────────────────────────────────────┐
│                      C4 Cloud                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Web Console │  │ API Gateway │  │ Worker Orchestrator │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                          │                                   │
│              ┌───────────┴───────────┐                      │
│              ▼                       ▼                      │
│       ┌────────────┐          ┌────────────┐               │
│       │ PostgreSQL │          │   Redis    │               │
│       │  (State)   │          │  (Locks)   │               │
│       └────────────┘          └────────────┘               │
└─────────────────────────────────────────────────────────────┘
```

---

## Migration Path

```text
v0.1-0.3        v0.4           v0.5           v0.6 (현재)    v0.7+
    │               │               │               │               │
    │  Multi-Worker │  Agent Routing│  Discovery    │  Team Collab  │
    │  SQLite       │  + Chaining   │  + Verifier   │  + MCP Tools  │
    ▼               ▼               ▼               ▼               ▼
┌─────────┐   ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│ Local   │──▶│ Agent   │───▶│ EARS +  │───▶│ Supabase│───▶│ Cloud   │
│ Files   │   │ Routing │    │ ADR     │    │ + Code  │    │ SaaS    │
└─────────┘   └─────────┘    └─────────┘    └─────────┘    └─────────┘
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
| Cloud API | P1 | 📋 Phase 7 |

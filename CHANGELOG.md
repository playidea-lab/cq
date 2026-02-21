# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.8.0] - 2026-02-21

### ✨ Features

- **멀티 세션 격리 개선 3종** (T-ISO-001~003)
  - **State Machine BEGIN IMMEDIATE**: `Transition()` 에 `db.Conn` + `BEGIN IMMEDIATE` 트랜잭션 적용
    - `ErrStateChanged` / `ErrInvalidTransition` / `ErrDatabase` 에러 3종 분류
    - 동시성 테스트 3개 추가 (`-race` 통과): `TestTransitionBeginImmediate`, `TestTransitionConcurrentStateChange`, `TestTransitionConcurrentRecovery`
  - **Advisory Phase Lock** (`c4_phase_lock_acquire` / `c4_phase_lock_release`, 신규 MCP 도구 2개)
    - `.c4/phase_locks/{phase}.lock` JSON 파일 기반, `polish` / `finish` 단계 보호
    - Stale 판정 5개 시나리오 (PID 생존/사망/EPERM + cross-host 2h 임계)
    - `/c4-polish` Phase 0 · `/c4-finish` Step 1에 lock check 통합
  - **Makefile atomic install**: `install-*` 타겟 `cp` → `cp .tmp && mv || rm .tmp` 원자적 교체

- **`cq serve` 컴포넌트 연동 완성**
  - EventBus / GPU Scheduler / EventSink / HubPoller config 로드 + 자동 등록
  - HubPoller: `NoopPublisher` fallback으로 nil publisher panic 제거

- **`c4-plan` Phase 4.5 Worker 기반 Plan Critique Loop**
  - 인라인 자가 비판 → 매 라운드 새 Worker(Task agent) 스폰 (confirmation bias 제거)
  - 수렴 조건: CRITICAL == 0 AND HIGH == 0 (최대 3라운드)

- **`c4-run` R-task 리뷰 Worker 자동 스폰**
  - 준비된 R- 태스크는 `review` 모델(opus)로 별도 Worker 할당

- **Hook Config SSOT 전환** (`.c4/hook-config.json`)
  - `permission_reviewer` 설정을 MCP 서버 시작 시 자동 내보내기
  - `hook-config.json` 우선, `~/.claude/hooks/*.conf` fallback
  - `AutoApprove` / `AllowPatterns` / `BlockPatterns` 필드 추가

### 🐛 Bug Fixes

- **`serve/realtime.go`**: 클린 disconnect 시 backoff가 계속 증가하는 버그 수정
  - backoff 증가를 error 분기 내로 이동 → 클린 재연결은 항상 1s로 리셋
- **`serve.go`**: `config.New()` 실패 시 `cfgMgr` nil 역참조 패닉 방지 (nil guard 추가)
- **`handlers/phase_lock.go`**: phase 파라미터 서버 측 allowlist 검증 누락 수정
- **`state/phase_lock.go`**: `lockFile()` path traversal 취약점 — `validPhase()` allowlist 강제
- **`fix(hook)`**: `mapfile` → `while-read` 교체 (bash 3.2 호환)
- **`fix(review)`**: R-task auto-cascade 제거 — 실제 리뷰 Worker가 처리하도록

### ♻️ Refactoring

- `cq init`: `.conf` embed/생성 제거, `hook-config.json` 패턴으로 통일

### 📚 Documentation

- MCP 도구 수: 154 → 156 (`c4_phase_lock_acquire/release` +2)
- Go 테스트 수: ~1,423 → ~1,430, 합계: ~2,205 → ~2,212

---

## [0.7.0] - 2026-02-21

### ✨ Features

- **`cq serve` 데몬 명령어**: ComponentManager 기반 상시 실행 서버
  - PID 락 파일, `/health` 엔드포인트, 컴포넌트 라이프사이클 관리
  - EventBus, EventSink, HubPoller, GPU Scheduler, Agent 컴포넌트 wrapping
  - Agent 컴포넌트: Supabase Realtime WebSocket → `@cq` mention 감지 → `claude -p` 디스패치
  - `cq serve` 실행 중이면 데몬 내장 컴포넌트 자동 skip (중복 방지)

- **Tiered Build System** (`solo` / `connected` / `full` / `nightly`)
  - Build tag 기반 컴포넌트 선택적 포함 (stub 파일 패턴)
  - `cq init --tier solo|connected|full` → `.c4/config.yaml` tier 저장
  - Makefile: `build-solo/connected/full/nightly`, `install-*` 타겟
  - C7 Observe / C6 Guard / C8 Gate 조건부 빌드 (nightly 태그)

- **C7 Observe**: Logger(slog) + Metrics + Registry Middleware 자동 계측
- **C6 Guard**: RBAC + Audit + Policy + Middleware (`c6_guard` 빌드 태그)
- **C8 Gate**: Webhook + Scheduler + Connector (`c8_gate` 빌드 태그)

- **Registry Middleware 체인**: `Registry.Use()` / `UseContextual()` 지원
  - `ToolNameFromContext()` 로 도구 이름 접근 가능

- **C5 Hub Artifact Pipeline**:
  - Signed Upload/Download (Supabase Storage), Worker input/output artifact 흐름
  - `LocalBackend` HTTP 핸들러 (`/v1/storage/upload/{path}`)
  - Long-poll Lease Acquire (20s server-side)

- **LLM Gateway 캐싱 개선**:
  - Anthropic Prompt Caching (ephemeral cache_control 블록)
  - `cache_hit_rate` + `cache_savings_rate` 노출 (`c4_llm_costs`)
  - `cache_by_default` 설정 지원

- **`cq doctor` 자가진단**: 8개 항목 (binary, .c4, .mcp.json, CLAUDE.md, hooks, Python, C5, Supabase)
- **c4-bash-security hook UX 개선**: 차단 메시지 가독성 향상

### 🐛 Bug Fixes

- `serve/agent.go`: `a.Status()` → `a.status` (미존재 메서드 수정)
- `serve/agent_test.go`: 중복 `mockComponent` 선언 제거, 잘못된 API 테스트 제거
- `c5`: LocalBackend HTTP 핸들러 디렉토리 리스팅 차단, partial file 정리
- LLM body limit `io.LimitReader(resp.Body, 1<<22)` 적용 (4 MiB)
- `guard/gate` SQLite + scheduler 마이너 수정

### 📚 Documentation

- `CLAUDE.md` / `AGENTS.md`: Tiered Build System, `cq serve` 섹션, C7/C6/C8 문서화
- `README.md`: CQ 브랜딩 통일, `cq doctor` 섹션, 154 tools / 2,205 tests / 141.7K LOC 수치 현행화

---

## [0.6.0] - 2026-01-26

### Added

- **DDD-CLEANCODE Worker Packet (Phase 6)**: 구조화된 태스크 명세
  - `Goal`: 완료 조건 + 범위 외 명시
  - `ContractSpec`: API 계약 (input/output/errors) + 테스트 요구사항 (success/failure/boundary)
  - `BoundaryMap`: DDD 레이어 제약 (target_domain, allowed/forbidden_imports)
  - `CodePlacement`: 파일 위치 명세 (create/modify/tests)
  - `QualityGate`: 검증 명령어 (name/command/required)
  - `CheckpointDefinition`: CP1/CP2/CP3 마일스톤 정의
  - `c4_add_todo` MCP 도구에 DDD-CLEANCODE 필드 지원 추가
  - `dod` 필드 deprecated 경고 (goal 미사용 시)
  - 12개 통합 테스트로 저장/로드 검증 완료

- **c4-plan/c4-submit 스킬 강화**
  - Worker Packet 구조화된 입력 UI (4.1~4.5절)
  - 경계 검증 자동 실행 (forbidden imports)
  - 작업 분해 검증 (max 2일, max 3 APIs)
  - ContractSpec 검증 (테스트 커버리지)

- **Semantic Search Engine (Phase 6.5)**: TF-IDF 기반 자연어 코드 검색
  - `SemanticSearcher`: 자연어 쿼리로 코드 검색
  - 프로그래밍 동의어 확장 (auth → authentication, db → database 등)
  - 범위 지정 검색 (symbols, docs, code, files)
  - 관련 심볼 찾기 및 타입별 검색

- **Call Graph Analyzer (Phase 6.5)**: 함수 호출 관계 분석
  - `CallGraphAnalyzer`: 호출자/피호출자 분석
  - 함수 간 호출 경로 찾기
  - 호출 그래프 통계 (핫스팟, 진입점, 고립 함수)
  - Mermaid 다이어그램 생성

- **Enhanced MCP Tools (Phase 6.5)**: 12개 새 코드 분석 도구
  - `c4_semantic_search`: 자연어 코드 검색
  - `c4_find_related_symbols`: 관련 심볼 찾기
  - `c4_search_by_type`: 타입별 심볼 검색
  - `c4_get_callers`: 호출자 찾기
  - `c4_get_callees`: 피호출자 찾기
  - `c4_find_call_paths`: 호출 경로 찾기
  - `c4_call_graph_stats`: 호출 그래프 통계
  - `c4_call_graph_diagram`: Mermaid 다이어그램
  - `c4_find_definition`: 심볼 정의 찾기
  - `c4_find_references`: 참조 찾기
  - `c4_analyze_file`: 파일 심볼 분석
  - `c4_get_dependencies`: 의존성 분석

- **Long-Running Worker Detection (Phase 6.5)**: Heartbeat 기반 이상 탐지
  - Worker heartbeat 모니터링
  - 장기 실행 태스크 자동 감지
  - Stale worker 복구 메커니즘

- **Team Collaboration (Phase 6)**: Supabase 기반 팀 협업 지원
  - `SupabaseStateStore`: 분산 프로젝트 상태 관리
  - `SupabaseLockStore`: 분산 잠금 (RLS 적용)
  - `TeamService`: 팀 생성/수정/삭제, 멤버 RBAC
  - `CloudSupervisor`: 팀 전체 리뷰 및 체크포인트 관리
  - `TaskDispatcher`: 우선순위 기반 태스크 분배
  - 6개 Supabase 마이그레이션 (`00001` ~ `00006`)

- **Branding Middleware**: 화이트라벨 커스텀 도메인 지원
  - `BrandingMiddleware`: Host 헤더 기반 브랜딩 적용
  - `BrandingCache`: TTL 캐시 (기본 60초)
  - 팀별 로고, 색상, 도메인 설정

- **Code Analysis Engine**: Python/TypeScript 코드 분석
  - `PythonParser`: Python AST 분석
  - `TypeScriptParser`: TypeScript 구문 분석
  - 심볼 테이블, 의존성 그래프, 호출 관계

- **Documentation Server (MCP)**: 문서화 자동화
  - `query_docs`: 문서 검색/쿼리
  - `create_snapshot`: 코드베이스 스냅샷
  - `get_usage_examples`: 사용 예시 추출
  - Context7 스타일 REST API (`/api/docs`)

- **Gap Analyzer (MCP)**: 명세-구현 매핑
  - `analyze_spec_gaps`: EARS 요구사항 갭 분석
  - `generate_tests_from_spec`: 명세→테스트 생성
  - `link_impl_to_spec`: 구현-명세 연결
  - `verify_spec_completion`: 완료 검증

- **GitHub Integration 강화**
  - `GitHubClient`: 팀 권한 동기화
  - `GitHubAutomation`: 자동 PR/Issue 생성
  - 웹훅 이벤트 처리

- **Review-as-Task**: 리뷰가 태스크로 관리됩니다
  - 태스크 ID에 버전 번호 추가 (T-XXX → T-XXX-0)
  - 구현 태스크 완료 시 자동으로 리뷰 태스크(R-XXX-N) 생성
  - REQUEST_CHANGES 시 다음 버전 태스크 자동 생성 (T-XXX-1)
  - `max_revision` 설정으로 최대 수정 횟수 제한 (기본값: 3)
  - **리뷰 태스크 자동 라우팅**: `task_type="review"` 설정으로 `code-reviewer` 에이전트 자동 할당

- **Checkpoint-as-Task**: 체크포인트가 태스크로 처리됩니다
  - Phase의 모든 리뷰가 APPROVE되면 CP-XXX 태스크 자동 생성
  - Worker가 체크포인트 검증 수행 (E2E, HTTP 등)
  - APPROVE 시 Phase 완료 및 main 머지
  - REQUEST_CHANGES 시 문제 태스크의 다음 버전 생성
  - `checkpoint_as_task: true` 설정으로 활성화

- **TaskType Enum**: 태스크 유형 구분
  - `IMPLEMENTATION`: 구현 태스크 (T-XXX-N)
  - `REVIEW`: 리뷰 태스크 (R-XXX-N)
  - `CHECKPOINT`: 체크포인트 태스크 (CP-XXX)

- **Task Model 확장**
  - `base_id`: 기본 태스크 ID ("001")
  - `version`: 버전 번호 (0, 1, 2...)
  - `type`: TaskType enum
  - `task_type`: 스킬 매칭용 태스크 유형 ("review", "debug", "security" 등)
  - `phase_id`: Phase 식별자
  - `required_tasks`: CP가 검증할 태스크 목록
  - `review_decision`: 리뷰 결정 (APPROVE/REQUEST_CHANGES)

- **GraphRouter.use_legacy_fallback** 속성 추가
  - skill matcher와 rule engine 미설정 시 도메인 기반 라우팅만 사용

- **GitLab Integration**: GitLab MR 웹훅 및 AI 코드 리뷰
  - `GitLabClient`: REST API 클라이언트 (diff 조회, 노트/토론 생성, 라벨)
  - `GitLabProvider`: 통합 프로바이더 (OAuth, 웹훅 검증)
  - `MRReviewService`: AI 기반 코드 리뷰 서비스 (LiteLLM/Anthropic)
  - `/webhooks/gitlab` 엔드포인트 추가
  - X-Gitlab-Token 헤더 기반 웹훅 검증
  - 환경 변수: `GITLAB_PRIVATE_TOKEN`, `GITLAB_WEBHOOK_SECRET`, `GITLAB_URL`

### Changed

- MCP 도구 25개 이상으로 확장
- `c4_add_todo`가 정규화된 태스크 ID 반환 (T-XXX → T-XXX-0)
- SupervisorLoop이 `checkpoint_as_task` 모드에서 큐 항목만 정리 (직접 처리 안함)
- 체크포인트 검증이 Design 단계 요구사항 기반으로 DoD 자동 생성

### Fixed

- `test_add_todo` 관련 10개 테스트 수정 (정규화된 ID 사용)
- GraphRouter 누락 속성 `use_legacy_fallback` 추가

## [0.1.0] - 2026-01-15

### Added

- 초기 릴리스
- State Machine 워크플로우 (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- MCP Server 통합 (19개 도구)
- Multi-Worker SQLite WAL 기반 병렬 실행
- Agent Routing (도메인별 에이전트 자동 선택)
- EARS 요구사항 수집 (5가지 패턴)
- Multi-LLM Provider 지원 (LiteLLM 기반 100+)
- Checkpoint Gates (단계별 리뷰 포인트)
- Auto-Validation (자동 lint/test 실행)
- 다중 플랫폼 지원 (Claude Code, Cursor, Codex CLI, Gemini CLI, OpenCode)

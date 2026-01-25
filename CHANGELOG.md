# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-01-25

### Added

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

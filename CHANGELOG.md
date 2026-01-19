# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Review-as-Task**: 리뷰가 태스크로 관리됩니다
  - 태스크 ID에 버전 번호 추가 (T-XXX → T-XXX-0)
  - 구현 태스크 완료 시 자동으로 리뷰 태스크(R-XXX-N) 생성
  - REQUEST_CHANGES 시 다음 버전 태스크 자동 생성 (T-XXX-1)
  - `max_revision` 설정으로 최대 수정 횟수 제한 (기본값: 3)

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
  - `phase_id`: Phase 식별자
  - `required_tasks`: CP가 검증할 태스크 목록
  - `review_decision`: 리뷰 결정 (APPROVE/REQUEST_CHANGES)

- **GraphRouter.use_legacy_fallback** 속성 추가
  - skill matcher와 rule engine 미설정 시 도메인 기반 라우팅만 사용

### Changed

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

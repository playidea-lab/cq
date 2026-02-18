# C4 System Health Fix - Dogfooding 결과 대응

> 참조: 시스템 점검 (2026-02-18), AGENTS.md

## 완료

- [x] T-SC-001: c4-submit 스킬에 handoff JSON 구조 문서 추가 (HIGH)
  - `.claude/skills/c4-submit/SKILL.md` - Section 5.1 Handoff 구조
- [x] T-SC-002: c4-review 스킬 내부 'c2-review' 잘못된 제목 수정 (HIGH)
  - `.claude/skills/c4-review/SKILL.md` - Line 12 제목 수정
- [x] T-SC-003: c4-add-task, c4-quick DoD에 Rationale 필드 추가 (MEDIUM)
  - `.claude/skills/c4-add-task/SKILL.md` - DoD 원칙 4번 항목
  - `.claude/skills/c4-quick/SKILL.md` - DoD 템플릿 Rationale 행
- [x] T-SC-004: c4-swarm handoff를 JSON 구조로 통일 (MEDIUM)
  - `.claude/skills/c4-swarm/SKILL.md` - MEMBER_PROMPT + Handoff Rules
- [x] T-SC-005: c4-refine에 Related Skills 교차참조 추가 (LOW)
  - `.claude/skills/c4-refine/SKILL.md` - Related Skills 테이블
- [x] T-EB-001: EventBus default_rules에 review/knowledge 규칙 추가 (MEDIUM)
  - `c4-core/internal/eventbus/default_rules.yaml` - 2개 규칙 추가
- [x] T-DB-001: Supabase c4_tasks 스키마에 execution_mode 컬럼 추가 (HIGH)
  - `infra/supabase/migrations/00020_c4_tasks_execution_mode.sql`

## 완료 (Phase 2)

- [x] M-3: store_review + store_status 테스트 추가 (16개 테스트 케이스)
  - `c4-core/internal/mcp/handlers/store_review_test.go` (7 tests)
  - `c4-core/internal/mcp/handlers/store_status_test.go` (9 tests)
- [x] M-4: Config GetModelForTask에 RF- (refine) 라우팅 추가 + 필드 주석 갱신
  - `c4-core/internal/config/config.go` - GetModelForTask + ModelRouting 주석
  - `c4-core/internal/config/config_test.go` - RF- 테스트 케이스
- [x] L-1: 주석 처리된 코드 - 조사 결과 문서용 주석만 존재 (문제 없음)
- [x] L-3: `_ = err` 패턴 수정 (bridge/sidecar_test.go:330)

## 남은 항목 (별도 계획 필요)

- [ ] Hub 핸들러 테스트 (hub_jobs, hub_infra, hub_edge, hub_dag - 30 functions)
- [ ] c2_native, persona, native_lsp 핸들러 테스트

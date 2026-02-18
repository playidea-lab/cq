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

## 보류 (향후 별도 태스크)

- [ ] M-3: 핸들러 테스트 커버리지 (knowledge, c2, research 등 7개)
  - 참조: `c4-core/internal/mcp/handlers/` 테스트 파일
- [ ] M-4: Config 미사용 model routing 필드 정리
  - 참조: `c4-core/internal/config/`
- [ ] L-1: 주석 처리된 코드 정리 (c4-core 내 commented-out 블록)
- [ ] L-3: 테스트 내 `_ = err` 패턴 수정

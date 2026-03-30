feature: tasks-supabase-ssot
domain: infra
description: connected tier에서 Tasks SSOT를 Supabase로 전환 (cloud-primary 모드)

requirements:
  - type: event-driven
    text: "When Supabase c4_tasks 테이블에 누락 컬럼(review_decision_evidence, failure_signature, blocked_attempts, last_error, files_changed, session_id, superseded_by)이 추가되면, the system shall CloudStore가 이 컬럼들을 읽고 쓸 수 있다"
  - type: event-driven
    text: "When connected tier 사용자가 cq auth로 인증하면, the system shall cloud.mode를 cloud-primary로 자동 설정한다"
  - type: state-driven
    text: "While cloud-primary 모드이면, the system shall 쓰기를 Supabase에 먼저 수행하고 SQLite는 캐시로만 사용한다"
  - type: unwanted
    text: "If Supabase가 응답하지 않으면, the system shall SQLite 캐시로 fallback하여 graceful degradation한다"
  - type: state-driven
    text: "While solo tier이면, the system shall 기존 local-first 동작을 유지한다"

non_functional:
  - 기존 store.Store 인터페이스 변경 없음
  - CloudStore 코드만 수정 (핸들러 변경 없음)

out_of_scope:
  - Persona/Twin/Lighthouse 핸들러의 CloudStore 지원 (별도 Phase)
  - Knowledge/Ontology 클라우드 이동 (Phase 3-4)
feature: Refine Gate Enforcement — plan 단계 critique loop 강제
domain: web-backend
requirements:
  ubiquitous:
    - c4_add_todo는 batch commit(4개 이상 태스크) 시 c4_gates에 refine=done 기록이 있는지 확인해야 한다
    - refine gate 미통과 시 add_todo를 거부하고 사유를 반환해야 한다
    - 태스크 3개 이하(소규모 계획)면 자동 pass해야 한다
    - 기존 c4_record_gate 도구를 재사용해야 한다 (gate 이름만 "refine")
  event_driven:
    - WHEN plan Phase 4 draft 완료 시 THEN critique worker를 스폰하고 refine loop를 실행한다
    - WHEN refine 수렴 시 THEN c4_record_gate(gate="refine", status="done") 기록한다
  unwanted:
    - 3개 이하 태스크 추가에 refine 강제 (false block)
    - c4_add_todo 개별 호출에 gate 강제 (batch commit 시에만)
    - Phase 4.5 critique-loop.md 스킬 스펙 변경
out_of_scope:
    - critique loop Go 구현 (스킬 마크다운 유지)
    - refine threshold 자동 조정
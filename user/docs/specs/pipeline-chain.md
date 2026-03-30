feature: pipeline-chain
domain: skill-system
description: 스킬 간 자동 체인 메커니즘 — plan → run → finish 논스탑 연결

requirements:
  - type: event-driven
    text: "When /pi가 '자동 구현'을 선택하면, the system shall .c4/pipeline-state.json을 생성하고 steps=[plan,run,finish], auto=true로 초기화한다"
  - type: event-driven
    text: "When 스킬이 자신의 완료 조건을 충족하면, the system shall pipeline-state.json의 current를 다음 단계로 전진시키고 해당 스킬을 Skill()로 호출한다"
  - type: event-driven
    text: "When 스킬 실행 중 실패가 발생하면, the system shall 에이전트가 자체 수정 루프를 실행한다 (파이프라인 미개입)"
  - type: event-driven
    text: "When 에이전트가 해결 불가능한 상황을 감지하면, the system shall 파이프라인을 일시 중지하고 사용자에게 알린다"
  - type: state-driven
    text: "While auto=true인 동안, the system shall 스킬 간 전환 시 사용자 확인을 요청하지 않는다"

non_functional:
  - Go 코드 변경 없음 — 스킬 MD + state 파일만으로 해결
  - 기존 스킬의 비-pipeline 동작에 영향 없음

out_of_scope:
  - Go 레벨 오케스트레이터
  - 커스텀 step 순서
  - 병렬 파이프라인
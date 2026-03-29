feature: learn-loop-wiring
domain: go-backend
description: 리뷰 승인/거절 피드백을 페르소나에 자동 학습시키고, 다음 태스크 할당 시 scope 경고를 DoD에 주입하여 learn→plan 루프를 닫는다.

requirements:
  - type: event-driven
    text: "When 리뷰 워커가 c4_submit으로 태스크를 완료하면, the system shall persona_learn_from_diff를 비동기 호출하여 성공 패턴을 학습한다"
  - type: event-driven
    text: "When 리뷰 워커가 c4_request_changes로 태스크를 거절하면, the system shall persona_learn을 호출하여 거절 사유를 scope에 바인딩 저장한다"
  - type: event-driven
    text: "When c4_get_task로 태스크를 할당할 때, the system shall 해당 scope의 과거 거절 경고를 knowledge_context에 자동 주입한다"
  - type: state-driven
    text: "While 학습된 패턴이 없으면, the system shall 기존 keyword 기반 knowledge_context만 제공한다"
  - type: optional
    text: "If 같은 거절 사유가 3회 이상 반복되면, the system shall 해당 패턴을 validation rule 승격 후보로 로깅한다"

non_functional:
  - submit 레이턴시 증가 0ms (비동기 goroutine)
  - scope 경고 주입은 최근 3건 제한 (과잉 경고 방지)

out_of_scope:
  - 페르소나 store 스키마 변경
  - Stage 2 패턴 승격 자동화
  - Stage 3 임베딩 기반 유사도
  - L3 집단 온톨로지 공유
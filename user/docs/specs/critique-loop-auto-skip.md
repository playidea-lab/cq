feature: critique-loop-auto-skip
domain: skill-system
description: 소규모 태스크(3개 이하)에서 critique loop 자동 skip

requirements:
  - type: event-driven
    text: "When draft 태스크가 3개 이하이면, the system shall Phase 4.5 critique loop을 skip한다"
  - type: state-driven
    text: "While planning.critique_loop.mode=skip이면, the system shall 태스크 수와 무관하게 skip한다"

out_of_scope:
  - critique loop 자체 로직 변경
  - config 키 추가
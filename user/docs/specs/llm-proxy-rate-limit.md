feature: llm-proxy-rate-limit
domain: infra
description: LLM Proxy Edge Function에 freemium 사용량 카운터 추가

requirements:
  - type: event-driven
    text: "When 인증된 사용자가 LLM proxy를 호출하면, the system shall 월간 사용량을 확인하고 100회 이하면 허용한다"
  - type: event-driven
    text: "When 월간 사용량이 100회를 초과하면, the system shall 429 응답과 limit reached 메시지를 반환한다"
  - type: event-driven
    text: "When Anthropic API 호출이 성공하면, the system shall 사용량 카운터를 1 증가시킨다"
  - type: event-driven
    text: "When 새 달이 시작되면, the system shall 카운터가 자동 리셋된다 (month 컬럼 기반)"

out_of_scope:
  - Redis 기반 카운터
  - Pro tier 무제한
  - 토큰 기반 과금
feature: llm-proxy-edge-function
domain: infra
description: Supabase Edge Function으로 LLM Proxy 구현. 사용자 API 키 불필요.

requirements:
  - type: event-driven
    text: "When connected tier 사용자가 LLM 호출(c4_llm_call, permission reviewer 등)을 하면, the system shall Supabase Edge Function을 통해 PI Lab API 키로 호출한다"
  - type: event-driven
    text: "When Edge Function이 요청을 받으면, the system shall JWT 토큰을 검증하고 미인증 요청을 거부한다"
  - type: event-driven
    text: "When Edge Function이 Anthropic API를 호출하면, the system shall 응답을 그대로 relay한다"
  - type: unwanted
    text: "If Edge Function이 실패하면(timeout, 500), the system shall 에러를 반환하고 클라이언트가 fallback 처리한다"
  - type: state-driven
    text: "While solo tier이면, the system shall 기존 로컬 API 키 방식을 유지한다"

non_functional:
  - 레이턴시: proxy 추가 100ms 이내
  - 인증: Supabase Auth JWT

out_of_scope:
  - Knowledge/Ontology 클라우드 이동 (Phase 2-4)
  - 사용량 제한/과금 (사용자 많아지면)
  - solo tier 변경
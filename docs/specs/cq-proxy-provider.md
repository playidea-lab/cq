feature: cq-proxy-provider
domain: llm-gateway
description: Go LLM Gateway에 CQ Proxy 전용 Provider 추가. JWT 인증 + Anthropic API 호환 relay.

requirements:
  - type: event-driven
    text: "When LLM Gateway가 cq-proxy provider를 선택하면, the system shall Authorization: Bearer <jwt>로 Edge Function에 Anthropic Messages API 포맷 요청을 relay한다"
  - type: event-driven
    text: "When JWT가 만료 임박이면, the system shall TokenFunc로 자동 갱신 후 요청한다"
  - type: event-driven
    text: "When Edge Function이 429를 반환하면, the system shall rate limit 에러를 전파한다"
  - type: optional
    text: "If cq-proxy provider가 config에 없으면, the system shall 기존 provider만 사용한다"
  - type: unwanted
    text: "If cloud auth가 비활성이면, the system shall cq-proxy를 자동 비활성화한다"

non_functional:
  - 레이턴시: proxy 오버헤드 무시 (Edge Function이 relay)
  - 인증: Supabase Auth JWT via cloud.TokenProvider

out_of_scope:
  - AnthropicProvider 수정
  - 새 모델 추가 (haiku만)
  - Edge Function 수정 (별도 spec)
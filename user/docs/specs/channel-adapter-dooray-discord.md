feature: Channel Adapter — Dooray + Discord
domain: web-backend

requirements:
  ubiquitous:
    - Hub는 Dooray webhook을 수신하고, 연결된 Channel 세션으로 relay해야 한다
    - Channel 세션이 없으면 Hub 내장 LLM이 fallback 응답해야 한다
    - cq serve의 dooray_poller는 제거하고 Channel 어댑터로 대체해야 한다

  event_driven:
    - Dooray 메시지 수신 시 Hub → Channel push → Claude Code 세션이 처리
    - Claude Code가 c4_dooray_reply 호출 시 Dooray Incoming Webhook으로 전송
    - Hub가 Channel 연결 상태를 추적해야 한다 (연결/해제 감지)

  state_driven:
    - Channel 세션 연결 시 Hub → Channel relay 모드
    - Channel 세션 미연결 시 Hub → 내장 LLM fallback 모드

  optional:
    - Discord는 공식 MCP Channel 플러그인 사용 (직접 구현 안 함)
    - Slack은 공식 MCP 서버 사용

  unwanted:
    - cq serve dooray_poller의 pop 경쟁 재발
    - Hub의 GET /v1/dooray/pending pop 큐 유지
    - allowlist 없는 메시지 수신

out_of_scope:
  - Tauri 데스크탑 앱
  - C3 EventBus 직접 의존
  - Discord/Slack 커스텀 봇 구현

non_functional:
  - WS 재연결 backoff (1s → 30s max)
  - Hub fallback 전환 지연 < 5초
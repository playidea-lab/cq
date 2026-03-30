feature: LLM-Agnostic 외장 두뇌 Phase 2 — OAuth + Sync
domain: web-backend
requirements:
  - id: E1
    type: ubiquitous
    text: "MCP 서버에 대한 모든 요청은 Supabase Auth가 발급한 OAuth 2.1 토큰으로 인증되어야 한다."
  - id: E2
    type: state-driven
    text: "cq serve가 실행 중일 때, 5분마다 Cloud→Local Pull을 수행하여 외부에서 저장된 스냅샷이 로컬 검색에 반영되어야 한다."
  - id: E3
    type: event-driven
    text: "cq serve가 시작되면, 즉시 Cloud→Local Pull을 1회 수행하여 최신 상태를 반영해야 한다."
  - id: E4
    type: ubiquitous
    text: "ChatGPT Apps & Connectors에서 OAuth 인증으로 CQ MCP 서버에 연결할 수 있어야 한다."
  - id: E5
    type: ubiquitous
    text: "claude.ai Connectors에서 OAuth 인증으로 CQ MCP 서버에 연결할 수 있어야 한다."
non_functional:
  - "Supabase Auth OAuth 2.1 Server 활용 (자체 구현 없음)"
  - "knowledge.Pull() 이미 구현됨 — serve 루프에 polling 추가만"
  - "기존 MCP Edge Function에 JWT 검증 추가"
out_of_scope:
  - 자체 OAuth 서버 구현
  - 실시간 Realtime 동기화 (polling으로 충분)
  - 도구 확장 (P3에서 별도 진행)
feature: LLM-Agnostic 외장 두뇌
domain: web-backend
requirements:
  - id: E1
    type: ubiquitous
    text: "CQ serve가 실행 중일 때, ChatGPT Developer Mode에서 CQ MCP URL을 등록하면 ChatGPT 대화에서 c4_knowledge_search, c4_status 등 CQ 도구를 호출할 수 있다."
  - id: E2
    type: event-driven
    text: "유저가 '이거 저장해'라고 말하면, LLM이 cq_snapshot 도구를 호출하여 대화 요약 + 결정사항 + 미결 질문을 구조화하여 knowledge base에 저장한다."
  - id: E3
    type: event-driven
    text: "유저가 다른 LLM에서 '아까 그거 이어서'라고 말하면, LLM이 cq_recall 도구를 호출하여 관련 스냅샷을 검색하고 컨텍스트로 제공한다."
  - id: E4
    type: ubiquitous
    text: "모든 Remote MCP 요청은 API Key로 인증되어야 한다."
non_functional:
  - "mcphttp는 이미 구현 완료 (internal/serve/mcphttp/mcphttp.go)"
  - "knowledge API 이미 존재 (c4_knowledge_record, c4_knowledge_search)"
out_of_scope:
  - OAuth 2.1 구현 (Bearer token으로 시작)
  - 웹 UI 대시보드
  - 자동 대화 전문 수집 (스냅샷만)
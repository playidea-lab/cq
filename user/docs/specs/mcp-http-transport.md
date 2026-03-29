feature: mcp-http-transport-docs-and-verification
domain: docs + infra
description: 이미 구현된 HTTP MCP transport의 E2E 검증 + 문서화 + 클라이언트 연결 가이드

context: |
  internal/serve/mcphttp/mcphttp.go에 HTTP MCP transport가 이미 완전 구현됨.
  POST /mcp (JSON-RPC) + GET /mcp (SSE keepalive) + X-API-Key 인증.
  하지만 사용자 문서가 없고, E2E 검증이 안 되어 있고, 클라이언트 연결 방법이 안내되지 않음.

requirements:
  - type: event-driven
    text: "When cq serve가 실행되면, the system shall HTTP MCP 엔드포인트가 정상 동작하는지 E2E 테스트로 검증한다"
  - type: ubiquitous
    text: "The system shall GitHub Pages에 HTTP MCP 사용 가이드를 제공한다"
  - type: event-driven
    text: "When 사용자가 원격에서 CQ에 접속하려 하면, the system shall .mcp.json 설정 예시를 제공한다"

out_of_scope:
  - HTTP transport 자체 구현 (이미 완료)
  - WebSocket transport
  - 자체 클라이언트 SDK
feature: relay-mcp-server
domain: go
description: Fly.io WSS relay로 NAT 관통 + HTTP-MCP 원격 접근. 워커가 독립 MCP 서버가 됨.

requirements:
  functional:
    - type: event-driven
      text: "When 워커가 cq serve를 실행하면, the system shall relay.pilab.kr에 WSS 연결을 맺고 worker_id를 등록한다"
    - type: unwanted
      text: "If WSS 연결이 끊어지면, the system shall exponential backoff로 자동 재연결한다"
    - type: event-driven
      text: "When 클라이언트가 /w/{id}/mcp에 HTTP 요청을 보내면, the system shall 해당 워커의 WSS로 JSON-RPC를 전달하고 응답을 HTTP로 반환한다"
    - type: event-driven
      text: "When Hub이 잡을 push하면, the system shall relay 경유로 워커에 즉시 전달한다"
    - type: unwanted
      text: "If 워커가 오프라인이면, the system shall 503을 반환한다"
    - type: event-driven
      text: "When 인증이 필요하면, the system shall cq auth JWT를 검증한다"

  non_functional:
    - "relay 서버는 stateless (인메모리 conn map)"
    - "응답 지연: relay 추가 < 50ms"
    - "워커 재연결: 5초 이내"
    - "Pull fallback 항상 유지"

  out_of_scope:
    - "멀티 리전 relay (v2)"
    - "relay HA (단일 인스턴스로 시작)"

verification:
  type: cli
  commands:
    - "cd infra/relay && go build ./..."
    - "cd infra/relay && go test ./..."
    - "cd c4-core && go build ./..."
    - "cd c4-core && go test ./internal/relay/..."
feature: token-free-mcp-json
domain: infra
summary: "cq serve 로컬 프록시로 .mcp.json에서 시크릿 제거 — 커밋 가능한 설정 파일"

requirements:
  ubiquitous:
    - ".mcp.json의 worker 항목에 Authorization 헤더가 포함되지 않는다"
    - "cq serve가 /w/{worker}/mcp relay proxy 엔드포인트를 제공한다"
    - ".mcp.json은 git에 커밋 가능해야 한다"

  event_driven:
    - "WHEN cq init 실행 THEN .mcp.json worker URL을 http://localhost:{serve_port}/w/{worker}/mcp로 설정 (토큰 없이)"
    - "WHEN cq serve에 /w/{worker}/mcp 요청 도착 THEN TokenProvider에서 fresh JWT를 가져와 relay에 포워딩"
    - "WHEN cq init 실행 THEN .gitignore에 .c4/ 자동 추가"

  state_driven:
    - "WHILE cq serve 실행 중 THEN 모든 relay proxy 요청에 유효한 토큰이 자동 주입된다"

  unwanted:
    - ".mcp.json에 Bearer 토큰, API 키 등 시크릿이 포함되면 안 된다"
    - "cq serve 미실행 시 워커 도구 unavailable (graceful, 크래시 아님)"

non_functional:
  - "프록시 레이턴시: localhost hop <1ms"
  - "기존 .mcp.json 자동 마이그레이션 (토큰 제거 + URL 변경)"

out_of_scope:
  - "relay 서버 자체 변경"
  - "cq serve 포트 동적 할당"

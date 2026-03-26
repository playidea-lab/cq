feature: remote-mcp-oauth21
domain: web-backend
description: CQ Remote MCP OAuth 2.1 Authorization Server — CF Worker 통합

requirements:
  - type: event
    id: E1
    text: "WHEN 원격 MCP 클라이언트가 mcp.pilab.kr에 연결을 시도하면, THEN 시스템은 OAuth 2.1 흐름(DCR + PKCE + authorization code)으로 인증한다."
  - type: event
    id: E2
    text: "WHEN 인증된 사용자가 c4_knowledge_search를 호출하면, THEN 시스템은 해당 user_id의 모든 프로젝트 knowledge를 검색하여 반환한다. IF project 파라미터가 주어지면, THEN 해당 프로젝트로 필터링한다."
  - type: event
    id: E3
    text: "WHEN 인증된 사용자가 c4_knowledge_record를 호출하면, THEN 시스템은 user_id와 연결된 기본 프로젝트(최근 활성)에 저장한다."
  - type: event
    id: E4
    text: "WHEN 인증된 사용자가 c4_status를 호출하면, THEN 시스템은 사용자의 프로젝트 태스크/워커 상태를 반환한다."
  - type: state
    id: S1
    text: "WHILE 시스템이 운영 중일 때, 기존 API key/URL token 인증도 하위 호환 유지한다."
  - type: unwanted
    id: U1
    text: "IF 인증되지 않은 요청이 도구를 호출하면, THEN 시스템은 401 + WWW-Authenticate 헤더를 반환한다. initialize/tools/list는 discovery 허용."
  - type: unwanted
    id: U2
    text: "IF 다른 사용자의 knowledge가 검색 결과에 포함되면, THEN 시스템은 이를 필터링하여 차단한다 (user_id 격리)."

non_functional:
  - "보안: 토큰은 KV에 해시만 저장. props는 토큰 키로 암호화."
  - "성능: MCP 도구 응답 < 2초 (Supabase API 호출 포함)."
  - "비용: CF Worker 무료 tier 범위 내 (10만 req/일)."

out_of_scope:
  - "로컬 MCP (Go 바이너리) 변경"
  - "Remote MCP 도구 추가 (3개 유지)"
  - "Supabase DB 스키마 변경"
  - "CMID (Client Metadata Documents) — MCP 스펙 아직 draft"
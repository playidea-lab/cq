feature: session-knowledge-loop
domain: web-backend
description: AI 대화 세션 자동 수집 → 비동기 요약 → 다음 세션 재등장

requirements:
  - type: event
    id: E1
    text: "WHEN Claude Code 세션이 종료되면(Stop 훅), THEN 시스템은 해당 세션의 메타데이터(세션 ID, 시작/종료 시간, 도구, 프로젝트, 턴 수, JSONL 경로)를 즉시 인덱싱한다."
  - type: event
    id: E2
    text: "WHEN Gemini CLI 세션이 종료되면(SessionEnd 훅), THEN 시스템은 동일하게 메타데이터를 인덱싱한다."
  - type: state
    id: E3
    text: "WHILE cq serve가 실행 중일 때, 시스템은 ~/.codex/sessions/ 등 감시 대상 디렉토리의 변경을 감지하여 새/업데이트된 세션을 인덱싱한다."
  - type: state
    id: E4
    text: "WHILE cq serve가 실행 중일 때, 시스템은 미요약 세션을 발견하면 LLM으로 요약하여 c4_knowledge_record에 저장한다."
  - type: event
    id: E5
    text: "WHEN 새 세션이 시작되면, THEN 시스템은 같은 프로젝트의 최근 세션 요약을 knowledge_context로 주입한다."

non_functional:
  - "성능: 메타데이터 인덱싱은 0.5초 이내"
  - "비용: 요약은 배치 처리, sonnet/haiku급"
  - "프라이버시: 세션 원문은 로컬에만. 클라우드에는 요약만"

out_of_scope:
  - "Cursor 세션 수집"
  - "ChatGPT 세션 수집 (Remote MCP 별도 경로)"
  - "팀 공유 (v1은 개인용)"
  - "부패 방지/자동 무효화"
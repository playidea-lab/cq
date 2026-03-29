feature: seamless-session-capture
domain: web-backend
description: "모든 AI 세션이 CQ 지식 체계에 자연스럽게 흡수되는 배관 수리 + 글로벌화"

requirements:
  functional:
    # P0: 배관 수리
    - id: E1
      pattern: event-driven
      text: "WHEN cq session index가 호출되고 프로젝트에 .c4/가 없으면, THEN 글로벌 ~/.c4/c4.db에 인덱싱한다"
    - id: E2
      pattern: unwanted
      text: "IF LLM 요약이 429/timeout으로 실패하면, THEN 메타데이터만이라도 knowledge에 등록하고 미요약 플래그를 남긴다"
    - id: E3
      pattern: state-driven
      text: "WHILE cq serve가 실행 중일 때, 미요약 세션을 주기적으로 발견하여 배치 요약한다 (background summarizer)"
    - id: E4
      pattern: event-driven
      text: "WHEN background summarizer가 요약할 때, THEN haiku급 모델로 비용을 최소화한다"

  non_functional:
    - id: NF-01
      text: "세션 인덱싱은 0.5초 이내 (세션 종료 블로킹 없음)"
    - id: NF-02
      text: "원본 대화는 로컬에만 보관, 요약만 knowledge DB에 저장"
    - id: NF-03
      text: "background summarizer 동시 요약 최대 2개 (비용/속도 균형)"

  out_of_scope:
    - Codex CLI watcher (P1)
    - Cursor SQLite reader (P1)
    - "기억해" 온보딩 가이드 (P2)
    - c4_session_summary 강화 (P2)
    - 브라우저 확장 (미정)

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./..."
    - "cd c4-core && go vet ./..."
    - "cd c4-core && go test ./cmd/c4/... -run TestSession"

feature: cq-import-history-phase1
domain: web-backend
description: "cq import chatgpt <zip> — Phase 1 Sessions 등록 (FREE)"

requirements:
  functional:
    - id: FR-01
      pattern: event-driven
      text: "WHEN cq import chatgpt <zip> 실행 시 THEN zip 파싱 → conversations-*.json에서 대화 메타데이터 추출 → 요약 출력"
    - id: FR-02
      pattern: event-driven
      text: "WHEN 파싱 완료 시 THEN 각 대화를 ~/.c4/imports/chatgpt/sessions.json에 저장"
    - id: FR-03
      pattern: event-driven
      text: "WHEN cq sessions 실행 시 THEN chatgpt/ 접두어로 임포트된 세션 표시"
    - id: FR-04
      pattern: event-driven
      text: "WHEN chatgpt 세션 선택 후 Space(history) 시 THEN 대화 내용 표시"
    - id: FR-05
      pattern: unwanted
      text: "IF zip 파일이 ChatGPT export 형식이 아닌 경우 THEN 에러 메시지 출력"

  non_functional:
    - id: NF-01
      text: "1,300개 대화 파싱 + 등록 < 10초"
    - id: NF-02
      text: "원본 대화는 로컬에만 보관 (~/.c4/imports/chatgpt/)"

  out_of_scope:
    - Persona 추출 (Phase 2)
    - Knowledge 추출 (Phase 3)
    - Toss 결제 연동
    - claude/gemini/perplexity import (이후 확장)
    - 이미지/파일 첨부 임포트

verification:
  type: cli
  commands:
    - "go build ./cmd/c4/..."
    - "go test ./cmd/c4/... -run TestImport"

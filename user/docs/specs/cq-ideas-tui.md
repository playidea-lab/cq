feature: cq-ideas-tui
domain: web-backend
description: "아이디어 중심 TUI + sessions 복수 아이디어 표시"

requirements:
  functional:
    - id: FR-01
      pattern: event-driven
      text: "WHEN cq ideas 실행 시 THEN .c4/ideas/*.md 파일 목록을 bubbletea TUI로 표시 (AltScreen)"
    - id: FR-02
      pattern: event-driven
      text: "WHEN 아이디어 선택 후 → 키 THEN 관련 spec/design/session 파일 트리 펼침"
    - id: FR-03
      pattern: event-driven
      text: "WHEN 검색 입력 시 THEN 파일명 + 파일 내용으로 필터링"
    - id: FR-04
      pattern: event-driven
      text: "WHEN 대시보드에서 i 키 THEN cq ideas TUI 열기"
    - id: FR-05
      pattern: event-driven
      text: "WHEN cq sessions에서 세션 선택 시 THEN 해당 세션과 관련된 모든 아이디어 파일 표시 (복수)"
    - id: FR-06
      pattern: event-driven
      text: "WHEN matchIdeasByTag 호출 시 THEN 3글자 이하 토큰은 퍼지 매칭에서 제외"

  non_functional:
    - id: NF-01
      text: "96개 파일 스캔 <500ms"
    - id: NF-02
      text: "cq sessions와 동일한 디자인 시스템"

  out_of_scope:
    - 아이디어 생성/편집 (/pi)
    - 아이디어 삭제
    - Enter로 세션 열기 (v2)

verification:
  type: cli
  commands:
    - "go build ./cmd/c4/..."
    - "go test ./cmd/c4/... -run TestIdeas"

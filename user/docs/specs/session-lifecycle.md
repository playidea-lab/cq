feature: session-lifecycle
domain: web-backend
version: 2.0

description: |
  세션 생명주기 완성 — SessionEnd 훅으로 자동 close.
  done 전환 + LLM 요약 + knowledge 저장 + persona 학습을 원샷 처리.
  "대화는 지식이다" 루프의 마지막 고리.

requirements:
  - id: R1
    pattern: event-driven
    text: "WHEN SessionEnd 훅이 발생하면, THEN session-close.sh가 백그라운드로 `cq session close`를 실행해야 한다"
  - id: R2
    pattern: ubiquitous
    text: "`cq session close`는 status→done + LLM 요약 + knowledge 저장 + persona 학습을 한 번에 처리해야 한다"
  - id: R3
    pattern: event-driven
    text: "WHEN LLM 요약이 생성되면, THEN 결정사항(decisions)과 선호(preferences)를 추출하여 persona store에 저장해야 한다"
  - id: R4
    pattern: optional
    text: "IF persona 업데이트가 config에서 비활성화되면, THEN 요약과 knowledge만 저장하고 persona는 skip해야 한다"
  - id: R5
    pattern: unwanted
    text: "IF 이미 done인 세션에 close가 호출되면, THEN 중복 처리 없이 skip해야 한다 (멱등성)"
  - id: R6
    pattern: unwanted
    text: "IF LLM gateway가 불가하면, THEN status→done만 처리하고 요약은 sessionsummarizer polling에 위임해야 한다"
  - id: R7
    pattern: unwanted
    text: "Stop 훅에서는 done 전환을 하지 않아야 한다 (대화 중간 오발 방지)"

non_functional:
  - "session-close.sh는 0으로 즉시 exit (background spawn + disown)"
  - "cq session close는 30초 timeout (LLM 응답 대기)"
  - "named-sessions.json 하위호환 유지"
  - "persona 추출은 기존 cq_persona_evolve 패턴 활용"

out_of_scope:
  - "코딩 스타일 자동 추출 (rules/CLAUDE.md는 수동)"
  - "Supabase 클라우드 동기화"
  - "세션 자동 정리/GC"
  - "세션 시작 시 맥락 주입 개선 (별도 피처)"

existing_components:
  - "session-capture.sh — background spawn + disown 패턴 (검증됨)"
  - "sessionsummarizer — JSONL→LLM→knowledge 파이프라인 + buildSummarizationPrompt()"
  - "namedSessionEntry — Status/Summary 필드 이미 존재"
  - "cq_persona_evolve — Soul의 Learned 섹션에 자동 반영"
  - "cq session index — sessions DB upsert"
  - "LLM gateway — session_summarize 태스크"

verification:
  - type: cli
    command: "cq session close --session-id test-123"
    expect: "status: done, summary 생성, knowledge 저장"
  - type: hook
    event: "SessionEnd"
    expect: "session-close.sh가 exit 0으로 즉시 반환, 백그라운드에서 cq session close 실행"
  - type: unit
    command: "go test ./cmd/c4/... -run TestSessionClose"
    expect: "멱등성, LLM 실패 시 fallback, done 상태 skip 확인"

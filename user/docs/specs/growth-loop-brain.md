feature: growth-loop-brain
domain: go-backend
description: |
  세션이 쌓일수록 AI가 나를 닮아가고(루프A), 내 실패가 남의 시행착오를
  줄여주는(루프B) 성장 시스템. v1.43 captureSession 파이프라인의 뒤쪽 절반을 닫는다.
  기존 스펙(persona-ontology-l1, collective-ontology-l3, session-knowledge-loop,
  knowledge-hierarchy)의 빠진 조각을 구현.

requirements:
  # 루프 A: 페르소나 성장 (안으로)
  - type: event-driven
    id: GA-01
    text: "WHEN captureSession이 preferences를 추출하면 THEN preference_ledger.yaml에 항목별 카운트를 누적한다"
  - type: state-driven
    id: GA-02
    text: "WHILE preference 카운트가 3 이상이면 THEN claude.md의 auto-learned 섹션에 hint로 추가한다"
  - type: state-driven
    id: GA-03
    text: "WHILE preference 카운트가 5 이상이면 THEN .claude/rules/auto-learned.md에 rule로 승격한다"
  - type: event-driven
    id: GA-04
    text: "WHEN 세션이 시작되면 THEN 마지막 세션 이후 추가된 규칙을 알림으로 표시한다"
  - type: event-driven
    id: GA-05
    text: "WHEN 사용자가 자동 생성된 규칙을 삭제하면 THEN preference_ledger에 suppressed 마킹하여 재생성을 방지한다"

  # 루프 B: 집단 지식 성장 (밖으로)
  - type: event-driven
    id: GB-01
    text: "WHEN 프로젝트 knowledge에 evidence_count>=3인 인사이트가 생기면 THEN LLM으로 탈개인화(프로젝트명/유저명/파일경로 제거)하여 글로벌 후보로 마킹한다"
  - type: event-driven
    id: GB-02
    text: "WHEN 글로벌 후보가 생성되면 THEN opt-in 설정 확인 후 Supabase 글로벌 풀에 업로드한다"
  - type: event-driven
    id: GB-03
    text: "WHEN knowledge_search가 실행되면 THEN 로컬 결과와 함께 글로벌 풀의 커뮤니티 인사이트도 반환한다"

  # 성장 측정
  - type: event-driven
    id: GM-01
    text: "WHEN 세션이 종료되면 THEN 해당 세션의 AI 제안 수정 횟수를 growth_metrics.yaml에 기록한다"
  - type: ubiquitous
    id: GM-02
    text: "cq doctor는 성장 지표(규칙 수, 지식 수, 수정률 추이)를 표시한다"

  # 안전장치
  - type: unwanted
    id: GS-01
    text: "IF 자동 생성 규칙이 빌드/테스트 실패를 유발하면 THEN 해당 규칙을 자동 비활성화하고 경고한다"
  - type: unwanted
    id: GS-02
    text: "IF 탈개인화 후에도 개인정보가 남아있으면 THEN 업로드를 차단하고 수동 리뷰를 요청한다"

non_functional:
  - "preference 누적 처리는 세션 종료 시 1초 이내"
  - "탈개인화 LLM 호출은 haiku급, 1건당 <3초"
  - "글로벌 풀 업로드/다운로드 <5초"
  - "프라이버시: 코드 원문, 프로젝트명, 유저명 글로벌 전송 금지"

out_of_scope:
  - "팀 온톨로지 (L2) — 별도 스펙 존재"
  - "유료/무료 분리"
  - "실시간 스트리밍 (배치만)"
  - "기존 1,898 세션 소급 분석 (향후 별도)"

verification:
  - type: cli
    command: "cd c4-core && go build ./... && go vet ./..."
  - type: cli
    command: "cd c4-core && go test ./internal/knowledge/... ./internal/persona/... ./cmd/c4/..."

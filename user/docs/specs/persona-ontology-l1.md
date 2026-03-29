feature: persona-ontology-l1
domain: go-backend
description: |
  L1 개인 온톨로지 시스템. 기존 POP 파이프라인을 확장하여
  유저의 선택/판단을 온톨로지(4축 코어 + 자유 확장)로 구조화.
  Hub 경유 Haiku로 추출, 로컬 ontology.yaml에 저장.

requirements:
  # 추출 (Event-Driven)
  - type: event-driven
    id: REQ-EXT-01
    text: "WHEN 유저가 커밋하면 THEN 시스템은 diff를 전처리(코드 원문 제거)하여 Hub 경유 Haiku로 온톨로지 노드를 추출한다"
  - type: event-driven
    id: REQ-EXT-02
    text: "WHEN c4_submit으로 리뷰 결과(approve/reject)가 기록되면 THEN 시스템은 판단 패턴을 judgment 축에 반영한다"
  - type: event-driven
    id: REQ-EXT-03
    text: "WHEN 세션이 종료되면 THEN 시스템은 대화에서 선호 표현을 추출하여 온톨로지에 반영한다"
  - type: event-driven
    id: REQ-EXT-04
    text: "WHEN 새 프로젝트에서 CQ를 시작하면 THEN 시스템은 글로벌 온톨로지(~/.c4/personas/)에서 시드한다"

  # 상태 (State-Driven)
  - type: state-driven
    id: REQ-STR-01
    text: "WHILE 온톨로지에 동일 패턴이 3회 이상 관찰되면 THEN confidence를 HIGH로 승격한다"
  - type: state-driven
    id: REQ-STR-02
    text: "WHILE 온톨로지 노드가 코어 4축에 매핑되지 않으면 THEN extended 노드로 저장한다"

  # 필수 (Ubiquitous)
  - type: ubiquitous
    id: REQ-UBI-01
    text: "온톨로지는 ~/.c4/personas/{username}/ontology.yaml에 저장한다"
  - type: ubiquitous
    id: REQ-UBI-02
    text: "코어 스키마는 judgment, domain, style, workflow 4축이다"
  - type: ubiquitous
    id: REQ-UBI-03
    text: "Haiku 호출 시 코드 원문은 전송하지 않고 행동 요약만 전송한다"

  # 에러 (Unwanted)
  - type: unwanted
    id: REQ-UNW-01
    text: "IF Haiku 호출이 실패하면 THEN 로컬 rule-based 추출로 폴백한다"
  - type: unwanted
    id: REQ-UNW-02
    text: "IF 온톨로지 파일이 손상되면 THEN 백업에서 복원한다"

non_functional:
  - "Haiku 호출 1건당 latency < 3s"
  - "ontology.yaml 크기 < 100KB"
  - "프라이버시: 코드 원문 Hub 전송 금지"

out_of_scope:
  - "L2 팀 온톨로지 집계 (Phase 2)"
  - "L3 Hub 군집 지능 (Phase 3)"
  - "유료/무료 분리"
  - "나처럼 판단해줘 Sonnet 모드"

verification:
  - type: cli
    command: "go test ./internal/pop/..."
  - type: cli
    command: "cq pop extract --dry-run"

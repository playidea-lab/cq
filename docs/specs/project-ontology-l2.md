feature: project-ontology-l2
domain: go-backend
description: |
  L2 프로젝트 온톨로지 + 크로스-포지션 지식 순환.
  모든 포지션(프론트/백엔드/연구원)의 경험이 프로젝트 수준에서 교차.
  piki(top-down) ↔ 온톨로지(bottom-up) 상승 피드백 순환.

requirements:
  # 프로젝트 온톨로지 기본
  - type: ubiquitous
    id: REQ-PO-01
    text: "프로젝트 온톨로지는 .c4/project-ontology.yaml에 저장한다"
  - type: ubiquitous
    id: REQ-PO-02
    text: "프로젝트 온톨로지 노드는 scope(project/cross), source_role, evidence_count를 포함한다"

  # L1 → 프로젝트 추출
  - type: event-driven
    id: REQ-EX-01
    text: "WHEN L1 온톨로지에 confidence HIGH 노드가 생성되면 THEN 프로젝트 온톨로지에 반영한다"
  - type: event-driven
    id: REQ-EX-02
    text: "WHEN 세션 대화에서 크로스-포지션 피드백이 감지되면 THEN scope를 cross:{source}→{target}으로 태깅하여 프로젝트 온톨로지에 추가한다"

  # 워커 주입
  - type: event-driven
    id: REQ-INJ-01
    text: "WHEN 워커가 태스크를 받으면 THEN 프로젝트 온톨로지에서 해당 역할 관련 노드(scope *→{role})를 knowledge_context에 주입한다"

  # piki 상승 피드백
  - type: state-driven
    id: REQ-PK-01
    text: "WHILE 프로젝트 온톨로지 노드가 piki 표준과 모순되고 evidence_count가 10 이상이면 THEN piki 갱신 제안을 생성한다"

  # 신규 유저 시드
  - type: event-driven
    id: REQ-SEED-01
    text: "WHEN 새 유저가 프로젝트에서 CQ를 시작하면 THEN 프로젝트 온톨로지를 해당 유저의 L1에 시드한다"

  # 에러
  - type: unwanted
    id: REQ-UNW-01
    text: "IF 프로젝트 온톨로지 파일이 손상되면 THEN 백업에서 복원한다"

non_functional:
  - "project-ontology.yaml 크기 < 200KB"
  - "워커 주입 latency < 100ms (로컬 파일 읽기)"
  - "piki 갱신 제안은 주 1회 이하로 제한 (노이즈 방지)"

out_of_scope:
  - "L3 Hub 군집 (Phase 3)"
  - "팀 온톨로지 분리 (scope로 준비만)"
  - "piki 자동 갱신 (사람 승인 필수)"

verification:
  - type: cli
    command: "go test ./internal/ontology/..."
  - type: cli
    command: "go test ./internal/pop/..."

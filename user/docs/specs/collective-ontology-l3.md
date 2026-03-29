feature: collective-ontology-l3
domain: go-backend
description: |
  L3 Hub 군집 지능. 프로젝트 온톨로지의 익명화된 패턴을 Hub(Supabase)에
  업로드하고, 새 유저/프로젝트에 도메인별 군집 패턴을 시드.
  "CQ가 원래 똑똑한 것"으로 체감.

requirements:
  - type: event-driven
    id: REQ-HUB-01
    text: "WHEN 프로젝트 온톨로지 노드의 evidence_count>=5이면 THEN 코드 원문 없이 패턴만 익명화하여 Hub에 업로드한다"
  - type: event-driven
    id: REQ-HUB-02
    text: "WHEN 새 유저가 CQ를 시작하면 THEN Hub에서 도메인(go/python/ts)에 맞는 군집 패턴을 다운로드하여 L1에 시드한다"
  - type: ubiquitous
    id: REQ-HUB-03
    text: "업로드 시 project_id, username, 파일경로 등 식별 정보는 제거한다"
  - type: ubiquitous
    id: REQ-HUB-04
    text: "Hub에 저장된 패턴은 도메인별(go, python, typescript, research)로 분류한다"
  - type: unwanted
    id: REQ-HUB-05
    text: "IF Hub 연결이 안 되면 THEN 로컬만으로 동작한다 (graceful degradation)"

non_functional:
  - "업로드 latency < 5s"
  - "다운로드 latency < 3s"
  - "프라이버시: 코드 원문, project_id, username 절대 업로드 금지"

out_of_scope:
  - "유료/무료 분리 (별도 Phase)"
  - "패턴 품질 투표/평가 시스템"
  - "실시간 스트리밍 (배치만)"

verification:
  - type: cli
    command: "go test ./internal/ontology/..."
  - type: cli
    command: "go test ./internal/hub/..."

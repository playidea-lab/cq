feature: ontology-e2e-wiring
domain: go-backend
description: |
  3계층 온톨로지 E2E 연결. 기존 함수를 적절한 호출 지점에 연결하는 글루 코드.
  Batch A(로컬): L1→L2 추출, piki 피드백, 신규 유저 시드.
  Batch B(Hub): L2→L3 업로드, L3→L1 다운로드 + MCP 핸들러.

requirements:
  # Batch A: 로컬 연결
  - type: event-driven
    id: REQ-A1
    text: "WHEN persona_learn_from_diff가 온톨로지를 갱신하면 THEN ExtractHighConfidence를 호출하여 L1→L2 추출한다"
  - type: event-driven
    id: REQ-A2
    text: "WHEN L1→L2 추출이 완료되면 THEN DetectPikiConflicts를 호출하여 모순을 감지한다"
  - type: event-driven
    id: REQ-A3
    text: "WHEN 세션 시작 시 유저의 L1이 비어있고 프로젝트 온톨로지가 있으면 THEN SeedFromProject를 호출한다"

  # Batch B: Hub 연결
  - type: event-driven
    id: REQ-B1
    text: "WHEN c4_finish 또는 persona_learn 후 THEN 프로젝트 온톨로지를 익명화하여 Hub에 업로드한다 (best-effort)"
  - type: event-driven
    id: REQ-B2
    text: "WHEN 세션 시작 시 Hub 연결 가능하면 THEN domain 매칭 군집 패턴을 다운로드하여 L1에 시드한다 (백그라운드)"
  - type: ubiquitous
    id: REQ-MCP
    text: "c4_collective_sync와 c4_collective_stats MCP 핸들러를 등록한다"

  # 안전
  - type: unwanted
    id: REQ-SAFE-01
    text: "IF 글루 코드에서 에러 발생하면 THEN non-fatal로 처리한다 (기존 기능 차단 금지)"
  - type: unwanted
    id: REQ-SAFE-02
    text: "IF Hub 다운로드가 3초 이상 걸리면 THEN 세션 시작을 차단하지 않는다 (goroutine)"

non_functional:
  - "모든 글루 코드는 non-fatal (에러 시 로그만)"
  - "세션 시작 latency 추가 < 100ms (Hub는 백그라운드)"

verification:
  - type: cli
    command: "cd c4-core && go build ./... && go vet ./..."

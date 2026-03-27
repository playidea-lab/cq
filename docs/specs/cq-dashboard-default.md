feature: cq-dashboard-default
domain: web-backend
description: "cq (no args) = 인터랙티브 대시보드. 로그인부터 도구 실행까지."

requirements:
  functional:
    - id: FR-01
      pattern: event-driven
      text: "WHEN cq (no args) 실행 시 AND 로그인 안 됨 THEN 로그인 플로우 실행 후 도구 선택 메뉴 표시"
    - id: FR-02
      pattern: event-driven
      text: "WHEN 도구 선택 완료 시 THEN ~/.c4/config.yaml에 default_tool 저장"
    - id: FR-03
      pattern: event-driven
      text: "WHEN cq (no args) 실행 시 AND 설정 완료 THEN bubbletea 대시보드 표시 (서비스 상태, 프로젝트 상태, What's New)"
    - id: FR-04
      pattern: event-driven
      text: "WHEN 대시보드에서 Enter 키 THEN default_tool 실행 (launchTool 호출)"
    - id: FR-05
      pattern: event-driven
      text: "WHEN 대시보드에서 s 키 THEN 상태 상세 표시"
    - id: FR-06
      pattern: event-driven
      text: "WHEN 대시보드에서 c 키 THEN 도구 선택 메뉴 재표시 (설정 변경)"
    - id: FR-07
      pattern: state-driven
      text: "WHILE CQ 버전이 last_seen_version과 다를 때 THEN 대시보드에 What's New 1줄 표시 후 last_seen_version 갱신"
    - id: FR-08
      pattern: event-driven
      text: "WHEN 도구 선택 메뉴 표시 시 THEN 각 도구의 설치 여부(exec.LookPath)와 현재 버전 표시"
    - id: FR-09
      pattern: unwanted
      text: "IF 비-TTY 환경 (CI/파이프라인) THEN 대시보드 생략하고 기존 텍스트 출력 폴백"
    - id: FR-10
      pattern: optional
      text: "IF .c4/config.yaml에 tool 키가 있으면 THEN 글로벌 default_tool 대신 프로젝트별 도구 사용"

  non_functional:
    - id: NF-01
      text: "대시보드 표시까지 <1초 (버전 체크 캐시 24h)"
    - id: NF-02
      text: "Enter 1키스트로크로 도구 진입"
    - id: NF-03
      text: "cq claude 직접 실행은 기존과 동일하게 유지"

  out_of_scope:
    - 도구 자동 설치 (안내만)
    - 프로젝트 초기화 (cq init은 별도)
    - 세션 이어가기 (대시보드 v2)
    - cq whatsnew CLI 명령 (별도 태스크)
    - MCP cq_whatsnew 도구 (별도 태스크)

verification:
  type: cli
  commands:
    - "go build ./cmd/c4/..."
    - "go test ./cmd/c4/... -run TestDashboard"

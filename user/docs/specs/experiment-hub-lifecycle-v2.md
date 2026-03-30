feature: experiment-hub-lifecycle-v2
domain: web-backend
description: "Hub 실험 lifecycle 최소 루프: submit → 실행 → 알림. nohup 탈출."

requirements:
  functional:
    - id: FR-01
      pattern: unwanted
      text: "IF hub.supabase_url이 비어있으면, THEN Hub Client는 cloud.url을 fallback으로 사용한다"
    - id: FR-02
      pattern: unwanted
      text: "IF hub.supabase_key가 비어있으면, THEN Hub Client는 cloud.anon_key를 fallback으로 사용한다"
    - id: FR-03
      pattern: event-driven
      text: "WHEN 사용자가 Mac에서 cq hub list를 실행하면, THEN 401 없이 잡 목록이 반환된다"
    - id: FR-04
      pattern: event-driven
      text: "WHEN job의 workdir가 비어있으면, THEN 워커는 자신의 현재 디렉토리를 사용한다"
    - id: FR-05
      pattern: event-driven
      text: "WHEN job의 workdir가 ~로 시작하면, THEN 워커는 $HOME으로 치환하여 사용한다"
    - id: FR-06
      pattern: event-driven
      text: "WHEN Hub Worker가 잡을 완료(SUCCEEDED)하면, THEN Telegram 알림을 발송한다"
    - id: FR-07
      pattern: event-driven
      text: "WHEN Hub Worker가 잡을 실패(FAILED)하면, THEN Telegram 알림을 발송한다 (exit code 포함)"

  non_functional:
    - "Hub Client fallback은 기존 명시적 hub.supabase_url/key 설정이 있으면 그것을 우선한다"
    - "Telegram 알림 실패가 잡 상태 보고를 막지 않는다 (fire-and-forget)"
    - "기존 Hub API/MCP 도구 스키마 변경 없음"

  out_of_scope:
    - "@metric=value stdout 파싱 (P3, 별도 작업)"
    - "stdout 파일 저장 (P3, 별도 작업)"
    - "QUEUED/RUNNING 좀비 자동 정리 (P4, 별도 작업)"
    - "ExperimentWrapper 자동 활성화 (v1 design 범위, 별도 작업)"
    - "Web UI 대시보드"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/hub/..."
    - "cd c4-core && go test ./internal/serve/..."

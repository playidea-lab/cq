feature: jobs-experiment-dashboard
domain: cli-tui
summary: 경쟁하는 실험들의 메트릭 궤적을 나란히 비교하고 지는 놈을 kill하는 TUI

requirements:
  - id: E1
    pattern: ubiquitous
    text: cq jobs는 hub_jobs + hub_metrics를 폴링하여 실험 목록을 primary_metric 기준으로 정렬 표시한다
  - id: E2
    pattern: ubiquitous
    text: 각 job 행에 status, job name, worker, step 수, primary metric 현재값, best값, sparkline 궤적을 표시한다
  - id: E3
    pattern: event-driven
    text: job을 선택하면 해당 job의 전체 메트릭 시계열을 detail 패널에 표시한다
  - id: E4
    pattern: event-driven
    text: compare 모드에서 완료된 job을 선택하여 현재 job과 step 기준으로 궤적을 겹쳐 비교한다
  - id: E5
    pattern: event-driven
    text: k 키를 누르면 선택된 running job에 cancel API를 호출한다 (확인 프롬프트 후)
  - id: E6
    pattern: state-driven
    text: running job이 없으면 최근 완료/실패 job 목록을 표시한다
  - id: E7
    pattern: unwanted
    text: 메트릭 폴링 실패 시 TUI가 크래시하지 않고 마지막 값을 유지한다

non_functional:
  - 최소 터미널 크기 80x24 지원
  - Hub 폴링 3-5초 tick (Supabase rate limit 내)
  - 기존 의존성만 사용 (bubbletea, lipgloss, ntcharts)

out_of_scope:
  - Supabase Realtime (WebSocket) 스트리밍
  - 자동 kill / 알림 (나중)
  - job 그룹/태그 시스템 (Phase 3)
  - job submit 기능

verification:
  type: cli
  commands:
    - cd c4-core && go build ./cmd/c4/...
    - cd c4-core && go vet ./cmd/c4/...
    - cd c4-core && go test ./cmd/c4/... -run TestJobs
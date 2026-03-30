feature: worker-dashboard-tui
domain: cli-tui
summary: cq workers를 htop 스타일 실시간 bubbletea TUI로 업그레이드

requirements:
  - id: R1
    pattern: ubiquitous
    text: cq workers는 bubbletea TUI로 워커 fleet 상태를 실시간 표시한다
  - id: R2
    pattern: ubiquitous
    text: TUI는 3초 tick으로 Hub REST + Relay /health를 폴링하여 갱신한다
  - id: R3
    pattern: ubiquitous
    text: 각 워커 행은 hostname, status, GPU model, GPU utilization bar, 현재 job을 표시한다
  - id: R4
    pattern: event-driven
    text: 워커를 선택하면 해당 워커의 실행 중 job 메트릭을 sparkline으로 표시한다
  - id: R5
    pattern: event-driven
    text: 메트릭 sparkline에서 Enter를 누르면 StreamLineChart로 상세 확장한다
  - id: R6
    pattern: event-driven
    text: 워커를 선택하면 live log 패널에 해당 job의 stdout를 스트리밍한다
  - id: R7
    pattern: ubiquitous
    text: 메트릭은 @key=value 프로토콜을 파싱하여 자동 추출한다
  - id: R8
    pattern: state-driven
    text: Relay 연결이 있으면 WebSocket 스트리밍, 없으면 hub_job_logs 폴링
  - id: R9
    pattern: unwanted
    text: 워커가 offline이 되면 행에 last seen 시간과 시각적 경고를 표시한다

non_functional:
  - 최소 터미널 크기 80x24 지원, 반응형 레이아웃
  - Hub 폴링 3초 — Supabase rate limit 내 유지
  - ntcharts 외 새 외부 의존성 추가 금지

out_of_scope:
  - 워커 프루닝/재시작 등 관리 액션 (v2)
  - job 제출/취소
  - 알림/알람 (텔레그램 봇 담당)

verification:
  type: cli
  commands:
    - go build ./cmd/c4/...
    - go test ./cmd/c4/... -run TestWorker
    - go vet ./cmd/c4/...
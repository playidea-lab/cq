feature: metric-best-tracking
domain: web-backend
version: 1.0

description: |
  "추적은 무조건, best 판단은 선언한 사람만"
  hub_jobs에 primary_metric + lower_is_better 선언 → Worker가 best_metric 자동 갱신.
  미선언 시 best_metric = NULL 유지, hub_metrics 시계열만 저장.

requirements:
  - id: R1
    pattern: ubiquitous
    text: "Worker는 job stdout에서 모든 @key=value를 hub_metrics에 저장해야 한다 (기존 동작 유지)"
  - id: R2
    pattern: state-driven
    text: "primary_metric + lower_is_better가 선언된 job에서 메트릭 파싱 시, key가 primary_metric과 일치하면 hub_jobs.best_metric을 갱신해야 한다"
  - id: R3
    pattern: state-driven
    text: "lower_is_better=true이면 min, false이면 max로 best_metric을 비교해야 한다"
  - id: R4
    pattern: unwanted
    text: "primary_metric 미선언 시 best_metric은 NULL로 유지해야 한다"
  - id: R5
    pattern: event-driven
    text: "config.yaml hub.primary_metric/lower_is_better를 기본값으로, CLI --primary-metric/--lower-is-better로 오버라이드할 수 있어야 한다"
  - id: R6
    pattern: unwanted
    text: "best_metric 갱신 실패가 job 실행을 중단해서는 안 된다"

non_functional:
  - 기존 hub_metrics INSERT 동작에 영향 없음
  - best_metric UPDATE는 best-effort (fire-and-forget)
  - 기존 JobSubmitRequest/Job JSON 하위호환 유지 (omitempty)

out_of_scope:
  - cq hub watch 실시간 스트리밍
  - 메트릭 시각화 대시보드
  - ConvergenceChecker와의 통합 (별도 작업)

verification:
  - type: cli
    command: "cd c4-core && go build ./... && go vet ./..."
    expect: "빌드 성공"
  - type: cli
    command: "cd c4-core && go test ./internal/serve/... ./internal/hub/..."
    expect: "테스트 통과"

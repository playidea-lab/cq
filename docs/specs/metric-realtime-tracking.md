feature: metric-realtime-tracking
domain: web-backend
description: "Worker가 job stdout에서 @key=value 패턴을 파싱하여 Hub hub_metrics 테이블에 실시간 전송"

requirements:
  functional:
    - id: FR-01
      pattern: ubiquitous
      text: "Worker는 job stdout에서 @key=value 패턴 (@(\\w+)=([0-9.e+-]+))을 파싱해야 한다"
    - id: FR-02
      pattern: ubiquitous
      text: "파싱된 메트릭은 Hub hub_metrics 테이블에 job_id, step, metrics로 INSERT되어야 한다"
    - id: FR-03
      pattern: event-driven
      text: "WHEN 메트릭이 수집되면, THEN hub_jobs.best_metric도 업데이트한다 (최소값 기준)"
    - id: FR-04
      pattern: unwanted
      text: "IF 메트릭 파싱/전송이 실패하면, THEN job 실행은 중단되지 않는다 (best-effort)"
    - id: FR-05
      pattern: ubiquitous
      text: "Worker는 stdout을 os.Stderr에 계속 출력하면서 동시에 파싱해야 한다 (tee 패턴)"

  non_functional:
    - "메트릭 전송: 라인 단위 즉시 (v1). 배치 최적화는 v2."
    - "기존 hub_metrics 테이블/RLS 변경 없음"
    - "Hub Client의 LogMetrics 또는 Supabase 직접 INSERT 중 적합한 것 사용"

  out_of_scope:
    - "cq hub watch 실시간 스트리밍 (별도 기능)"
    - "메트릭 시각화 대시보드"
    - "best_metric 자동 업데이트 (hub_jobs 테이블) — v2"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/serve/..."
    - "cd c4-core && go test ./internal/hub/..."

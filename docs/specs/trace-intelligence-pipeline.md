feature: trace-intelligence-pipeline
domain: go-backend
summary: 수집된 137K+ trace를 활용 — cost 산출, session↔task 매핑, task_type 추론, 자동 리포트, Persona 연결

requirements:
  ubiquitous:
    - "trace_steps의 cost_usd는 모델 가격표(llm.LookupModel) 기반으로 산출되어야 한다"
    - "JSONL의 worker 프롬프트에서 task_id와 task_type을 자동 추출해야 한다"
    - "DetectPatterns()는 trace 데이터 기반 LLM 사용 패턴을 포함해야 한다"

  event_driven:
    - "WHEN HarnessWatcher가 assistant usage를 추출하면 THEN cost_usd를 함께 계산하여 저장한다"
    - "WHEN JSONL 세션의 첫 user 메시지를 읽으면 THEN task_id/task_type을 추출하여 trace에 설정한다"
    - "WHEN 일일 cron 또는 세션 종료 시 THEN TraceAnalyzer 집계를 knowledge에 자동 기록한다"

  unwanted:
    - "가격표에 없는 모델의 cost_usd를 에러로 처리하면 안 된다 (0으로 유지)"
    - "task_type 추론 실패가 trace 수집을 방해하면 안 된다"

  optional:
    - "IF task_type이 추론되면 THEN observe_policy의 SuggestRoutes에서 활용한다"

out_of_scope:
  - "Eval 벤치마크 데이터셋 구축 (GAP-6)"
  - "라우팅 자동 적용 (v2)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/harness/..."
    - "cd c4-core && go test ./internal/observe/..."
    - "cd c4-core && go test ./internal/mcp/handlers/..."

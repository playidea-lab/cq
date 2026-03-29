feature: c7-trace-eval
domain: go-backend
summary: C7 Observe 확장 — Trace 수집/학습 루프 + Eval 벤치마크 인프라

requirements:
  ubiquitous:
    - "C7은 모든 LLM 호출을 TraceStep으로 수집하여 SQLite traces 테이블에 비동기 저장해야 한다"
    - "TraceStep은 type(LLM_CALL/TOOL_CALL), provider, model, tokens, latency_ms, cost_usd, success를 포함해야 한다"
    - "Trace는 session_id와 task_id(nullable)로 상관관계를 추적해야 한다"
    - "TraceAnalyzer는 task_type별 모델 성공률과 품질 점수를 집계해야 한다"
    - "Eval Runner는 TOML 설정으로 N모델 × M벤치마크 매트릭스를 실행해야 한다"
    - "Eval 결과는 JSONL로 저장하고 MetricStats(mean/p50/p90/p99)로 집계해야 한다"

  event_driven:
    - "WHEN LLM Gateway.Chat()이 호출되면 THEN TraceCollector가 LLM_CALL step을 기록한다"
    - "WHEN c4_submit으로 태스크가 완료되면 THEN 해당 session의 Trace에 outcome을 연결한다"
    - "WHEN trace 샘플이 min_samples(5) 이상 쌓이면 THEN TraceDrivenPolicy가 라우팅 테이블을 업데이트한다"
    - "WHEN cq bench run이 실행되면 THEN EvalRunner가 매트릭스를 확장하고 병렬 실행한다"

  state_driven:
    - "WHILE trace 샘플이 min_samples 미만일 때 THEN default 모델로 라우팅한다 (탐색 모드)"
    - "WHILE trace 샘플이 min_samples 이상일 때 THEN 학습된 policy로 라우팅한다 (활용 모드)"

  optional:
    - "IF c7_observe 빌드 태그가 없으면 THEN trace 수집은 no-op이다"
    - "IF EventBus가 연결되어 있으면 THEN trace.recorded 이벤트를 발행한다"

  unwanted:
    - "Trace 수집이 Gateway.Chat() 지연을 10ms 이상 증가시키면 안 된다"
    - "Policy 업데이트가 동기적으로 라우팅을 블로킹하면 안 된다"
    - "새 빌드 태그를 추가하면 안 된다 (기존 c7_observe 태그 사용)"

out_of_scope:
  - "SFT/GRPO 파인튜닝 (외부 모델 사용)"
  - "에너지/전력 측정 (Edge 전용, 별도 작업)"
  - "Hub worker VRAM 매칭 (갈래 A, 별도 작업)"
  - "Bandit 알고리즘 (task_type 분류가 명확하여 불필요)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/observe/..."
    - "cd c4-core && go test ./internal/llm/..."

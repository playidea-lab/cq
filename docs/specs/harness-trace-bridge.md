feature: harness-trace-bridge
domain: go-backend
summary: HarnessWatcher의 JSONL 파서에서 LLM usage 메타데이터를 추출하여 Observe trace_steps에 자동 기록

requirements:
  ubiquitous:
    - "HarnessWatcher는 Claude Code JSONL에서 assistant 메시지의 model/usage를 추출하여 trace_steps에 저장해야 한다"
    - "수집된 데이터는 기존 c4_observe_traces, c4_observe_trace_stats, c4_observe_policy에서 조회 가능해야 한다"
    - "모든 프로젝트의 JSONL을 감시해야 한다"

  event_driven:
    - "WHEN Claude Code가 assistant 응답을 JSONL에 기록하면 THEN JournalWatcher가 usage를 추출하여 trace_steps에 INSERT한다"

  unwanted:
    - "대화 내용(content)을 trace에 저장하면 안 된다 (메타데이터만)"
    - "JSONL 파싱 실패가 기존 c1push 동작을 방해하면 안 된다"
    - "HarnessWatcher의 성능에 유의미한 영향을 주면 안 된다"

  optional:
    - "IF provider가 모델명에서 추론 불가능하면 THEN provider를 'unknown'으로 저장한다"

out_of_scope:
  - "Cursor adapter 통합 (별도 후속 작업)"
  - "Knowledge 자동 리포트 생성 (별도 후속 작업)"
  - "대화 내용 저장 (기존 c1push가 담당)"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./... && go vet ./..."
    - "cd c4-core && go test ./internal/harness/..."
    - "cd c4-core && go test ./internal/observe/..."

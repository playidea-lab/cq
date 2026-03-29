feature: hub-execution-engine
domain: go
description: DAG 실행 엔진 + 크론 스케줄러 + pg_notify 실시간 워커 디스패치

requirements:
  functional:
    # DAG Execution Engine
    - type: event-driven
      text: "When DAG가 ExecuteDAG(dagID)로 실행되면, the system shall topological sort로 실행 순서를 결정하고 root 노드들을 hub_jobs로 제출한다"
    - type: event-driven
      text: "When DAG 노드의 job이 SUCCEEDED되면, the system shall 해당 노드에 의존하는 다음 노드들의 선행 조건을 확인하고 모두 충족 시 자동 제출한다"
    - type: unwanted
      text: "If DAG 노드의 job이 FAILED되면, the system shall max_retries까지 재시도하고 초과 시 DAG를 failed로 마킹한다"
    - type: event-driven
      text: "When 모든 DAG 노드가 SUCCEEDED되면, the system shall DAG status를 completed로 전환한다"
    - type: unwanted
      text: "If DAG에 순환 의존성이 있으면, the system shall ExecuteDAG에서 에러를 반환한다"
    - type: unwanted
      text: "If cq 프로세스가 재시작되면, the system shall running 상태의 DAG를 감지하고 진행을 resume한다"

    # Cron Scheduler
    - type: event-driven
      text: "When 크론 스케줄이 등록되면, the system shall cron expression에 맞춰 자동으로 job 또는 DAG를 제출한다"
    - type: state-driven
      text: "While cq 프로세스가 실행 중이면, the system shall 크론 스케줄러가 매 분 활성 체크를 수행한다"

    # pg_notify
    - type: event-driven
      text: "When Worker가 시작되고 direct_url이 설정되면, the system shall LISTEN 'new_job'으로 구독하고 알림 시 즉시 ClaimJob한다"
    - type: unwanted
      text: "If LISTEN 연결이 끊어지면, the system shall polling fallback으로 전환한다"

  non_functional:
    - "DAG 엔진은 Supabase RPC(advance_dag)로 구현 — cq 프로세스 독립"
    - "크론은 분 단위 정확도"
    - "pg_notify는 direct_url 미설정 시 기존 polling 유지 (graceful degradation)"
    - "기존 hub_jobs/hub_dags 스키마 구조 변경 최소화"

  out_of_scope:
    - "자동 Worker Affinity (이력 기반) — 수동 지정으로 충분"
    - "DAG 조건부 분기 (conditional) — v2"
    - "DAG 간 의존성 — v2"

verification:
  type: cli
  commands:
    - "cd c4-core && go build ./..."
    - "cd c4-core && go test ./internal/hub/..."
    - "cd c4-core && go vet ./..."
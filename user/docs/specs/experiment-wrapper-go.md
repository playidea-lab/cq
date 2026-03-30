feature: ExperimentWrapper Go 포팅 — @key=value 실시간 메트릭 수집
domain: web-backend
requirements:
  ubiquitous:
    - hub_worker.go claimAndRun에서 experiment_id가 있으면 MetricWriter가 stdout에 자동 삽입
    - stdout @key=value 패턴이 실시간으로 Supabase experiment_checkpoint RPC에 전송
    - MetricWriter는 io.Writer 구현, 줄 단위 버퍼링
    - 기존 outputBuf + os.Stdout 캡처에 영향 없음
  event_driven:
    - WHEN stdout 줄에 @key=value 감지 THEN channel로 metric 전송
    - WHEN metric channel에서 값 수신 THEN Supabase RPC 비동기 호출
    - WHEN 3회 연속 실패 THEN circuit breaker 활성화 + stderr 경고
  unwanted:
    - MetricWriter의 HTTP 호출이 cmd.Run() blocking
    - experiment_id 없는 잡에서 MetricWriter 활성화
    - circuit breaker 후 stdout 캡처 중단
out_of_scope:
    - caps.yaml 지원 (job payload 기반으로 충분)
    - is_best 기반 자동 모델 저장 (Phase 2)
    - Research Loop 연동 (이미 KnowledgeHubPoller로 동작)
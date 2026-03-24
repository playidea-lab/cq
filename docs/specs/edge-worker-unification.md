feature: Edge-Worker 통합
domain: web-backend
source: .c4/ideas/edge-worker-unification.md

requirements:
  functional:
    - "시스템은 Edge 관련 API, CLI, MCP 도구를 제거한다 (edge.go, hub_edge_*.go, /v1/edge/*, c4_edge_*, c4_deploy_*)"
    - "Worker 등록 시 capabilities에 추론 런타임 태그를 지원한다 (onnx, tflite, tensorrt, arm64)"
    - "WHEN 잡이 required_capabilities를 지정하면 THEN 해당 capability 가진 Worker에만 할당한다"
    - "WHEN DAG 스텝이 target_workers 배열을 지정하면 THEN 각 Worker에 동일 잡을 병렬 생성한다"
    - "WHEN 선행 잡이 완료되면 THEN depends_on으로 연결된 후속 잡들이 자동 트리거된다"
    - "WHEN Hub가 control message(shutdown, restart, health_check)를 전송하면 THEN Worker가 실행하고 결과를 보고한다"
    - "WHEN 기존 Edge 설정(hub-edge.yaml)이 발견되면 THEN 마이그레이션 가이드를 표시한다"

  non_functional:
    - "기존 Worker API 하위호환 유지"
    - "DAG fan-out 최대 100 Worker"

  out_of_scope:
    - "오프라인 독립 동작 (IoT 엣지)"
    - "자동 프로비저닝"
    - "비용 최적화 라우팅"
    - "상시 추론 모니터링 (cq serve 별도 컴포넌트)"

verification:
  type: cli
  commands:
    - "go build ./..."
    - "go test ./internal/hub/..."
    - "go vet ./..."

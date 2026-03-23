feature: relay-tunnel-transfer
domain: go
description: relay 경유 WSS 바이너리 파이프로 대용량 P2P 전송. cq transfer 한 줄.

requirements:
  functional:
    - type: event-driven
      text: "When 사용자가 cq transfer <path> --to <worker_id>를 실행하면, the system shall relay 경유 WSS 터널을 열고 tar 스트리밍으로 데이터를 전송한다"
    - type: event-driven
      text: "When 전송이 완료되면, the system shall 터널을 정리하고 전송 크기/소요시간을 표시한다"
    - type: unwanted
      text: "If 워커가 오프라인이면, the system shall 에러를 반환한다"
    - type: unwanted
      text: "If 전송 중 연결이 끊어지면, the system shall 에러를 반환한다"

  non_functional:
    - "relay는 데이터를 메모리에 버퍼링하지 않는다 (io.Copy 스트리밍)"
    - "전송 속도는 min(로컬 업로드, 워커 다운로드) 대역폭에 의존"

  out_of_scope:
    - "resume (v2)"
    - "양방향 동시 전송"
    - "디렉토리 동기화 (rsync delta)"

verification:
  type: cli
  commands:
    - "cd infra/relay && go build ./... && go test ./..."
    - "cd c4-core && go build ./... && go vet ./..."
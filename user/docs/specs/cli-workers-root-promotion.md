feature: cli-workers-root-promotion
domain: cli
summary: cq workers를 루트 명령으로 승격 + cq serve 로컬 워커를 Hub에 자동 등록

requirements:
  - id: R1
    pattern: ubiquitous
    text: cq workers는 rootCmd에 직접 등록되어 루트에서 실행 가능하다
  - id: R2
    pattern: ubiquitous
    text: cq hub workers도 기존대로 작동한다 (호환성)
  - id: R3
    pattern: event-driven
    text: cq serve가 시작하면 WorkerComponent는 Hub에 type=embedded로 자동 등록한다
  - id: R4
    pattern: ubiquitous
    text: 등록된 로컬 워커는 30초 간격으로 Hub heartbeat를 전송한다
  - id: R5
    pattern: event-driven
    text: cq serve가 종료하면 status=offline heartbeat를 전송한다
  - id: R6
    pattern: state-driven
    text: Hub이 미설정이��� 로컬 워커 등록을 skip하고 serve는 정상 동작한다
  - id: R7
    pattern: ubiquitous
    text: workers TUI에서 embedded/standalone 워커를 type 배지로 구분한다

non_functional:
  - Hub 등록은 async — serve 시작을 block하지 않는다
  - Hub 장애 시 등록 실패해도 serve 정상 동작
  - heartbeat timeout 5분 (기존 prune 로직과 동일)

out_of_scope:
  - cq hub 그룹 해체 (별도 아이디어)
  - job 관련 명령어 재편
  - hub_workers 테이블 스키마 변경 (capabilities로 type 구분)

verification:
  type: cli
  commands:
    - go build ./cmd/c4/...
    - go vet ./cmd/c4/...
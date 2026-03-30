# Spec: cq serve --worker (Hub Auto Worker)

## Feature
cq serve가 시작 시 Hub worker를 자동 등록하여 job queue에서 작업을 순차 실행.

## Domain
infra / hub / serve

## Requirements (EARS)

- R1 [Ubiquitous]: cq serve는 hub.auto_worker=true일 때 시작 시 Hub에 워커를 자동 등록해야 한다
- R2 [Ubiquitous]: 등록된 워커는 Hub에서 job을 자동으로 claim하고 subprocess로 실행해야 한다
- R3 [Ubiquitous]: job 완료 후 다음 job을 자동으로 claim해야 한다 (연속 실행)
- R4 [Event-driven]: job 실행 중 relay 요청이 오면 독립적으로 즉시 처리해야 한다
- R5 [Event-driven]: job이 실패하면 Hub에 FAILED + 에러 메시지를 보고하고 다음 job으로 넘어가야 한다
- R6 [Unwanted]: job의 OOM/크래시가 cq serve 프로세스를 죽여서는 안 된다
- R7 [State-driven]: cq serve 종료 시 실행 중인 job에 graceful shutdown 시그널을 보내야 한다

## Design Decisions

- DEC-001: 새 실행 루프 (worker_standby 래핑이 아님). hub.Client 메서드 재사용, 실행 로직은 새로 작성
- DEC-002: subprocess로 job 실행 (exec.Command). 사용자 login shell 환경 상속
- DEC-003: config hub.auto_worker (bool) + hub.worker_tags ([]string) + CLI --worker 플래그
- DEC-004: Phase 1은 메트릭 파싱 없이 실행+성공/실패만 보고

## Non-Functional
- hub.auto_worker 기본값 false (opt-in)
- worker_id는 hostname 기반 (relay와 동일)
- heartbeat 30초 간격
- lease renewal 60초 간격
- subprocess는 사용자 shell 환경 상속 (PATH, CUDA 등)

## Out of Scope
- 메트릭 파싱 (@metric=value stdout 수집) — Phase 2
- 실험 패킷 (yaml 기반 code/data/config) — Phase 2
- 의존성 체인 (depends_on) — Phase 3
- 다중 job 동시 실행 — 한 번에 하나만

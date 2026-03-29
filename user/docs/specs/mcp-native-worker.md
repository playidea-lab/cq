# Spec: MCP Native Worker — Hub Push Dispatch

## Overview
워커가 mcphttp(Streamable HTTP MCP)를 노출하고 Hub에 `mcp_url`을 등록하면,
Hub가 잡 도착 시 워커 MCP에 직접 push dispatch하는 구조.

## EARS Requirements

### R1: mcp_url Registration (MUST)
**When** 워커가 `POST /v1/workers/register`로 등록할 때,
**the system shall** `mcp_url` 필드를 수신하여 workers 테이블에 저장한다.
- `WorkerRegisterRequest.MCPURL` 또는 `Capabilities["mcp_url"]`에서 추출
- 빈 문자열이면 push 미지원 워커로 취급 (기존 pull 유지)

### R2: Push Dispatch (MUST)
**When** 새 잡이 QUEUED 상태로 생성되고 `mcp_url != ''`인 online 워커가 있을 때,
**the system shall** 해당 워커의 MCP 엔드포인트에 `hub_dispatch_job` 도구를 호출한다.
- JSON-RPC 2.0 POST, 5초 타임아웃
- 워커는 `{"status":"accepted"}` 즉시 반환
- 잡 실행은 에이전트가 비동기 수행 후 `POST /v1/jobs/{id}/complete`

### R3: Push Fallback (MUST)
**If** push dispatch가 실패하면 (네트워크 오류, 타임아웃, 비정상 응답),
**the system shall** lease를 해제하고 잡을 QUEUED로 복원하여 기존 pull 경로로 fallback한다.

### R4: hub_dispatch_job Tool (MUST)
**When** 워커의 mcphttp가 `hub_dispatch_job` 도구 호출을 받으면,
**the system shall** 잡 페이로드(job_id, lease_id, command, env, tags 등)를 수신하고
즉시 `{"status":"accepted", "job_id":"..."}` 응답을 반환한다.
- Build tag: `hub`
- 에이전트가 반환값으로 실행 판단 (c4_get_task or c4_execute)

### R5: Configurable Timeout (SHOULD)
**When** mcphttp가 도구 호출을 처리할 때,
**the system shall** `serve.mcp_http.tool_timeout_sec` 설정값을 사용한다.
- 기본값: 60초, 설정 가능

### R6: Backward Compatibility (MUST)
**The system shall** `mcp_url`이 없는 기존 워커의 pull 모드를 변경 없이 유지한다.
- `c4_worker_standby` + `AcquireLease` 경로 그대로 동작
- push는 additive — 기존 워커에 영향 없음

## Scope

### In Scope
- c5 Hub: DB migration, 모델, 스토어, API (mcp_url 필드)
- c5 Hub: Push dispatcher (dispatcher.go)
- c4-core: `hub_dispatch_job` MCP 도구
- c4-core: mcphttp 타임아웃 설정
- c4-core: worker_standby 등록 시 mcp_url 전달

### Out of Scope
- c5 standalone worker (`c5 worker`)의 mcphttp 내장 — 별도 계획
- `c4_worker_standby` 제거 — Phase 3 (추후)
- TLS/mTLS — reverse proxy 위임 (기존 보안 모델)
- MCP session 관리 — stateless request-response로 충분

## Architecture

```
[Agent] ──HTTP──→ [Worker: cq serve (mcphttp :4142)]
                       │
                       ├─ register(mcp_url) ──→ [Hub]
                       │
[Hub] ──────────────────┘
  │  잡 도착 시:
  ├─ GetWorkerForPushDispatch (mcp_url != '')
  ├─ AcquireLeaseForWorker (원자적)
  └─ POST worker:4142/mcp → hub_dispatch_job
       │
       ├─ 성공: accepted → 에이전트 실행 → complete
       └─ 실패: release lease → re-queue → pull fallback
```

## Data Changes

### DB: workers 테이블
```sql
ALTER TABLE workers ADD COLUMN mcp_url TEXT NOT NULL DEFAULT '';
```

### Model: Worker
```go
MCPURL string `json:"mcp_url,omitempty"`
```

### Model: WorkerRegisterRequest
```go
MCPURL string `json:"mcp_url,omitempty"`
```

## Key Files

| Component | File | Change |
|-----------|------|--------|
| DB migration | `c5/internal/store/sqlite.go` | ALTER TABLE |
| Model | `c5/internal/model/model.go` | Worker, WorkerRegisterRequest |
| Store | `c5/internal/store/sqlite.go` | RegisterWorker, scan, GetWorkerForPushDispatch |
| API register | `c5/internal/api/workers.go` | mcp_url 추출 |
| Dispatcher | `c5/internal/api/dispatcher.go` | 신규 — TryPushDispatch |
| Server | `c5/internal/api/server.go` | dispatcher 필드 |
| Jobs | `c5/internal/api/jobs.go` | push 트리거 |
| Dispatch tool | `c4-core/internal/mcp/handlers/worker_dispatch.go` | 신규 — hub_dispatch_job |
| Init | `c4-core/cmd/c4/mcp_init_hub.go` | dispatch 등록 |
| Standby | `c4-core/internal/mcp/handlers/worker_standby.go` | mcp_url 전달 |
| Config | `c4-core/internal/config/config.go` | ToolTimeoutSec |
| mcphttp | `c4-core/internal/serve/mcphttp/mcphttp.go` | 타임아웃 설정 |

## Validation

### Phase 1
- `go build ./...` + `go vet ./...` (c5, c4-core)
- mcp_url register → DB → list 왕복 테스트
- mcphttp 타임아웃 설정 동작 확인

### Phase 2
- push dispatch e2e: 잡 제출 → 워커 수신 → complete
- fallback: 워커 미응답 시 re-queue 확인
- 기존 pull 모드 regression 없음

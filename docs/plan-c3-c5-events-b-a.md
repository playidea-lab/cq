# Plan: C3–C5 연동 및 누락 이벤트 발행 (B + A)

> **범위**: B(누락 이벤트) + A(C3–C5 통합). 완료 후 run → refine → finish. C(Edge/Deploy)는 별도 plan.

---

## 1. Feature Overview

| 구분 | 내용 |
|------|------|
| **Feature** | c3-c5-events-integration |
| **Domain** | backend (Go MCP server, EventBus, Hub client) |
| **Goal** | (B) 문서화된 이벤트 전부 발행, (A) Hub↔EventBus 양방향 연동 |

---

## 2. Discovery (EARS Requirements)

### B. 누락 이벤트 발행

| ID | Pattern | Text |
|----|---------|------|
| REQ-B1 | Event-Driven | When a task status changes (assign/claim/auto-cascade), the system shall publish `task.updated` with task_id, status, previous_status, worker_id. |
| REQ-B2 | Event-Driven | When a knowledge document is created or an experiment is recorded (Go native), the system shall publish `knowledge.recorded` with doc_id, doc_type, title. |
| REQ-B3 | Event-Driven | When a knowledge search is performed (Go native), the system shall publish `knowledge.searched` with query, doc_type, result_count. |
| REQ-B4 | Event-Driven | When a research project is started or an iteration is recorded (Go native), the system shall publish `research.started` or `research.recorded` with project_id, iteration_id, and relevant payload. |
| REQ-B5 | Event-Driven | When a soul section is created or updated, the system shall publish `soul.updated` with username, role, section, action. |
| REQ-B6 | Event-Driven | When persona evolution is applied, the system shall publish `persona.evolved` with persona_id, suggestions, applied. |

### A. C3–C5 통합

| ID | Pattern | Text |
|----|---------|------|
| REQ-A1 | Event-Driven | When a Hub job is submitted, cancelled, or completed/failed (via MCP), the system shall publish `hub.job.submitted`, `hub.job.cancelled`, `hub.job.completed`, or `hub.job.failed` with job_id and relevant payload. |
| REQ-A2 | Event-Driven | When a worker registers with the Hub (c4_worker_standby), the system shall publish `hub.worker.registered` with worker_id and capabilities. |
| REQ-A3 | Event-Driven | When an event matches a rule with action_type `hub_submit`, the system shall submit a job to the Hub using the rule’s action_config (with template substitution). |
| REQ-A4 | Unwanted | If Hub submitter is not configured, the system shall not crash and shall record an error in the dispatcher. |

---

## 3. Design

### 3.1 Selected Option

- **B**: 기존 `notifyEventBus` / `eventPub.PublishAsync` 패턴 재사용. 각 핸들러/스토어에 EventBus Publisher 주입.
- **A-1**: C4-core MCP 핸들러에서만 발행 (Option A). C5 서버 수정 없음. Drive/Validation과 동일하게 `SetHubEventBus` + 발행.
- **A-2**: Dispatcher에 `HubSubmitter` 인터페이스 추가, `executeHubSubmit` 구현, `mcp.go`에서 Hub client 주입.

### 3.2 Components

| Component | Responsibility |
|-----------|----------------|
| SQLiteStore | Publish `task.updated` at reassignStaleOrFindPendingTask, ClaimTask. |
| Store (review) | Publish `task.updated` at completeReviewTask (auto-cascade). |
| KnowledgeNativeOpts | Add `EventPub eventbus.Publisher`; publish in record/search/experiment handlers. |
| Research handlers | Receive EventPub (via opts or reg); publish in start/record handlers. |
| Soul handlers | RegisterSoulHandlers receives EventPub; publish in setSoulSection. |
| Persona handlers | RegisterPersonaHandlers uses store or EventPub; publish in persona_evolve. |
| Hub job handlers | SetHubEventBus(pub); publish in handleHubSubmit, handleHubCancel. |
| Worker standby | WorkerDeps.EventPub; publish in handleWorkerComplete, handleWorkerStandby. |
| Dispatcher | HubSubmitter interface; executeHubSubmit with template substitution; ReplayRule support. |
| mcp.go | Wire EventPub to hub handlers and WorkerDeps; SetHubSubmitter(dispatcher). |

### 3.3 Data Flow (A-2)

```
Event (e.g. task.completed)
  → StoreEvent (if remote)
  → MatchRules
  → executeRule → case "hub_submit"
  → executeHubSubmit: parse action_config, substitute {{task_id}} etc., call HubSubmitter.SubmitJob
  → Hub API POST /jobs/submit
```

### 3.4 Decisions

| ID | Question | Decision | Rationale |
|----|----------|----------|------------|
| DEC-1 | Where to publish Hub events? | C4-core MCP handlers only | No C5 change; projectID available; same pattern as C1. |
| DEC-2 | HubSubmitter interface | Single method SubmitJob(req) (JobSubmitResponse, error) | Matches hub.Client; wrapper can hold projectID. |
| DEC-3 | Template vars for hub_submit | Same as c1_post: {{event_type}}, {{task_id}}, {{title}}, plus {{workdir}}, {{job_id}} | Reuse existing template logic. |

---

## 4. Task Breakdown

### 4.1 Dependency Graph

```
T-001 (task.updated)     T-002 (knowledge)    T-003 (research)    T-004 (soul/persona)
       |                        |                     |                     |
       +------------------------+---------------------+---------------------+
                                    |
T-005 (Hub→EventBus) ---------------+
       |
T-006 (EventBus→Hub) depends on T-005
```

- **T-001..T-004**: No dependencies between them (can run in parallel).
- **T-005**: No dependency on B tasks (can run in parallel with T-001..T-004).
- **T-006**: Depends on T-005 (dispatcher needs Hub client wired).

### 4.2 Task Definitions (for c4_add_todo)

---

#### T-001-0: task.updated 이벤트 발행

- **Title**: task.updated 이벤트 발행 (sqlite_store, store_review)
- **Scope**: `c4-core/internal/mcp/handlers/sqlite_store.go`, `store_review.go`
- **DoD**:
  - Goal: 상태 변경 시 `task.updated` 발행. payload: task_id, status, previous_status, worker_id.
  - Rationale: REQ-B1; 기존 task.created/completed 패턴과 동일하게 notifyEventBus 호출.
  - ContractSpec: (1) reassignStaleOrFindPendingTask 내 UPDATE 후 notifyEventBus("task.updated", ...). (2) ClaimTask 내 UPDATE 후 notifyEventBus("task.updated", ...). (3) store_review completeReviewTask 내 UPDATE 후 notifyEventBus("task.updated", ...). 기존 EventBus 연동 테스트 또는 수동 발행 확인.
  - CodePlacement: sqlite_store.go 678 근처, 953 근처; store_review.go 23 근처.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-002-0: Go native knowledge 이벤트 발행

- **Title**: knowledge.recorded / knowledge.searched 이벤트 발행 (knowledge_native)
- **Scope**: `c4-core/internal/mcp/handlers/knowledge_native.go`, `cmd/c4/mcp.go`
- **DoD**:
  - Goal: Go native 지식 기록/검색/실험 기록 시 EventBus 이벤트 발행.
  - Rationale: REQ-B2, REQ-B3; Python sidecar는 이미 발행 중.
  - ContractSpec: KnowledgeNativeOpts에 EventPub eventbus.Publisher 추가. knowledgeRecordNativeHandler 성공 후 knowledge.recorded (doc_id, doc_type, title). knowledgeSearchNativeHandler 성공 후 knowledge.searched (query, doc_type, result_count). experimentRecordNativeHandler 성공 후 knowledge.recorded (doc_type=experiment). mcp.go에서 knowledge 옵션에 EventPub 전달.
  - CodePlacement: knowledge_native.go opts 구조체 및 3곳 발행; mcp.go RegisterKnowledgeNativeHandlers 호출부.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-003-0: Go native research 이벤트 발행

- **Title**: research.started / research.recorded 이벤트 발행 (research_native)
- **Scope**: `c4-core/internal/mcp/handlers/research_native.go`, `cmd/c4/mcp.go`
- **DoD**:
  - Goal: 연구 프로젝트 시작/반복 기록 시 EventBus 이벤트 발행.
  - Rationale: REQ-B4.
  - ContractSpec: Research 핸들러에 EventPub 주입 (opts 또는 reg). researchStartHandler 성공 후 research.started (project_id, name, iteration_id). researchRecordHandler 성공 후 research.recorded (project_id, iteration_id, updates). mcp.go에서 EventPub 연결.
  - CodePlacement: research_native.go 107, 202 근처 + opts/reg; mcp.go.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-004-0: soul.updated / persona.evolved 이벤트 발행

- **Title**: soul.updated, persona.evolved 이벤트 발행 (soul, persona)
- **Scope**: `c4-core/internal/mcp/handlers/soul.go`, `persona.go`, `cmd/c4/mcp.go`
- **DoD**:
  - Goal: Soul 섹션 변경·페르소나 진화 시 EventBus 이벤트 발행.
  - Rationale: REQ-B5, REQ-B6.
  - ContractSpec: RegisterSoulHandlers에 EventPub 인자 추가; setSoulSection 성공 후 soul.updated (username, role, section, action). Persona 쪽은 store.eventPub 사용 또는 EventPub 주입; persona_evolve 성공 후 persona.evolved (persona_id, suggestions, applied). mcp.go에서 soul/persona 등록 시 EventPub 전달.
  - CodePlacement: soul.go setSoulSection 반환 전; persona.go handler 내; mcp.go RegisterSoulHandlers, RegisterPersonaHandlers.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-005-0: Hub → EventBus 이벤트 발행 (A-1)

- **Title**: Hub 작업/워커 이벤트 EventBus 발행 (hub_jobs, worker_standby)
- **Scope**: `c4-core/internal/mcp/handlers/hub_jobs.go`, `worker_standby.go`, `hub.go`, `cmd/c4/mcp.go`
- **DoD**:
  - Goal: Hub 제출/취소/완료/실패/워커 등록 시 해당 이벤트 발행.
  - Rationale: REQ-A1, REQ-A2; Option A (C4-core only).
  - ContractSpec: hub_jobs.go에 var hubEventPub + SetHubEventBus. handleHubSubmit 성공 후 hub.job.submitted (job_id, name, command, workdir, tags, priority). handleHubCancel 성공 후 hub.job.cancelled (job_id). worker_standby.go WorkerDeps에 EventPub 필드; handleWorkerComplete 성공 후 hub.job.completed 또는 hub.job.failed (job_id, status, exit_code, commit_sha, summary, worker_id); handleWorkerStandby에서 RegisterWorker 성공 후 hub.worker.registered (worker_id, capabilities). mcp.go: wireEventBusClient에서 SetHubEventBus(ebClient); WorkerDeps 생성 시 EventPub 설정 (hub.enabled일 때).
  - CodePlacement: hub_jobs.go 전역 + 2곳 발행; worker_standby.go deps + 2곳 발행; mcp.go 352–369, WorkerDeps 생성부.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-006-0: EventBus → Hub hub_submit 액션 (A-2)

- **Title**: Dispatcher hub_submit 액션 및 HubSubmitter 연동
- **Scope**: `c4-core/internal/eventbus/dispatcher.go`, `cmd/c4/mcp.go`
- **DoD**:
  - Goal: rule action_type hub_submit 시 action_config로 Hub에 작업 제출 (템플릿 치환).
  - Rationale: REQ-A3, REQ-A4.
  - ContractSpec: Dispatcher에 HubSubmitter 인터페이스 및 SetHubSubmitter. executeRule에 case "hub_submit" → executeHubSubmit. executeHubSubmit: action_config JSON 파싱, c1_post와 동일한 템플릿 치환({{event_type}}, {{task_id}}, {{title}}, {{workdir}}, {{job_id}} 등), HubSubmitter.SubmitJob 호출. submitter nil이면 에러 반환(로그). ReplayRule에도 hub_submit 처리. mcp.go: embeddedEB.Dispatcher().SetHubSubmitter(hubSubmitterWrapper), hub.enabled일 때만 wrapper 주입.
  - CodePlacement: dispatcher.go 구조체/SetHubSubmitter/executeRule/executeHubSubmit/ReplayRule; mcp.go 405 근처.
- **Dependencies**: T-005-0 (Hub client wiring)
- **mode**: worker
- **domain**: implementation

---

## 5. Validation Strategy

- **Build**: `cd c4-core && go build ./... && go vet ./...`
- **Unit**: `go test ./internal/eventbus/ ./internal/mcp/handlers/ -count=1 -run EventBus|Hub|Knowledge|Research|Soul|Persona|task.updated`
- **Smoke**: Publish event via MCP, confirm rule dispatch (log or c1_post); hub_submit rule add + trigger event, confirm job submitted (or mock).

---

## 6. Checkpoints

- **CP-1**: B 완료 (T-001..T-004) — 모든 누락 이벤트 발행 구현 완료, 빌드/테스트 통과.
- **CP-2**: A 완료 (T-005, T-006) — Hub↔EventBus 양방향 연동 구현 완료, 빌드/테스트 통과.

---

## 7. Next Steps (사용자 실행)

1. **태스크 생성** (C4 MCP 연결 시):
   - `c4_add_todo`로 T-001-0 ~ T-006-0 위 정의대로 추가 (dependency: T-006-0 depends on T-005-0).
2. **실행**: `/c4-run` (Worker 스폰).
3. **정제**: `/c4-refine` (품질 게이트 통과까지).
4. **마무리**: `/c4-finish` (빌드·테스트·설치·문서·커밋).
5. **이후**: C(Edge/Deploy) 별도 plan 수립.

---

## 8. Task Payloads (c4_add_todo 참고)

아래는 MCP `c4_add_todo` 호출 시 사용할 수 있는 요약이다. title, scope, dod, dependencies, mode, domain 만 기재.

```yaml
# T-001-0
title: "task.updated 이벤트 발행 (sqlite_store, store_review)"
scope: "c4-core/internal/mcp/handlers/sqlite_store.go, store_review.go"
dod: "reassignStaleOrFindPendingTask, ClaimTask, completeReviewTask 내 UPDATE 후 notifyEventBus(task.updated, {task_id, status, previous_status, worker_id}). 빌드 및 관련 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-002-0
title: "knowledge.recorded/searched 이벤트 발행 (knowledge_native)"
scope: "c4-core/internal/mcp/handlers/knowledge_native.go, cmd/c4/mcp.go"
dod: "KnowledgeNativeOpts에 EventPub 추가. record/search/experiment 성공 후 해당 이벤트 발행. mcp.go에서 EventPub 주입. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-003-0
title: "research.started/recorded 이벤트 발행 (research_native)"
scope: "c4-core/internal/mcp/handlers/research_native.go, cmd/c4/mcp.go"
dod: "Research 핸들러에 EventPub 주입. start/record 성공 후 이벤트 발행. mcp.go 연동. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-004-0
title: "soul.updated, persona.evolved 이벤트 발행"
scope: "c4-core/internal/mcp/handlers/soul.go, persona.go, cmd/c4/mcp.go"
dod: "RegisterSoulHandlers에 EventPub 추가; setSoulSection 후 soul.updated. persona_evolve 후 persona.evolved. mcp.go 주입. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-005-0
title: "Hub→EventBus 이벤트 발행 (hub_jobs, worker_standby)"
scope: "c4-core/internal/mcp/handlers/hub_jobs.go, worker_standby.go, hub.go, cmd/c4/mcp.go"
dod: "SetHubEventBus 패턴. hub.job.submitted/cancelled (hub_jobs). hub.job.completed/failed, hub.worker.registered (worker_standby). mcp.go wire. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-006-0
title: "EventBus→Hub hub_submit 액션 (Dispatcher)"
scope: "c4-core/internal/eventbus/dispatcher.go, cmd/c4/mcp.go"
dod: "HubSubmitter 인터페이스, executeHubSubmit(템플릿 치환), executeRule/ReplayRule에 hub_submit. mcp.go SetHubSubmitter. 빌드 및 테스트 통과."
dependencies: ["T-005-0"]
mode: worker
domain: implementation
```

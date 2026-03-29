# ~~Plan: C — Edge/Deploy 실행 계층~~ (DEPRECATED)

> **DEPRECATED 2026-03-24**: Edge 개념이 Worker로 통합됨. 상세: `.c4/ideas/edge-worker-unification.md`
>
> ~~**범위**: C5 Hub의 Edge/Deploy에서 **룰 트리거 실행**, **배포 실행(아티팩트 전달)**, **엣지 에이전트**까지 구현.~~
> ~~B+A 완료 후 진행. run → refine → finish 동일.~~

---

## 1. Feature Overview

| 구분 | 내용 |
|------|------|
| **Feature** | c5-edge-deploy-execution |
| **Domain** | backend (C5 Hub server, C5 store, edge agent) |
| **Goal** | Deploy Rule이 Job/DAG 완료 시 자동 평가·배포 생성, pending 배포를 엣지로 전달, 엣지 에이전트가 수신·다운로드·상태 보고 |

---

## 2. Discovery (EARS Requirements)

### C-1. 룰 트리거 실행

| ID | Pattern | Text |
|----|---------|------|
| REQ-C1 | Event-Driven | When a Hub job completes (SUCCEEDED), the system shall evaluate all enabled deploy rules whose trigger matches the job (e.g. job_tag:*, job_id) and create a deployment for each matching rule with edges from MatchEdges(edge_filter). |
| REQ-C2 | Event-Driven | When a DAG completes (all nodes done), the system shall evaluate deploy rules whose trigger matches the DAG (e.g. dag_complete:pipeline-*) and create deployments for matching rules. |
| REQ-C3 | State-Driven | Trigger expression shall support at least: `job_tag:<tag>`, `job_id:<id>`, `dag_complete:<dag_id_pattern>`. Matching is string prefix or glob. |
| REQ-C4 | Unwanted | If no edges match the rule's edge_filter, the system shall not create a deployment (or create with 0 targets and mark skipped). |

### C-2. 배포 실행(아티팩트 전달)

| ID | Pattern | Text |
|----|---------|------|
| REQ-C5 | Event-Driven | When a deployment exists with status pending, the system shall make artifact metadata (job_id, artifact paths matching artifact_pattern, presigned URLs or download API) available to edges assigned in deploy_targets. |
| REQ-C6 | State-Driven | Edge agents shall be able to poll or receive an assignment (e.g. GET /v1/deploy/assignments/{edge_id} or GET /v1/deploy/pending) to get deploy_id, job_id, artifact list, and download URLs. |
| REQ-C7 | Event-Driven | When an edge finishes downloading and optional post_command, it shall report status (succeeded/failed) via API so that deploy_targets and deployment status are updated. |

### C-3. 엣지 에이전트

| ID | Pattern | Text |
|----|---------|------|
| REQ-C8 | Ubiquitous | The system shall provide an edge agent (CLI or small daemon) that: registers the edge with the Hub, sends heartbeats, polls for pending deployments for this edge, downloads artifacts to a local directory, optionally runs post_command, and reports success/failure. |
| REQ-C9 | Optional | If post_command is configured in the rule or trigger request, the edge agent shall execute it after downloading artifacts; non-zero exit shall be reported as failed. |

---

## 3. Design

### 3.1 Current State (No Code Assumption)

- **C5** (`c5/`): Edge CRUD, heartbeat, deploy rules CRUD, deploy trigger (manual), deploy status, deploy_targets, UpdateDeployTarget, checkDeploymentComplete. **Missing**: rule evaluation on job/DAG complete, API for edges to fetch assignments and artifact URLs, edge agent binary.
- **c4-core** Hub client: edge/deploy APIs already present; missing MCP handlers only for edge_heartbeat, edge_remove, deploy_rule_list, deploy_rule_delete, deploy_list (optional).

### 3.2 Architecture Options

| Option | Description | Complexity |
|--------|-------------|------------|
| **A** | Rule evaluation in C5 onJobComplete + new DAG-complete hook; new C5 API GET /v1/deploy/assignments/{edge_id}; edge agent in c5/cmd as subcommand. | Medium |
| **B** | Rule evaluation in C5; edge agent as separate repo/tool calling C5 APIs only. | Low (split) |
| **C** | Rule evaluation and “deploy executor” in c4-core that calls Hub; edge agent in c4-core. | High (wrong place) |

**Selected**: Option A — all execution logic in C5; edge agent as `c5 edge-agent` or `c5 deploy-agent` subcommand in the same repo.

### 3.3 Components

| Component | Responsibility |
|-----------|----------------|
| **C5 Store** | Add MatchDeployRules(triggerExpr, jobID, jobTags, dagID) or equivalent; extend CreateDeployment to accept rule_id. |
| **C5 API (jobs)** | After CompleteJob, call evaluateDeployRulesForJob(jobID, status, jobTags); on match, create deployment (rule_id, job_id, edges from MatchEdges(rule.EdgeFilter)). |
| **C5 API (dags)** | When DAG finishes (in AdvanceDAG or onJobComplete when last node completes), call evaluateDeployRulesForDAG(dagID); create deployments for matching rules. |
| **C5 API (deploy)** | New endpoint GET /v1/deploy/assignments/{edge_id} returning list of pending deploy_targets for this edge with deploy_id, job_id, artifact_pattern, post_command, and artifact list (paths + presigned URLs or /v1/artifacts/{job_id}/download?path=...). |
| **C5 Storage/Artifacts** | Use existing ListArtifacts(jobID), presigned URL or download API so edge can fetch files. |
| **Edge agent** | Register edge, heartbeat loop, poll GET /v1/deploy/assignments/{edge_id}, for each assignment: download artifacts to local dir, run post_command, call POST /v1/deploy/target-status (or existing UpdateDeployTarget API) to report succeeded/failed. |

### 3.4 Data Flow

```
Job completes (or DAG completes)
  → evaluateDeployRules(triggerContext)
  → For each matching rule: MatchEdges(rule.EdgeFilter) → CreateDeployment(rule_id, job_id, edges)
  → deployment + deploy_targets created (pending)

Edge agent (periodic poll)
  → GET /v1/deploy/assignments/{edge_id}
  → Response: [{ deploy_id, job_id, artifact_pattern, post_command, artifacts: [{ path, url }] }]
  → For each: download artifacts → run post_command → POST deploy target status (succeeded/failed)
  → C5 UpdateDeployTarget(deploy_id, edge_id, status, errMsg) → checkDeploymentComplete
```

### 3.5 Decisions

| ID | Question | Decision | Rationale |
|----|----------|----------|------------|
| DEC-C1 | Where to evaluate rules? | C5 server only (onJobComplete + DAG completion path). | Single source of truth; no c4-core dependency. |
| DEC-C2 | Trigger expression format | Simple prefix/glob: job_tag:X, job_id:J-123, dag_complete:dag-*. Parser in C5 store or api. | Keeps MVP small; no full expression engine. |
| DEC-C3 | How do edges get artifact files? | C5 exposes GET /v1/artifacts/{job_id}/list and GET /v1/artifacts/{job_id}/download?path=... (or presigned URL from existing storage). | Reuse existing artifact storage (local or Supabase). |
| DEC-C4 | Edge agent location | c5/cmd/c5: new subcommand `edge-agent` or `deploy-agent`. | Same binary, same config (Hub URL, API key). |

---

## 4. Task Breakdown

### 4.1 Dependency Graph

```
T-C01 (rule evaluation job) ──┐
T-C02 (rule evaluation DAG) ──┼──► T-C03 (assignments API) ──► T-C04 (edge agent)
```

- **T-C01**: Job 완료 시 룰 평가 및 배포 생성 (C5).
- **T-C02**: DAG 완료 시 룰 평가 및 배포 생성 (C5).
- **T-C03**: GET /v1/deploy/assignments/{edge_id} 및 아티팩트 다운로드 URL 제공 (C5).
- **T-C04**: Edge agent CLI (폴링, 다운로드, post_command, 상태 보고).

### 4.2 Task Definitions (for c4_add_todo)

---

#### T-C01-0: Job 완료 시 Deploy Rule 평가 및 배포 생성

- **Title**: C5 Job 완료 시 Deploy Rule 자동 평가·배포 생성
- **Scope**: `c5/internal/api/jobs.go`, `c5/internal/store/sqlite.go`, `c5/internal/model/model.go`
- **DoD**:
  - Goal: Job이 SUCCEEDED로 완료될 때 enabled deploy rules 중 trigger가 해당 job과 매칭되는 규칙에 대해 MatchEdges(edge_filter)로 엣지 목록을 구하고, CreateDeployment(rule_id, job_id, edges)로 배포 생성.
  - Rationale: REQ-C1, REQ-C3, DEC-C1.
  - ContractSpec: Trigger 표현식 파서: `job_tag:<tag>` (job tags에 tag 포함), `job_id:<id>` (완전 일치 또는 prefix). Store에 ListDeployRulesEnabled, MatchRuleTrigger(jobID, jobTags, rule) 또는 EvaluateRulesForJob(jobID, status, jobTags) 반환 규칙 목록. CreateDeployment 시 rule_id 전달 가능하도록 시그니처/INSERT 확장. onJobComplete(또는 CompleteJob 핸들러 내)에서 status==SUCCEEDED일 때만 평가 호출.
  - CodePlacement: store: EvaluateDeployRulesForJob; CreateDeployment에 rule_id 추가. jobs.go: handleJobComplete 후 평가 호출.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-C02-0: DAG 완료 시 Deploy Rule 평가 및 배포 생성

- **Title**: C5 DAG 완료 시 Deploy Rule 자동 평가·배포 생성
- **Scope**: `c5/internal/api/dags.go`, `c5/internal/store/sqlite.go`
- **DoD**:
  - Goal: DAG가 완료(모든 노드 처리)될 때 deploy rules 중 trigger가 `dag_complete:<pattern>` 형태로 해당 DAG와 매칭되는 규칙에 대해 배포 생성.
  - Rationale: REQ-C2, DEC-C1.
  - ContractSpec: DAG 완료 감지: AdvanceDAG 후 또는 기존 로직에서 “DAG status가 completed/failed로 바뀐 시점”에서 EvaluateDeployRulesForDAG(dagID) 호출. Trigger 표현식에 `dag_complete:<dag_id_pattern>` 지원 (prefix/glob). CreateDeployment 시 job_id는 DAG의 “대표 job” 또는 마지막 job 등 정책 결정 후 전달(또는 rule에 따라 다를 수 있음 — MVP는 DAG ID로 아티팩트를 묶거나 대표 job_id 사용).
  - CodePlacement: store: EvaluateDeployRulesForDAG; dags.go: DAG 완료 분기에서 호출.
- **Dependencies**: T-C01-0 (trigger parsing and CreateDeployment with rule_id shared)
- **mode**: worker
- **domain**: implementation

---

#### T-C03-0: 엣지 배포 할당 API 및 아티팩트 URL 제공

- **Title**: C5 GET /v1/deploy/assignments/{edge_id} 및 아티팩트 다운로드 URL
- **Scope**: `c5/internal/api/edges.go` (또는 deploy.go), `c5/internal/store/sqlite.go`, `c5/internal/api/storage.go`(있을 경우)
- **DoD**:
  - Goal: Edge가 GET /v1/deploy/assignments/{edge_id} 호출 시 해당 edge_id에 대한 pending deploy_targets 목록을 반환. 각 항목에 deploy_id, job_id, artifact_pattern, post_command, 및 artifact 목록(path + download URL) 포함.
  - Rationale: REQ-C5, REQ-C6, DEC-C3.
  - ContractSpec: Store: ListPendingDeployTargetsForEdge(edgeID). API: GET /v1/deploy/assignments/{edge_id} → JSON. 아티팩트 목록은 ListArtifacts(job_id) + artifact_pattern glob 필터. 다운로드 URL은 기존 presigned URL API 또는 GET /v1/artifacts/{job_id}/download?path=... 형태. UpdateDeployTarget 호출은 기존 API 사용(edge가 상태 보고).
  - CodePlacement: store: ListPendingDeployTargetsForEdge; edges.go 또는 새 deploy.go: handleDeployAssignments.
- **Dependencies**: (none)
- **mode**: worker
- **domain**: implementation

---

#### T-C04-0: Edge Agent CLI (등록·하트비트·폴링·다운로드·상태 보고)

- **Title**: C5 Edge Agent 서브커맨드 (등록, 하트비트, 배포 폴링, 아티팩트 다운로드, post_command, 상태 보고)
- **Scope**: `c5/cmd/c5/`, 새 패키지 `c5/internal/edgeagent/` (선택)
- **DoD**:
  - Goal: `c5 edge-agent` (또는 `deploy-agent`) 실행 시 Hub에 엣지 등록, 주기적 하트비트, GET /v1/deploy/assignments/{edge_id} 폴링, 할당된 배포에 대해 아티팩트 다운로드, post_command 실행, UpdateDeployTarget(succeeded/failed) 호출.
  - Rationale: REQ-C8, REQ-C9, DEC-C4.
  - ContractSpec: Cobra 서브커맨드. 플래그: --hub-url, --api-key, --edge-name, --workdir (다운로드 경로), --poll-interval. 등록 실패 시 재시도. 배포 처리 시 deploy_targets 상태를 downloading → deploying(또는 running) → succeeded/failed로 업데이트. post_command 실패 시 failed 보고.
  - CodePlacement: cmd/c5/root.go에 서브커맨드 추가; edgeagent 패키지에 Run(ctx, config) 및 다운로드/실행 로직.
- **Dependencies**: T-C03-0
- **mode**: worker
- **domain**: implementation

---

## 5. Validation Strategy

- **Build**: `cd c5 && go build ./... && go vet ./...`
- **Unit**: `go test ./internal/store/ ./internal/api/ -count=1 -run Deploy|Edge|Rule`
- **Integration**: API 테스트에서 job 완료 후 규칙 매칭 시 배포 생성, GET assignments 반환 값 검증.

---

## 6. Checkpoints

- **CP-C1**: T-C01, T-C02 완료 — Job/DAG 완료 시 룰 평가 및 배포 생성 동작, 빌드/테스트 통과.
- **CP-C2**: T-C03, T-C04 완료 — Assignments API 및 Edge Agent 동작, 빌드/테스트 통과.

---

## 7. Next Steps (사용자 실행)

1. **태스크 생성**: `c4_add_todo`로 T-C01-0 ~ T-C04-0 추가 (T-C02 depends on T-C01, T-C04 depends on T-C03).
2. **실행**: `/run`
3. **정제**: `/refine`
4. **마무리**: `/finish`

---

## 8. Task Payloads (c4_add_todo 참고)

```yaml
# T-C01-0
title: "C5 Job 완료 시 Deploy Rule 평가 및 배포 생성"
scope: "c5/internal/api/jobs.go, c5/internal/store/sqlite.go, c5/internal/model/model.go"
dod: "Job SUCCEEDED 시 enabled deploy rules 중 trigger 매칭 규칙에 대해 MatchEdges 후 CreateDeployment(rule_id, job_id, edges). Trigger 파서 job_tag/job_id. Store EvaluateDeployRulesForJob, CreateDeployment에 rule_id 추가. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-C02-0
title: "C5 DAG 완료 시 Deploy Rule 평가 및 배포 생성"
scope: "c5/internal/api/dags.go, c5/internal/store/sqlite.go"
dod: "DAG 완료 시점에서 EvaluateDeployRulesForDAG(dagID) 호출. trigger `dag_complete:<pattern>` 지원. 매칭 규칙에 대해 배포 생성. 빌드 및 테스트 통과."
dependencies: ["T-C01-0"]
mode: worker
domain: implementation

# T-C03-0
title: "C5 GET /v1/deploy/assignments/{edge_id} 및 아티팩트 URL"
scope: "c5/internal/api/edges.go or deploy, c5/internal/store/sqlite.go"
dod: "ListPendingDeployTargetsForEdge(edgeID). GET /v1/deploy/assignments/{edge_id} 응답에 deploy_id, job_id, artifact_pattern, post_command, artifacts(path+url). 기존 ListArtifacts·presigned URL 활용. 빌드 및 테스트 통과."
dependencies: []
mode: worker
domain: implementation

# T-C04-0
title: "C5 Edge Agent 서브커맨드 (등록·하트비트·폴링·다운로드·상태)"
scope: "c5/cmd/c5/, c5/internal/edgeagent/"
dod: "c5 edge-agent 서브커맨드. 등록·하트비트·GET assignments 폴링·아티팩트 다운로드·post_command 실행·UpdateDeployTarget 호출. 빌드 및 테스트 통과."
dependencies: ["T-C03-0"]
mode: worker
domain: implementation
```

---

## 9. Optional (Phase 2)

- **MCP 보강**: c4-core에 `c4_hub_edge_heartbeat`, `c4_hub_deploy_rule_list`, `c4_hub_deploy_list` 등 누락된 Hub 엣지/배포 도구 추가 (S).
- **배포 후 검증**: 엣지에서 post_command 후 헬스체크 호출 또는 간단 검증 (M).
- **롤백**: 배포 버전 추적 및 이전 버전 복구 API (M).

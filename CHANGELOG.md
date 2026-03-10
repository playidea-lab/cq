# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.94.2] - 2026-03-10

### 🐛 Bug Fixes
- **doctor**: hooks fix 후 OK 상태 반영 + .mcp.json --fix 시 자동 생성 (install 후 WARN 0개)
- **edge-agent**: collect 업로드를 Supabase Storage 직접 경유로 수정

---

## [v0.94.0] - 2026-03-10

### ✨ Features
- **hub**: `cq hub edge control <edge-id> <action>` 서브커맨드 추가 (collect/restart/stop/update)
- **hub**: `newHubClient` JWT fallback — `cq auth login` 후 hub list/control 명령 자동 인증

### 🐛 Bug Fixes
- **hub-edge**: `cq hub edge start` JWT + builtinHubURL fallback 추가 — `cq hub worker start`와 동일하게 `cq auth login`만으로 동작

---

## [v0.93.0] - 2026-03-10

### ✨ Features
- **cli**: `cq hub edge init/start/install` — edge agent를 worker start와 동일한 UX로 시작. `~/.c5/edge.yaml` 설정, systemd/launchd 서비스 자동 생성
- **c5/store**: `MarkStaleWorkers` — `busy` 상태 zombie 워커도 stale 처리 (heartbeat 미전송 시)

### 🔒 Security
- **hub-edge**: `DriveAPIKey`를 CLI 플래그 대신 `C5_DRIVE_API_KEY` env var로 전달 (ps 노출 방지)
- **hub-edge**: systemd `Environment=` / launchd `EnvironmentVariables` 블록에 `C5_DRIVE_API_KEY` 포함

### 🧪 Tests
- `hub_edge_start_test.go` — 8개 테스트 (init 멱등성, 플래그 검증, dry-run, Cmd.Env 캡처, env auto-init)
- `TestMarkStaleWorkersBusy` — zombie busy worker GC 검증

---

## [v0.91.0] - 2026-03-10

### ✨ Features
- **c5/hub**: Zombie Worker GC — 24h offline 워커를 `worker_history`로 아카이브 후 자동 삭제 (lease expiry 루프, 1h rate-limit)
- **c5/api**: `POST /v1/workers/prune` endpoint (dry_run 지원)
- **c5/worker**: Capability 3-tier fallback chain (file → caps.yaml Command → C5_PARAMS.command)
- **cli**: `cq hub workers prune [--dry-run]` 좀비 워커 정리 명령
- **cli**: `cq hub workers` 기본 active-only 표시 (`--all` 플래그로 전체)

### 🧪 Tests
- `TestPurgeStaleWorkers` + `TestPurgeStaleWorkersTransaction` (store 원자성 검증)

---

## [v0.90.0] - 2026-03-10

### ✨ Features
- **gpu-worker**: one-command setup script for GPU servers (`docs/gpu-worker/setup.sh`)

### 🔧 Chores
- **worker**: JWT retry guard (maxRetries=3), io.LimitReader on refresh response, session.json field preservation during token refresh
- **archtest**: hub_worker.go allowlist 4→7

---

## [v0.89.0] - 2026-03-10

### ✨ Features
- **c5/auth**: Scoped API keys — `sk-user-*`/`sk-worker-*` prefix enforcement with endpoint-level scope checking
- **c5/worker**: 1-tier container mode (`C5_CONTAINER_MODE=1`) — worker runs jobs as subprocess, no docker-in-docker
- **c5/worker**: `c5 worker install` — Docker pull + container run with GPU auto-detection (nvidia-smi)
- **c5**: GPU worker Dockerfile (multi-stage: Go alpine builder + PyTorch 2.5 CUDA 12.4 runtime)

### 🐛 Bug Fixes
- **c5/auth**: scope escalation fix — DB scope "full" no longer overridden by key prefix

### 📚 Documentation
- Docker Worker 1-tier 모델 + Scoped API Key 문서 업데이트 (AGENTS.md, docs/gpu-worker/)

---

## [v0.85.1] - 2026-03-09

### 🐛 Bug Fixes
- **env**: CI_HUB_URL/CI_API_KEY → C5_HUB_URL/C5_API_KEY 통일 — `.gitlab-ci.yml`, `auth.go`, `hub.go`, `mcp_init_hub.go` 환경변수명 일관성 수정

---

## [v0.85.0] - 2026-03-09

### ✨ Features
- **worker-ux**: 3-layer Worker UX — onboarding script (gpu-setup.sh), observability (cq hub log --follow), routing (capabilities auto-detect)
- **c5/model**: HeartbeatRequest/Worker/JobSubmitRequest 필드 확장 (GPU Worker 지원)
- **gpu-worker**: E2E smoke test script + SETUP.md 가이드
- **hub_worker**: `cq hub worker start` — config 없으면 env에서 auto-init

### 🐛 Bug Fixes
- **hub-worker**: 3 E2E review bugs — systemd env, upgrade exit code, auto non-interactive
- **hub_worker**: systemd unit file injection 방어 + 파일 권한 분기
- **c5/worker**: workdir 없으면 현재 디렉토리 fallback
- **edge**: code review 4 HIGH + 6 MEDIUM issues resolved
- **build**: serve_loop_orchestrator_stub.go for non-research builds
- **docs**: piqsol.com → GitHub 설치 URL 수정
- **archtest**: update errorf ratchet for hub_edge.go + hub_worker.go
- **skills**: c4-plan c4_notify → mcp__cq__c4_notify 파라미터 수정

### 🔧 Polish
- **worker-ux**: round 2-5 보안·안정성 수정
- **smoke_test**: $NF + SETUP.md output 동기화 + 테스트 명확화
- **launchd**: plist에 C5_API_KEY 주입 + 테스트 wantNot 제거
- **hub_worker**: systemd 보안 강화

### 📚 Documentation
- **agents**: Go 테스트 카운트 업데이트 (~2,835)
- **gpu-worker**: c5 worker UX code review with CI var mapping issue

---

## [v0.82.0] - 2026-03-08

### ✨ Features
- **c5/efl**: Hub Edge Feedback API Foundation — POST /edges/{id}/metrics, GET /edges/{id}/control (auto-ack), HealthCheck struct, DeployAssignmentResponse 업그레이드
- **c5/edge**: MetricsReporter (stdout KEY=VALUE→Hub 60s), ControlPoller (GET 30s auto-ack), HealthCheckGate, RollbackManager (atomic temp-rename .prev/.failed 전략)
- **c5/threshold**: Edge 메트릭 임계값 모니터링 — edge metadata `threshold_<key>` 기반, 60s cooldown, EventBus `edge.metrics.threshold_exceeded` 발행
- **hub-edge**: `c4_hub_edge_control`, `c4_hub_edge_metrics` MCP 도구 추가
- **hub-edge**: `c4_hub_deploy_rule` — `health_check`, `health_check_timeout` 파라미터 노출
- **c5/worker**: GPU 자동 감지 + caps.yaml 자동 생성
- **notify**: 이벤트 필터링 + Teams 알림 테스트 추가

### 🐛 Bug Fixes
- **c5/edge**: control.go collect — `filepath.Clean(..)` path traversal 방어
- **c5/edge**: Drive 업로드 URL — `url.Values.Encode()` (인젝션 방지)
- **c5/api**: MaxBytesReader 64KB 제한 + edgeIDRe regexp 검증
- **c5/api**: tryAcquireCooldown race — sync.Map → sync.Mutex+map (비원자 Load+Store 제거)
- **c5/models**: EdgeMetricEntry.Values `map[string]float64` (hub.Client와 타입 일치)
- **c1**: 채널 UI 이슈 수정 4건
- **skills**: c4_notify MCP namespace 파라미터 수정

### 📚 Documentation
- **agents**: Go 테스트 카운트 업데이트 — c4-core ~2,082 + c5 ~334 = ~2,416

---

## [v0.78.0] - 2026-03-08

### ✨ Features
- **research**: `TypeDebate` knowledge type — `DocumentType="debate"`, prefix "deb", frontmatter `hypothesis_id`/`trigger_reason`/`verdict`
- **research**: `c4_research_spec` MCP tool — ExperimentSpec 생성 (hypothesis → DoD: success_condition, null_condition, escalation_trigger, controlled_variables)
- **research**: `c4_research_checkpoint` MCP tool — LLM-Optimizer + LLM-Skeptic 2-agent DoD 검토, conservative verdict (either NEGATIVE → revision_requested)
- **research**: `c4_research_debate` MCP tool — Optimizer→Skeptic→Synthesis 3-phase debate flow, TypeDebate 지식 문서 자동 기록, next_hypothesis_draft 생성
- **serve**: `AnomalyMonitor` component — TypeExperiment frontmatter `expected_metrics_range` JSON 폴링, 24h dedup watermark, 이상 감지 시 TypeDebate 에스컬레이션 자동 생성
- **cli**: `cq research spec/checkpoint/debate` 서브커맨드 — Level 3 연구 루프 CLI 진입점

### 🐛 Bug Fixes
- **research**: debate verdict 오프셋 버그 수정 — `strings.ToUpper` 이후 byte 오프셋 불일치 → 원본 문자열에서 직접 탐색
- **research**: synth verdict JSON 우선 파싱 → skeptic 텍스트 fallback
- **research**: optimizer NEGATIVE 포함 시 conservative verdict 적용
- **serve**: AnomalyMonitor `Stop()` nil guard — `Start()` 전 `Stop()` 호출 시 deadlock 방지
- **serve**: path traversal 방어 강화 — `"/"` + `".."` 동시 검사
- **serve**: TypeDebate 메타 키 통일 — `hypothesis_id` (research_debate.go 계약 준수)
- **serve**: lastEscalation 타임스탬프 업데이트를 Store.Create 성공 후로 이동

### 🔧 Chores
- **archtest**: research Level 3 파일 fmt.Errorf allowlist 추가 (`research_checkpoint.go:2`, `research_debate.go:2`, `research_spec.go:3`, `research_level3.go:1`)
- **.gitignore**: `c5/c5` 바이너리 추가

### 📚 Documentation
- **agents**: Go 테스트 수 업데이트 (~2,021, Level 3 research 21 tests 추가)

---

## [v0.77.1] - 2026-03-08

### ✨ Features
- **notify**: `c4_notification_set`, `c4_notification_get`, `c4_notify` MCP tools — per-user workflow alerts via Dooray/Slack/Discord/Teams webhook
  - `c4_notification_set`: webhook URL 등록 (channel + webhook_url)
  - `c4_notification_get`: 설정된 채널 조회 (webhook_url 마스킹)
  - `c4_notify`: 등록된 채널에 메시지 발송 (title + message)
  - Teams MessageCard `@context` 필드 포함, `errors.Is` 모던 Go 패턴 적용
  - `.c4/notifications.json`에 0o640 권한으로 저장

### 🐛 Bug Fixes
- **serve**: `registerHubPollerServeComponent`에 `initCtx.hubClient` 전달 — credentials 재해결 없이 재사용
- **serve/hub**: hubpoller stub 시그니처를 any parameter 매칭으로 업데이트

---

## [v0.76.0] - 2026-03-08

### ✨ Features
- **serve**: HypothesisSuggester component — LLM gateway injection for experiment analysis
- **cli**: `cq suggest list/approve` — human approval gate for TypeHypothesis
- **knowledge**: `c4_research_suggest` MCP tool — on-demand LLM analysis of experiments
- **knowledge/hypothesis**: shared analyzer package for LLM-based experiment analysis

### 🐛 Bug Fixes
- **suggest-poller**: restore T-1196-0 canonical knowledgeSuggestPoller impl + fix LLMCaller interface alignment
- **research-suggest**: rename LLMCaller type to avoid collision, rune-safe truncation, actual experiment count

### 🔧 Chores
- **archtest**: add Level 2 suggest files to allowlist
- **agents**: update Go test count for Level 2 research loop additions

---

## [v0.75.0] - 2026-03-08

### ✨ Features
- **c4-plan**: Phase 2.65 Conflict Gate 추가 — Design Complete 후, 태스크 생성 전 활성 워커/기존 스펙/지식과의 충돌을 감지하는 소프트 게이트
  - 파일 충돌(HIGH), 개념 겹침(MEDIUM), 지식 참고(LOW) 3단계 분류
  - 충돌 없으면 절대 중단 없음, 사용자는 항상 무시하고 진행 가능 (하드 블로킹 금지)
  - 무시 결정 시 `c4_knowledge_record`로 이력 기록

---

## [v0.74.1] - 2026-03-08

### 🐛 Bug Fixes
- **hub_poller**: `knowledgeHubPollerConfig`에 `APIKeyEnv` 미전달로 `C4_HUB_API_KEY` env var가 무시되어 원격 C5 Hub에 401 반환되던 버그 수정

---

## [v0.74.0] - 2026-03-08

### ✨ Features
- **c5/mcp**: 잡 완료 대기를 2초 ticker 폴링에서 `sync.Map` completion channel로 교체 — 거의 제로 레이턴시 응답
  - `completionHub sync.Map[jobID → chan struct{}]` — `handleWorkerComplete` 훅 연동
  - `LoadAndDelete` 원자적 삭제로 double-close panic 방지
  - Race 보상: `notifyJobAvailable()` 호출 전에 채널 Store → Post-Store terminal 상태 체크
  - 5분 컨텍스트 타임아웃, `DeadlineExceeded` 시 MCP 에러 응답 (기존: 행잠)
- **gpu-worker**: GPU 워커용 스크립트 및 capability 선언 추가 (`docs/gpu-worker/`)
  - `gpu-caps.yaml`, `gpu-status.sh`, `gpu-train.sh`, `gpu-infer.sh` — path traversal guard, OOM 방지, shell injection 방지
- **scripts**: GPU Hub E2E smoke test (`scripts/test-gpu-e2e.sh`) — `--dry-run` 지원

### 🐛 Bug Fixes
- **docs**: GPU onboarding C5 워커 CLI 플래그 오류 수정 (`--caps`→`--capabilities`, `--hub`→`--server`, `C5_HUB_API_KEY`→`C5_API_KEY`)

### 📚 Documentation
- **agents**: C5 Hub 섹션에 `### GPU Worker 연결` 온보딩 가이드 추가

### 🔧 Chores
- **polish**: completion channel race fix + shell injection (round-1)
- **polish**: path guard + OOM fix (round-2)

---

## [v0.73.0] - 2026-03-08

### ✨ Features
- **serve**: `knowledgeHubPoller` serve.Component 추가 — CQ Research Loop Level 1
  - Hub 완료 잡 30초 주기 폴링 (`cfg.Hub.Enabled && cfg.Hub.URL` 조건 시 자동 등록)
  - stdout KEY=VALUE 파싱 → `knowledge.Create(TypeExperiment)` 자동 기록
  - `seenIDs` JSON 파일 원자적 저장 + 30일 TTL cleanup (`cleanupSeenIDs`)
  - `hub.Client` 1회 생성 후 재사용 (per-poll allocation 제거)
  - `sort.Strings(parts)` — knowledge body 결정적 재현성 보장
  - `C1Notifier` optional nil-safe interface — 잡 기록 시 C1 Messenger 알림
  - `hubPollerLogLimit=1000` 상수, HasMore 페이지네이션 스킵(의도적)

### 🐛 Bug Fixes
- **docs**: CLAUDE.md MCP HTTP transport type `url` → `http` 수정 (Claude Code 2.1.x 스키마)
- **serve**: `os.MkdirAll` errcheck nolint 제거 → 에러 로그 + component 등록 중단
- **hub/client**: `ListJobsCtx` limit 파라미터 추가 (≤0 = 서버 기본값)

---

## [v0.72.1] - 2026-03-07

### ✨ Features
- **cli**: `cq config get/set` 명령 추가 — `.c4/config.yaml` 값을 dot-notation으로 조회·설정 (`serve.mcp_http.enabled`, `serve.mcp_http.bind` 등)
- **cli**: `cq secret set <key> [value]` — 인라인 값 직접 전달 지원 (기존: 프롬프트만 가능); shell history 경고 출력
- **hub/client**: `ListJobsCtx` + `GetJobLogsCtx` context-aware API 메서드 추가
- **hub**: `cq.yaml`의 `experiment:` 섹션 파싱 → `JobSubmitRequest` 매핑 (C9 루프 연동)

---

## [v0.72.0] - 2026-03-07

### ✨ Features
- **serve**: HTTP MCP 엔드포인트(`mcpHTTPComponent`) 추가 — `cq serve`를 원격 MCP 서버로 노출
  - `POST /mcp`: JSON-RPC 2.0 핸들러 (`handleRequestWithCtx` 재사용)
  - `GET /mcp`: SSE keepalive (15초 간격)
  - API key 인증: `X-API-Key` 헤더 / `Authorization: Bearer` (constant-time compare)
  - key 우선순위: `secrets.db → CQ_MCP_API_KEY env → config.yaml` (dev fallback)
  - 빈 key 시 컴포넌트 시작 거부 (실수 방지)
  - 요청 크기 1MB 제한, write deadline 65s
  - `newMCPServer()` 를 `runServe()` 레벨로 끌어올려 UDS(tool-socket)과 HTTP 두 transport 공유
- **config**: `ServeMCPHTTPConfig` 추가 (`serve.mcp_http.*`, 기본 port=4142, bind=127.0.0.1)

---

## [v0.71.3] - 2026-03-07

### ✨ Features
- **bridge**: c4-bridge Python 사이드카 소스를 public 레포(PlayIdea-Lab/cq)에 공개 — `uv tool install git+https://github.com/PlayIdea-Lab/cq`로 설치 가능

### 🐛 Bug Fixes
- **doctor**: `cq doctor --fix` 실행 시 c4-bridge 자동 설치 (`uv tool install git+https://github.com/PlayIdea-Lab/cq`)
- **doctor**: `checkPythonSidecar` Fix 힌트 → `cq doctor --fix` (이전: `uv tool install c4` — 잘못된 PyPI 패키지)
- **install**: `install.sh` c4-bridge 설치 URL 수정 (git+https 직접 사용)

---

## [v0.71.3] - 2026-03-07

### ✨ Features
- **c5/worker**: `uv: true` (default) — cq.yaml의 `run:` 명령 앞에 `uv run` 자동 prefix → pyproject.toml 기반 의존성 자동 설치
- **c5/worker**: `uv: false` 명시 시 명령어 그대로 실행 (bash 스크립트 등)

---

## [v0.71.2] - 2026-03-07

### 🐛 Bug Fixes
- **c5/worker**: `builtinServerURL` ldflags 지원 — CI `CI_HUB_URL` 변수로 Hub URL 바이너리 내장 (zero-config worker)
- **c5/edge-agent**: 동일한 `builtinServerURL` fallback 적용
- **ci**: c5 빌드 시 `-ldflags "-X main.builtinServerURL"` 주입

---

## [v0.71.1] - 2026-03-07

### 🐛 Bug Fixes
- **c5/model**: cherry-pick 충돌 마커 제거 — `GitHash` 필드 유지 (v0.71.0 빌드 실패 수정)

---

## [v0.71.0] - 2026-03-07

### ✨ Features
- **cloud**: 디렉토리명 기반 자동 project 감지 — `getActiveProjectIDWithProjects` (config.yaml `active_project` 불필요)
- **c5/model**: `Job`/`JobSubmitRequest`에 `SnapshotVersionHash` + `GitHash` 추가 — 재현성 향상

### 🐛 Bug Fixes
- **doctor**: `checkPythonSidecar` false-positive 수정 — `uv` PATH 존재 여부만 확인 → `uv run c4-bridge --version` 실행 확인 (5s 타임아웃)
- **install**: `install.sh`에 Python sidecar 설치 단계 추가 — `uv tool install c4` (PyPI) → git+https fallback, `--dry-run` 지원

---

## [v0.70.0] - 2026-03-07

### ✨ Features
- **c5/worker**: Stateless Worker — Hub 잡 payload의 `project_id`를 자식 프로세스 `C4_PROJECT_ID` env로 주입
- **c5**: Hub Version Gate — `C5_MIN_VERSION` env 기반 semver 비교, 구버전 워커에 `control: upgrade` 반환
- **c5/worker**: Control Message 처리 — `upgrade`(cq upgrade 후 재시작) / `shutdown`(루프 종료) 분기
- **c5/worker**: Hub 등록 시 `CQ_VERSION` 버전 보고
- **c5/model**: `WorkerRegisterRequest.Version` + `ControlMessage` 타입 추가
- **c5**: `workers` 테이블 `version` 컬럼 마이그레이션
- **cloud**: `getActiveProjectID()` — `C4_PROJECT_ID` env var 우선 지원 (config.yaml보다 선행)

### 🐛 Bug Fixes
- **c5**: conversation 패키지 merge conflict 해결 (Updated upstream 기준)

### 🔧 Chores
- **archtest**: `allowlist.go`에 upgrade.go + worker.go sentinel error 허용 항목 추가
- **docs**: C5 Hub 섹션에 Stateless Worker + Version Gate 운영 가이드 추가

---

## [Unreleased]

## [v0.67.0] - 2026-03-06

### ✨ Features
- **drive/dataset**: 버전된 데이터셋 스토리지 (CAS + Supabase) — `DatasetClient.Upload/Pull/List`
  - content-addressable storage (`{projectID}/cas/{hash[:2]}/{hash}`)
  - 증분 Upload (version_hash 비교 → 변경 시만 CAS 업로드)
  - 증분 Pull (로컬 파일 hash 비교 → 변경 파일만 다운로드)
  - streaming upload (io.ReadAll 미사용, 대용량 파일 안전)
- **drive/mcp**: `c4_drive_dataset_upload`, `c4_drive_dataset_list`, `c4_drive_dataset_pull` MCP 도구
- **drive/cli**: `cq drive dataset upload/list/pull` CLI 서브커맨드
- **infra**: migration 00031 — `c4_datasets` 테이블 + RLS (INSERT/SELECT: member, UPDATE/DELETE: service_role)
- **hub**: secrets store에서 `hub.api_key` 우선 조회

### 🐛 Bug Fixes
- **drive/dataset**: `hashByPath` map을 `sort.Slice` 전에 구축 — 정렬 후 인덱스 불일치 버그 수정
- **drive/dataset**: `casStoragePath` — hash 64자 미만 시 panic 대신 error 반환
- **drive/dataset**: `Pull` dest 상대경로 처리 — `filepath.Abs()` 적용 후 경로 순회 방어
- **drive/dataset**: `validateName` — 빈 이름, `/`, `\`, `..` 포함 시 거부
- **drive/cli**: DatasetClient 완전 위임 — 이중 백엔드 불일치 제거
- **archtest**: ratchet — `cmd/c4/drive.go` 5 violations 허용치 등록
- **handlers**: `drive.go/drive_stub.go` 위임 파일 추가 — `drive_test.go` build 오류 수정
- **doctor**: C5 Hub 체크 시 `api_prefix` 반영

### 🧪 Tests
- **drive**: 4개 신규 — `TestDatasetPull_RelativeDest`, `TestDatasetPull_TraversalRejected`, `TestValidateName`, `TestCasStoragePath_ShortHash`
- **handlers**: `TestRegisterDriveHandlers` + drive handler 통합 테스트

---

## [v0.66.2] - 2026-03-06

### 🐛 Bug Fixes
- **skillevalhandler**: `c4_skill_eval_status` body 파싱 버그 수정 — `List()`가 body를 반환하지 않아 항상 빈 결과; `confidence` 필드 + title 기반으로 교체
- **skillevalhandler**: `runHandler`에서 `confidence: trigger_accuracy` 메타데이터 저장 (indexed 필드)
- **skillevalhandler**: 동일 스킬 중복 제거 (최신 엔트리만 유지)
- **skillevalhandler**: 항상 0인 `unknown` 카운터 제거
- **eval**: c4-finish EVAL.md 부정 케이스 교체 — `trigger_accuracy` 0.938 (16케이스)

---

## [v0.66.1] - 2026-03-06

### 🐛 Bug Fixes
- **skilleval**: k 트라이얼 병렬화 — `sync.WaitGroup` + pre-allocated slice (소켓 65s 타임아웃 해소)
- **skilleval**: `mockProvider` race condition 수정 — `sync.Mutex` 추가 (`go test -race` clean)
- **skilleval**: `RunEval` ctx 취소 전파 — 케이스 루프 진입 전 `ctx.Err()` 체크
- **skilleval**: k 상한 100 + k=0 기본값 5 함수 레벨 가드 추가
- **skilleval**: majority vote `float64` 비교로 integer division tie-bias 제거
- **skilleval**: `RunEval` 함수 레벨 path traversal guard (`strings.ContainsAny`)
- **skilleval**: confidence 값 `[0,1]` 클램프 (LLM 이상 출력 방어)
- **eval**: `[X]` uppercase 체크박스 파싱 버그 수정 (`ToLower` 후 단일 prefix 비교)
- **archtest**: `errors.New` 사용으로 `fmt.Errorf-without-%w` ratchet 준수

### 🧪 Tests
- **skilleval**: 9개 테스트 (`TestRunEval_AllTrialsFail`, `TestRunEval_KZero`, `TestRunEval_PathTraversal`, `TestRunEval_KClamp` 신규)

---

## [v0.66.0] - 2026-03-06

### ✨ Features
- **init**: 온보딩 UX — `buildLaunchArgs` 순수함수 분리 + `isFirstRun`/`markFirstRun` (`~/.c4/first_run`, 전역 1회)
- **init**: 첫 실행 시 `--append-system-prompt`로 온보딩 컨텍스트 자동 주입 (`launchTool` + `launchToolNamed` 양쪽)
- **init**: `printReadyBox(io.Writer)` — 동적 너비 박스 출력 (rune 카운트 기반)

### 🐛 Bug Fixes
- **doctor**: `checkSkillHealth` 리팩터링 — `checkInfo` 상수 + `parseSkillEvalAccuracy` 헬퍼 + 테스트 3개 추가
- **c1**: MessageViewer 레이아웃 개선 + `useSessions` flex chain 수정 (min-height:0 완성)
- **archtest**: `skillevalhandler` allowedDeps baseline + `skilleval` fmt.Errorf ratchet 추가

---

## [v0.65.0] - 2026-03-06

### ✨ Features
- **skilleval**: `c4_skill_eval_run` — haiku LLM-as-judge 기반 스킬 트리거 정확도 측정 (pass@k / pass^k)
- **skilleval**: `c4_skill_eval_generate` — SKILL.md → EVAL.md 자동 생성 (skill='all' 일괄 지원)
- **skilleval**: `c4_skill_eval_status` — C9 experiment 기반 전체 스킬 헬스 요약
- **doctor**: `checkSkillHealth` — 낮은 trigger accuracy WARN, 미평가 스킬 OK
- **init**: `initAndLaunch()` ✓ 단계 출력 + `printReadyBox` 분리 (첫 실행 컨텍스트 주입)
- **skills/pi**: idea.md 저장 후 에디터 자동 열기

### 🐛 Bug Fixes
- **skilleval**: 에러 트라이얼을 `false` 결과로 계산하던 accuracy 위조 수정 (successCount 분리)
- **skilleval**: `AvgConfidence` 분모를 에러 포함 `len(Trials)` → `successCount`로 수정
- **skilleval**: `os.Stat(err) != nil` → `errors.Is(err, os.ErrNotExist)` 가드 (EPERM 구분)
- **skilleval**: `k` 상한 20 추가 (LLM 비용 폭탄 방지)
- **skilleval**: `skillName` 경로 순회 방어 (.., /, \\)
- **c1**: `.channels` flex:1 + parent flex-col — height:100% → flex 기반 교체
- **c1**: 스크롤 영역 확보 — flex chain min-height:0 완성
- **c1**: `sync_session_channels` 페이로드에서 `created_by` 제거
- **c1**: sessions.css flex:1/min-height:0 + claude_code slug `_` → `-`
- **c1**: 3개 버그 수정 — slug/스크롤/프로젝트전환

### 📚 Documentation
- **roadmap**: v0.64.1로 현행화 — v0.58~v0.64.1 이력 추가

---

## [v0.64.1] - 2026-03-06

### 🐛 Bug Fixes
- **c1**: MessageViewer 스크롤 앵커링 + 깜빡임 제거 (anchor 기반 bottom-lock)

### 🔧 Chores
- **hooks**: `Agent(isolation=worktree)` 차단 규칙 제거 — C4 worker 브랜치 자체가 이미 격리된 환경
- **hooks**: `settings.json` 특수문자 유니코드 이스케이프 정규화

---

## [v0.64.0] - 2026-03-06

### ✨ Features
- **pop**: `c4_pop_extract` content 파라미터 추가 — C1 Messenger 없이 Claude Code 세션 텍스트 직접 주입
- **pop**: `Engine.SetMessageSource()` — 런타임 message source override 메서드
- **pop**: `staticMessageSource` 어댑터 — 주입된 텍스트를 단일 Message로 래핑
- **pop**: `c4_pop_reflect` 구현 완성 — HIGH confidence 제안 목록 조회 (Validate 단계)
- **skills**: `/c4-finish` Step 7.7 추가 — 세션 요약 → `c4_pop_extract(content=...)` 자동 주입
- **skills**: `/pi` → `/c4-plan` 워크플로우 재설계 — EARS를 /pi로 병합

### 🐛 Bug Fixes
- **c1**: MessageViewer 가상화 제거 → 단순 overflow 스크롤
- **c1**: messenger 모드에서 240px 빈 channelList 칸 제거
- **c1**: messenger 탭에 ChannelsView 연결 + layout overflow 수정
- **c1**: `auth_login` anon_key 해석 시 `supabase.json` fallback 추가
- **c1**: `auth.rs`가 `~/.c4/supabase.json`에서 credentials 읽도록 수정
- **c1**: Terminal `fitAddonRef.ref` → `fitAddonRef.current` 오타 수정
- **twin**: `RegisterPopReflectHandler` → `RegisterPopReflectHandlers` 네이밍 수정

---

## [v0.63.0] - 2026-03-06

### ✨ Features
- **doctor**: stale-socket / zombie-serve / sidecar-hang 체크 추가
  - `checkStaleSocket`: `.c4/tool.sock` 존재하나 연결 불가 시 WARN
  - `checkZombieServe`: `cq serve` 프로세스 2개 이상 시 WARN
  - `checkSidecarHang`: PID 파일 기반 프로세스 생존 + 소켓 응답 체크
  - `cq doctor --fix` 연동: socket rm / pkill / sidecar kill 자동 처리
- **submit**: `validation_results`에 `status="fail"` 포함 시 서버 단 reject
  - 기존 optional 설계 유지 (생략 시 검증 없이 통과)
  - Worker가 실패 상태로 submit 강행하는 경로 차단

### 📚 Documentation / Config
- **AGENTS.md**: insights 기반 규칙 4개 추가
  - Workflow: 명시적 허가 전 태스크 스폰·코드 변경 금지
  - Efficiency: 코드베이스 탐색 시 Agent 위임 우선
  - Debugging: 조사 대상 시스템 도구 사용 금지
  - Git Workflow: 작업 전 미커밋 변경사항 확인
- **settings.json**: PostToolUse hook — `go vet` 추가, `python` → `uv run python`

---

## [v0.62.0] - 2026-03-06

### 🐛 Bug Fixes
- **tool**: `CQ_TOOL_SOCK` 환경변수 오버라이드 — `cq serve`/`cq tool` 다른 디렉토리 실행 시 소켓 경로 불일치 해결
- **pi**: sentinel 패턴 제거 (`/tmp/.c4_allow_plan_mode`) — stale 파일 gate 우회 취약점 제거, c4-gate.sh 정합성 복구

### 📚 Documentation
- **tool**: `cq tool`은 사람용 터미널 도구 명시 (에이전트는 MCP `c4_*` 사용)
- **AGENTS.md**: /pi 스킬 EnterPlanMode 담당 문구 제거

---

## [v0.61.0] - 2026-03-05

### ✨ Features
- **tool**: `cq serve` + Unix Domain Socket bridge — socket-first MCP→CLI 게이트웨이
  - `toolSocketComponent`: `.c4/tool.sock` UDS 서버, `cq serve` 컴포넌트로 등록
  - `cq tool` cold start ~500ms → socket 경유 ~10ms (serve 실행 중)
  - `accept(ctx, ln)` 패턴으로 Stop() race condition 방지
  - SIGTERM이 in-flight tool call에 전파 (serve ctx 스레딩)
  - `io.LimitReader` 4MB cap, 5s read / 65s write 분리 deadline
  - socket-first → inline MCP 폴백 자동 처리

### 🔧 Chores
- **tool**: Schema Diet 롤백 (prompt caching으로 실익 없음, 대안으로 Option D 채택)

---

## [v0.59.0] - 2026-03-05

### ✨ Features
- **tool**: `cq tool` MCP→CLI 자동 게이트웨이 PoC — `DisableFlagParsing=true` + 동적 `cobra.Command`로 모든 MCP 도구를 CLI에서 직접 호출 가능. 에이전트는 `Bash("cq tool <name> --json")`으로 스키마 등록 없이 읽기 전용 도구 호출 (Schema Diet 전략). Cold start ~0.49s. 테스트 10개.

## [v0.58.0] - 2026-03-05

### ✨ Features
- **init**: `cq gemini` 서브커맨드 추가 — Gemini CLI를 CQ 프로젝트에 통합 (cq claude와 동등, `-t` 세션 명명 지원)
- **skills**: `/pi` → `/c4-plan` → `/c4-run` → `/c4-finish` 자동 체인 — ideation에서 배포까지 원스톱 워크플로우

### 🔧 Chores
- **docs**: Korean i18n (VitePress) — user/docs/ko/ 완전 미러링
- **docs**: 문서 전체 현행화 v0.45→v0.57 (스킬 36개, MCP 148개, 테스트 ~3,277개)

## [v0.57.0] - 2026-03-03

### ✨ Features
- **pop**: Personal Ontology Pipeline (POP) 초기 구현 — Extract → Consolidate → Propose → Validate → Crystallize 5단계 파이프라인
  - `c4_pop_extract` MCP tool — 메시지 기반 LLM 추출 사이클 (BlockingHandlerFunc, 60s 서브-데드라인)
  - `c4_pop_status` MCP tool — 파이프라인 상태 + gauge 값 + knowledge 통계 (domain="pop" 필터)
  - `c4_pop_reflect` MCP tool — HIGH confidence (≥0.8) POP 제안 조회
  - `cq pop status` CLI 커맨드 — gauge.json + state.json 인라인 표시
  - `GaugeTracker` — merge_ambiguity/avg_fan_out/contradictions/temporal_queries 임계값 + 원자적 Save
  - `Engine.RunOnce()` — confidence 게이팅: RecordProposal(ALL) → Notify/soul(HIGH only)
  - `Consolidator` — normalizedLevenshtein 기반 병합, 모순 감지, gauge 업데이트
  - `Crystallizer` — 원자적 SOUL.md 쓰기, soul_backup/ pruning (10개 유지)
  - `CLINotifier` — `$EDITOR` 인터랙티브 검증, y→validated/n→boundary knowledge 기록
- **pop/pkg**: `ConfidenceThreshold = 0.8` exported 상수, `DefaultStatePath()`, `DefaultGaugePath()`
- **pop/security**: path traversal guard (soulWriterAdapter), insight 4096 bytes truncation

### 🐛 Bug Fixes
- **pop**: `RecordProposal` — mapItemType() 검증으로 "fact"/"rule" 등 비유효 item_type의 raw DocumentType 캐스팅 방지
- **pop**: state.Save() 원자적 쓰기 (tmpfile → Rename) — gauge.Save()와 동일 보장
- **pop**: checkGauges non-ErrNotExist 에러 로깅 (파일 손상 감지 가능)
- **pop**: reflectHandler에서 비-POP 고신뢰 문서 오염 제거 (`domain="pop"` 태그 + 필터)

---

## [v0.56.0] - 2026-03-03

### ✨ Features
- **harness**: Cursor IDE SQLite 어댑터 추가 — `~/Library/.../state.vscdb` 5분 주기 폴링, read-only 모드, `cursor_processed` DB로 composer 레벨 중복 방지
- **harness**: `llm-call.sh` 신규 — Gemini/Claude/OpenAI LLM 라우터 (`SOUL_LLM=auto|claude|gemini|openai`), `soul-evolve.sh` 위임
- **infra**: migration 00030 — `c1_channels(tenant_id, platform, name) WHERE channel_type='session'` 부분 유니크 인덱스 (EnsureChannel 멱등성 보장)

### 🐛 Bug Fixes
- **harness**: `SetOffset`을 `Push` 성공 후에만 호출 — 이전에는 push 실패 시에도 offset 전진 → 메시지 유실. 이제 push 오류 시 즉시 return으로 다음 fsnotify 이벤트에서 재시도

### 🔧 Chores
- **docs**: 테스트 수 현행화 (`~2,044 → ~2,457`) + archtest allowlist 추가 (c1push HTTP status 에러)
- **skills**: c4-finish Step 7.6 Persona Evolution (soul-check 자동 실행) 추가

---

## [v0.55.1] - 2026-03-03

### ✨ Features
- **exec/ux**: `c4_execute` 우선 사용 원칙 강화 — AGENTS.md Efficiency Rules에 추가. `go test`, `git log`, `git diff`, `find`, `builds` 등 대용량 출력 명령은 Bash 대신 `c4_execute` 사용. 파이프(`|`) 체인만 Bash 유지. tool description도 지시적으로 강화("PREFER over Bash for: ...")

### 🐛 Bug Fixes
- **exec/test**: `=== RUN` 라인 항상 제거 — 이전에는 출력이 임계값(4KB)을 넘을 때만 제거. 이제 test 모드에서는 임계값 무관하게 항상 `=== RUN`/`=== PAUSE`/`=== CONT` 노이즈 제거

---

## [v0.55.0] - 2026-03-03

### ✨ Features
- **mcp/execute**: `c4_execute` 도구 추가 — 컨텍스트 압축 명령 실행기. 대용량 Bash 출력을 자동 압축(test/build/git/generic 모드)해 컨텍스트 소비 절감. 압축 임계값 4KB, 최대 반환 8KB, 타임아웃 60s
- **auth/signup**: `cq auth signup` UX 개선 — `term.ReadPassword`로 password echo 방지, `UpsertProfile` + `PatchTeamYAMLCloudUID` 자동 호출, URL injection 방지(`url.QueryEscape`), io.LimitReader 응답 크기 제한
- **cloud/project**: `ProjectClient` 신설 — `cq project list/new/use` 명령어 + `SetActiveProject` YAML round-trip
- **skills/pi**: `EnterPlanMode` 허용 — `/pi` 스킬 ideation 진입 시 Plan Mode UI 활성화

### 🐛 Bug Fixes
- **archtest**: `getActiveProjectID` YAML section 버그 수정 — `project.id` → `cloud.active_project_id`
- **cloud/auth**: `PatchTeamYAMLCloudUID` exported (cmd/c4에서 직접 호출 가능)
- **exechandler**: `fmt.Errorf("command is required")` → `errors.New` (archtest ratchet)

---

## [v0.54.1] - 2026-03-03

### 🐛 Bug Fixes
- **reboot**: `$PPID` 빈값 버그 수정 — Bash 툴 서브셸에서 `$PPID`가 항상 비어있어 SIGTERM 미발송. `ps -o ppid= -p $$`로 claude PID 직접 조회
- **reboot**: 세션명 덮어쓰기 버그 수정 — `cq -t <new-name>`으로 새 세션 시작 시 reboot 후 기존 저장 세션명으로 덮어쓰이던 문제. `nameWasKnown` 플래그로 차단

---

## [v0.54.0] - 2026-03-03

### ✨ Features
- **ux/serve**: `cq serve` 투명화 — `confirmServeInstall()` 제거, `ensureServeRunning()` 성공/타임아웃 메시지에 PID 추가
- **ux/doctor**: `os-service` 미설치 WARN → OK ("auto-starts on next cq claude")
- **skills/pi**: Play Idea 스킬 신설 — ideation 모드 (c4-plan 이전). 발산/수렴/심화/반론 4모드, WebSearch + c4_knowledge_search 병렬 리서치, idea.md → /c4-plan 자동 전환
- **cloud/team**: `TeamClient` + `cq team list/invite/remove` 명령어 추가
- **persona**: `getActiveUsername` 비결정성 수정 + `TeamMember.CloudUID` 추가
- **infra**: migration 00029 — `c4_profiles`, `c4_pending_invitations` 테이블 + RPC

### 🐛 Bug Fixes
- **c5/conversation**: `EnsureChannel`/`EnsureParticipant` 409 Conflict 처리 추가

---

## [v0.53.0] - 2026-03-02

### ✨ Features
- **c5/conversation**: 통합 대화 플랫폼 — Dooray 봇 대화를 C1 `c1_*` 테이블로 통합
  - Supabase migration 00028: `tenant_id`/`platform` 컬럼, `project_id` nullable, `channel_type IN ('bot','event')` 확장
  - 부분 유니크 인덱스 `uniq_c1_channels_bot`, `uniq_c1_members_platform` (PostgreSQL NULL 중복 방지)
  - `conversations` 테이블 → `conversations_legacy` 리네임 + 하위호환 읽기 뷰
  - `conv_to_knowledge` DB 트리거: bot 채널 어시스턴트 답변 → `c4_documents` 자동 ingestion
  - `Store` 인터페이스: `EnsureChannel`, `ListChannels`, `EnsureParticipant` 추가
  - `SupabaseStore`: `c1_messages`/`c1_channels`/`c1_members` 직접 사용 (PostgREST)
  - `MemoryStore`: in-process fallback, null-byte 키 구분자(`\x00`)
- **c5/dooray**: Dooray 봇 채널 UUID 관리 — `EnsureChannel`/`EnsureParticipant` 호출, knowledge goroutine 제거 (DB 트리거 대체)

## [v0.52.0] - 2026-03-01

### ✨ Features
- **c5**: `/v1/research/state` REST API — C9 분산 라운드/페이즈 상태 관리 (G12)
  - Optimistic lock (CAS) — `version` 컬럼 불일치 시 409 Conflict
  - Advisory lock — `POST/DELETE /v1/research/state/lock` (TTL + stale auto-evict)
  - `GetOrCreateResearchState`, `UpdateResearchState`, `AcquireResearchLock`, `ReleaseResearchLock`
  - 테스트 6개: GetInitial, PutVersionMatch, PutVersionConflict, Lock, LockExpiry, ConcurrentPut
- **c4**: SQLite advisory scope lock — C4 Worker 파일 충돌 방지 (G11)
  - `c4_phase_lock_acquire` / `c4_phase_lock_release` MCP 도구
- **c3/eventbus**: `SubscribeWithProject` 헬퍼 + 프로젝트 ID 누락 시 WARN 로그
- **c9-scripts**: `c9-state-api.py` stdlib-only CLI helper — API-first + state.yaml fallback
  - `c9-run.sh` `set_phase()`: API PUT → 성공 시 state.yaml 동기화, 409 시 GET version 재동기화
  - `c9-check.sh`: yaml.dump 후 Research State API 비동기 동기화 (non-fatal)

### 🐛 Bug Fixes
- **c9-run.sh**: 409 충돌 후 GET으로 version 재동기화 (version drift 방지)
- **research.go**: DELETE `/lock` body → query param (`?worker_id=`) 으로 변경 (HTTP intermediary 호환)

### 🔧 Chores
- **research.go**: PUT/POST handler에 `http.MaxBytesReader(64KiB)` 추가
- **model.go**: `ResearchState.LockHolder`에 `omitempty` 추가
- **store/sqlite.go**: `AcquireResearchLock` 설계 주석 추가 (row-exists gap, 포맷 차이)

## [v0.51.0] - 2026-03-01

### ✨ Features
- **c5**: Dooray 멀티턴 대화 히스토리 + Supabase 영구 저장
  - `conversation.Store` 인터페이스 — MemoryStore (TTL fallback) + SupabaseStore (PostgREST)
  - `llmclient.ChatWithHistory` — 채널별 20턴 히스토리를 LLM context로 전달
  - `knowledge.Client.Record` — 대화 내용 C9 Knowledge async ingestion
  - Supabase migration 00027: conversations 테이블 + RLS 정책
- **secrets**: `GetNS` / `SetNS` project-namespaced secret API 추가
- **c5**: job 제출 시 `submitted_by` audit trail 기록

### 🐛 Bug Fixes
- **c9-scripts**: `python3` → `uv run python` (uv 환경 기준 통일)

### 🔧 Chores
- **c4-core**: CLI 표기 `c4` → `cq` 일관성 정렬 (daemon/auth/doctor/hub/root)
- **c4-core**: `version` 명령 추가 — tier 빌드 정보 포함

### 📚 Documentation
- **AGENTS**: 테스트 수 업데이트 — c5 9개 패키지, ~2,038 Go 테스트

---

## [v0.50.1] - 2026-03-01

### 🐛 Bug Fixes
- **c9-scripts**: Shell injection 전면 제거 — 21개 injection site → 0 (CRITICAL 보안 수정)
  - `python3 -c "... '$VAR' ..."` → `VAR="$VAR" python3 -c "os.environ['VAR']"` env var passthrough 패턴 전면 적용
  - `<<'PYEOF'` single-quoted heredoc으로 bash 확장 차단
  - curl word-splitting 방지: `curl_args=(); curl "${curl_args[@]}"` 배열 패턴
  - path traversal 방지: `mktemp + trap 'rm -f "$TMPFILE"' RETURN + job_id regex 검증`
  - `grep -F` 고정문자열 매칭 (regex 메타문자 injection 방지)
  - ROUND 검증 guard: `[[ ! "$ROUND" =~ ^[0-9]+$ ]]` exit 1
- **c9-watch.sh**: `C9_METRIC_NAME` env var 파이프라인 위치 수정
  - 파이프 좌측 curl 앞이 아닌 우측 python3 앞에 위치해야 python3가 env var 수신 가능
  - `curl ... | C9_METRIC_NAME="$M" python3 -c "os.environ.get('C9_METRIC_NAME', ...)"` 올바른 패턴
- **c9-watch.sh**: metric_name 계약 정렬 — c9-check.sh와 동일한 `re.escape(metric_name)` 패턴으로 파싱
- **c9-watch.sh**: TOCTOU 방지 — state.yaml metric 설정을 1회만 파싱 후 env var 전달

### 🔧 Chores
- **c9-check.sh**: ROUND 검증 guard 추가 (`^[0-9]+$` 정규식)
- **c9-run.sh**: `with open()` context manager, `grep -m 1` 첫 매칭만, curl_args 배열

---

## [v0.50.0] - 2026-03-01

### ✨ Features
- **c5**: Capability Broker — 워커 자기선언(YAML), typed invocation, project-scoped discovery
  - `GET /v1/capabilities`: online 워커 그룹핑, input_schema 포함
  - `POST /v1/capabilities/invoke`: capability 잡 생성 + 큐잉
  - `POST /v1/capabilities/update`: 워커 capability 실시간 갱신 (worker ownership 검증)
  - `AcquireLease`: capability 필터 — 등록 워커만 해당 잡 acquire
  - Worker: `--capabilities caps.yaml` 플래그 + `C5_PARAMS`/`C5_CAPABILITY`/`C5_RESULT_FILE` env 주입
  - `SetJobResult`: capability 잡 structured result 저장
- **c5/mcp**: MCP Streamable HTTP 서버 (`POST /v1/mcp`, JSON-RPC 2.0)
  - `initialize` / `tools/list` / `tools/call` / `ping` 지원
  - capability → MCP tool 1:1 매핑 + 5개 내장 hub tools
  - `tools/call`: client disconnect 감지 (r.Context().Done()) — goroutine leak 방지
  - `.mcp.json`에서 `{"type":"url","url":"https://hub/v1/mcp"}` 직접 연결
- **c5/dooray**: `invoke_capability` action 추가 + 동적 capability 목록 시스템 프롬프트 주입

### 🐛 Bug Fixes
- **c5/mcp**: `hub_cancel_job` EventBus publish 누락 수정 (handleJobCancel과 일관성)
- **c5/worker**: capability sentinel path traversal 방지 (`filepath.Base` 검증)
- **c5/mcp**: `tools/call` nil params 가드 추가

---

## [v0.49.0] - 2026-03-01

### ✨ Features
- **c9**: domain-neutral `state.yaml` 스키마 도입 — `metric.name/unit/lower_is_better/convergence_threshold` 범용화, HMR(MPJPE) 하드코딩 제거 + migration guide 포함
- **c5/dooray**: `query_status` action — 워커+잡 교차 참조 종합 현황 조회 + 프롬프트 강화

### ♻️ Refactoring
- **c9-run/check/watch**: HMR 하드코딩 제거 — `metric.name` 동적 참조로 범용화
- **c9-loop/c9-survey**: HMR 하드코딩 제거 — `metric.name/unit/lower_is_better` 동적 참조

### 🔧 Chores
- **c9 polish(rounds 1-9)**: C9 스크립트 품질 강화
  - JSON 직렬화로 다줄 템플릿 값 안전 파싱 (notify.sh)
  - env var injection 방지 (`EXP_NAME_ENV`/`C9_API_KEY_ENV` 패턴)
  - urllib/curl 타임아웃 추가 — 서버 비응답 행잠 방지 (`urlopen(timeout=30/60)`, `--max-time 10`)
  - webhook curl `--max-time 10 --connect-timeout 5`
  - HUB_URL 로드 블록 `STATE_FILE` env var 통일
  - `--poll-only` 모드 ROUND 변수 버그 수정
  - eval 타임아웃 후 SUCCEEDED/DONE 명시 검증 (조건 반전)
  - 원자 저장 (`NamedTemporaryFile → os.replace`) 전체 적용
  - `metric_unit None` → 빈문자열 처리 (`or ''` 패턴)

### 📚 Documentation
- **AGENTS.md**: skills 수 22 → 35 업데이트

---

## [v0.48.0] - 2026-03-01

### ✨ Features
- **c5/dooray**: LLM action 확장 — `query_workers` / `query_jobs` 자연어 조회 지원 (`"서버 상태 봐바"` → `{"action":"query_workers"}` → `store.ListWorkers()` → Dooray 포맷 응답)
- **c5/dooray**: 잡 완료 Dooray 알림 — `notifyDoorayJobComplete`: SUCCEEDED 시 최종 메트릭 요약, FAILED 시 실제 마지막 3줄 로그 전송 (DOORAY_CHANNEL env 없는 잡은 no-op)
- **c5/dooray**: semaphore-full 시 silent drop → 유저 알림 (`"⚠️ 현재 요청이 많아 처리할 수 없습니다."`)
- **c5/dooray**: 시스템 프롬프트 CQ 정체성 + 새 action 어휘 (`query_workers`, `query_jobs`) 추가
- **skills**: `/c9-init` SKILL.md — 신규 C9 연구 루프 프로젝트 초기화 스킬
- **skills**: `/c9-steer` SKILL.md — phase 전환 추상화 + 원자 저장 패턴 스킬

### 🐛 Bug Fixes
- **c5/llmclient**: 기본 모델 `gemini-3-flash-preview` 수정 (존재하는 모델 ID)
- **c5/dooray**: `submit_job` 잡에 `DOORAY_CHANNEL` env 보존 → 완료 알림 연동
- **c5/dooray**: `query_jobs` status 대소문자 정규화 (`strings.ToUpper`) — LLM이 소문자 반환 시 조회 실패 방지
- **c5/dooray**: `query_jobs` limit 상한선 100 추가 — LLM 주입 대량 DB scan 방지
- **c5/dooray**: `handleJobComplete` 내 GetJob 이중 호출 → 단일 fetch 후 deploy rules + Dooray notify 공유

### ♻️ Refactoring
- **c5/llmclient**: 중복 모델명 fallback 제거 — `config.LLM.Model`에서 단일 관리

---

## [v0.47.0] - 2026-03-01

### ✨ Features
- **c5/dooray**: 서버사이드 LLM 워커 — `POST /v1/webhooks/dooray` 수신 즉시 ephemeral ack 후 goroutine에서 LLM 호출 + Dooray Incoming Webhook 응답 (LLM 미설정 시 Hub Job fallback으로 후방 호환)
- **c5/llmclient**: OpenAI-compatible chat completion 클라이언트 신규 추가 — Gemini/Ollama/OpenAI 지원, stdlib only, 4MiB LimitReader
- **c5/knowledge**: Supabase PostgREST `c4_documents` 검색 클라이언트 신규 추가 — channelId→projectId 라우팅, 2000 rune body truncation
- **c5/config**: `LLMConfig`, `DoorayConfig`, `DoorayChannelCfg` 신규 — env override 지원 (`C5_LLM_*`, `C5_DOORAY_*`)

### 🐛 Bug Fixes
- **c5/dooray**: request body 512KiB 제한 — OOM 방지
- **c5/dooray**: `doorayHTTPClient` 15s timeout + response body drain — goroutine leak 방지
- **c5/dooray**: `llmSem` (cap=16) goroutine 동시 실행 제한 — 고부하 시 goroutine 폭증 방지
- **c5/knowledge**: PostgREST metachar strip (`(`, `)`, `,`, `*`) — filter injection 방지
- **c5/llmclient**: 기본 모델 `gemini-3.0-flash` → `gemini-2.0-flash-lite` 수정 (존재하는 모델 ID)
- **c5/llmclient**: HTTP status 체크를 API error 체크 이전으로 이동 + 에러 body 512B 절단

### 📚 Documentation
- **agents**: Go 테스트 수 업데이트 (c5 ~249, 합계 ~2,836)
- **c5/config**: `ExampleConfigYAML`에 `llm:` / `dooray:` 섹션 추가

---

## [v0.46.0] - 2026-03-01

### ✨ Features
- **c9-loop**: Knowledge integration 추가 — CHECK phase `c4_experiment_record`, CONFERENCE/REFINE phase `c4_knowledge_search(doc_type="experiment")`, FINISH phase `c4_knowledge_record`
- **c9-survey**: Step 5 `c4_knowledge_record` 연동 추가 — survey 결과를 Knowledge DB에 insight로 저장
- **skills**: c9-loop/c9-survey 파라미터 수정 및 호출 방식 명확화 — `tags=` → `doc_type=` 대체, bash pipe → 스킬 직접 호출
- **c5**: LLM + Knowledge client integration for Dooray server-side processing

### 🔧 Chores
- **docs**: user/ 서브모듈 v0.45.0 현행화

---

## [v0.45.0] - 2026-03-01

### ✨ Features
- **ls**: `cq ls` 출력 포맷 전면 개선 — 컬럼 정렬, CJK 폭 인식 (`lsDispWidth/lsPadToWidth/lsTruncateToWidth`), `●` 현재 세션 인디케이터, `Jan 02 15:04` 간소화 날짜, `✉N` unread 아이콘

### 🐛 Bug Fixes
- **session**: reboot 후 세션 이름 미갱신 수정 — `launchToolNamed` reboot 핸들러에서 `named-sessions.json` 재조회하여 `name` 변수 갱신
- **session**: `cq session name` 동일 UUID 중복 alias 방지 — `break` 제거로 같은 UUID 항목 전체 삭제 (first-memo/tool 보존)
- **ls**: `cq ls`에서 동일 UUID `(current)` 중복 표시 방지 — 첫 일치 후 `curUUID` 초기화

---

## [v0.44.0] - 2026-03-01

### ✨ Features
- **c5/sse**: SSE 이벤트 스트림 프로젝트 격리 — `broadcastSSEEvent(projectID)` 3-arg 시그니처, sync.Map value에 project_id 저장, 테넌트 간 이벤트 누출 차단
- **c5/tenant**: DAG.ProjectID, Edge.ProjectID, DeployRule.ProjectID 필드 추가 — API 레이어에서 `projectIDFromContext(r)` 기반 격리 쿼리 (`WHERE project_id = ? OR project_id = ''`)
- **c5/dooray**: Dooray webhook 핸들러 추가 — `POST /v1/webhooks/dooray` 수신, fly.io 배포 E2E 검증 완료

### 🐛 Bug Fixes
- **c4-attach**: `CQ_SESSION_UUID` env var로 세션 UUID 확인 (JSONL 경로 추론 금지)

### ♻️ Refactoring
- **c4-gate**: c4-finish 인터셉트 게이트 강화 — polish 미완료 시 c4-finish 차단, git commit은 in_progress + polish 미완료 AND 조건으로 완화 (deprecated 스킬 차단 블록 제거)
- **dooray**: 로컬 webhook-gateway Dooray inbound 코드 제거 (방향 B 정리) — fly.io 배포로 통합

### 📚 Documentation
- **agents**: Go 테스트 수 업데이트 (c4-core ~1,767 + c5 ~232 = ~1,999, 45 packages)

---

## [v0.43.0] - 2026-03-01

### ✨ Features
- **gemini**: Gemini 3.0 에이전트 업그레이드 + C1 "Hands" 브리지 (WebSocket 기반 네이티브 셸 실행)
- **enforce**: 3-layer deprecated 스킬 강제 시스템
  - Layer 1: `c4-gate.sh` Hook Gate — c4-polish/c4-refine → c4-finish 리다이렉트 차단
  - Layer 2: Arch Test — `TestDeprecatedSkillsAreStubs`, `TestFinishSkillsHaveKnowledgeGate`, `TestPlanSkillsHaveKnowledgeRead`
  - Layer 3: Skill Linter — `scripts/lint-skills.sh`, `make lint-skills`
- **c3/eventbus**: Dooray 양방향 응답 — `c4_dooray_respond` MCP 도구 + `dooray_respond_llm` action type + LLM caller
- **skills**: plan-run-finish 3단계 워크플로우 + C9 지식 게이트 패턴 (finish=기록 게이트, plan=조회 게이트)

### 🐛 Bug Fixes
- **reboot**: `.reboot` 파일에 UUID 기록하여 올바른 세션 복구 보장
- **archtest**: C3 EventBus dooray_respond 추가에 따른 ratchet allowlist 업데이트

### ♻️ Refactoring
- **skills**: c4-polish(336→17줄), c4-refine(339→17줄) deprecated stub으로 축약

### 🔧 Chores
- **dooray**: WebhookGateway Stop() mutex 해제 후 Shutdown 패턴 + HTTPS 강제 + 보안 모델 문서화
- **bats**: c4gate hooktest 케이스 14→19개 (deprecated 차단 4 + c4-finish 긍정 케이스 1 추가)

---

## [v0.42.1] - 2026-02-28

### 🧪 Tests
- **internal/cloud**: CloudPrimaryStore + HybridStore 커버리지 60.7% → 75.0% (30+ 신규 테스트)
  - mock store injection 패턴 (httptest 금지), compile-time assertion
- **internal/eventbus**: client.go 커버리지 67.2% → 78.2% (28 신규 테스트)
  - 실제 Unix Domain Socket + gRPC 서버 패턴, macOS 104-byte 경로 제한 해결
- **internal/secrets**: store 커버리지 68.0% → 83.6% (16 신규 테스트)
  - NewWithPaths edge cases, 잘못된 권한/hex/length, closed store 에러 경로
- **internal/drive**: client 커버리지 68.2% → 79.1% (14 신규 테스트)
  - doWithRetry 35% → 95%, 5xx 재시도/연결 실패/4xx 즉시 실패/POST body 재읽기
- **artifacthandler**: Register() + resolvePath() 커버리지 67.5% → 81.8% (7 신규 테스트)
- **knowledgehandler**: knowledge_coverage_test.go 475줄 추가, 62.3% → 77.1%

### 📚 Documentation
- **agents**: Go 테스트 수 수치 보정 (c4-core ~1,753 + c5 ~214 = ~1,967)

---

## [v0.42.0] - 2026-02-28

### ✨ Features
- **c3/webhookgw**: 양방향 Dooray 연동 — WebhookGateway inbound 수신 (v0.41.0 outbound 완성)
  - `WebhookGatewayComponent`: `POST /v1/webhooks/dooray` 핸들러 (port 4142, configurable)
  - `subtle.ConstantTimeCompare` cmdToken 검증 — timing side-channel 방지
  - `webhook.dooray.inbound` EventBus 이벤트 발행 (appToken/cmdToken/responseUrl 제외)
  - Dooray 응답: `{"text":"수신 완료","responseType":"ephemeral"}`
  - `serve.webhook_gateway.enabled` + `dooray.cmd_token` config 스키마 추가
  - `DOORAY_CMD_TOKEN` 환경변수 폴백 지원
  - `serve_webhookgw.go` (c3_eventbus) + stub 빌드 태그 분리
  - 테스트 12개 추가 (token 검증, 보안 필드 노출 방지, nil publisher, method 검증 등)

### 📚 Documentation
- **AGENTS.md**: 테스트 수 갱신 (c4-core ~1,982, 합계 ~2,197)

---

## [v0.41.0] - 2026-02-28

### ✨ Features
- **c3/eventbus**: Dooray/Discord/Slack/Teams 외부 알람 채널 연동
  - `notifications.channels` config: `type` 기반 프리셋 (dooray/discord/slack/teams/generic)
  - `c4_rule_add`에 `channel` 단축 파라미터 추가 — JSON payload 없이 채널명만 지정
  - `c4_notification_channels` MCP 도구: 등록된 채널 목록 + URL masking 조회
  - `payload_template` + `payload_content_type` webhook action 파라미터 지원
  - `BuildPayloadTemplate()`: type → JSON payload 자동 생성, `jsonStr()` 헬퍼로 proper escaping

### 🐛 Bug Fixes
- **c3/eventbus**: `resolveJSONTemplateString()` 추가 — application/json 템플릿에서 `"`, `\n` 등 특수문자 포함 이벤트 데이터 시 알림 무음 실패 방지
- **c3/eventbus**: `resolveTemplateString`/`resolveJSONTemplateString` cursor 기반 스캔 — 치환된 값에 `{{}}` 포함 시 템플릿 인젝션 방지
- **c3/eventbus**: webhook POST 후 `resp.Body` drain (LimitReader 4096B) — HTTP keep-alive 연결 재사용 보장
- **config**: `GetNotificationChannel()` 포인터 → 값 반환 — `Set()` 동시 호출 시 aliasing 위험 제거
- **config**: generic 타입 채널에 `payload_template` 미설정 시 명시적 에러 반환 (기존: CloudEvents silent fallback)
- **c3/eventbus**: eventData unmarshal 실패 시 `log.Printf` 경고 추가

### 📚 Documentation
- **AGENTS.md**: 테스트 수 갱신 (c4-core ~1,970, c5 ~215), C3 EventBus notification channel 사용법 추가

---

## [v0.40.1] - 2026-02-28

### 📚 Documentation
- **AGENTS.md**: MCP 도구 수 148개 (Base 118 + Hub 30), `c4_hub_lease_renew` 추가, Skills 22개 수정
- **설치-가이드.md**: 148 tools 수정 (118 base + 30 Hub), GitLab→GitHub URL, LLM 키 우선순위 현행화
- **deployment-topology.md**: `c4 auth` → `cq auth` 정정, 마이그레이션 수 21개, 도구 수 118/148개
- **배포-체크리스트.md**: 마이그레이션 21개, MCP 도구 수 118 base / Hub 활성 시 148개 정정
- **ROADMAP.md**: Hub 도구 수 30개 (총 148개), 릴리즈 이력 v0.26~v0.40 현행화
- **워크플로우-개요.md**: REFINE/POLISH 상태, DISCOVERY/DESIGN 서브페이즈 추가
- **빠른-시작.md**: 워크플로우 명령어 현행화
- **ARCHITECTURE.md**: 테스트 수치 AGENTS.md SSOT 기준 동기화
- **LLM-설정.md**, **config-guide.md**: LLM 키 우선순위 구버전 제거 (`~/.c4/secrets.db > 환경변수`)
- GitLab→GitHub URL 전체 현행화 (6개 파일)

---

## [v0.40.0] - 2026-02-28

### ✨ Features
- **knowledge**: `OllamaEmbeddings` 프로바이더 + `reindex` 모듈 (`provider` 파라미터 지원)
  - `get_embeddings_provider("ollama")` — `nomic-embed-text`(768dim) / `mxbai-embed-large`(1024dim)
  - `reindex_all(provider, base_path, _embedder)` — knowledge.db 전체 재임베딩 + legacy `documents` 테이블 fallback
  - `c4_knowledge_reindex` MCP 도구 `provider` 파라미터 연동
- **knowledge**: `recreate_for_dimension(768)` + migration `00026`
  - `VectorStore.recreate_for_dimension(dim)` — 기존 벡터 삭제 후 새 차원으로 테이블 재생성
  - migration `00026_knowledge_vectors_768dim.sql`: pgvector 컬럼 1536→768 마이그레이션 + rollback SQL 주석
- **knowledge**: `distill` 모듈 — 빈/소규모 코퍼스 guard 포함
  - `cosine_similarity`, `find_clusters`, `distill_knowledge` 함수
  - 코퍼스 0건 / 임베딩 부족 시 graceful early-return
- **skills**: `c4-release` — 태그 생성 + origin push 자동 포함 (`--no-push` 옵션)
- **skills**: `c4-finish` Step 9 — auto tag + release notes 자동 실행

### 🐛 Bug Fixes
- **knowledge/llm**: QM batch 3-round quality convergence
  - `reindex.py`: conn 이중 close 수정 (loop 전체를 outer try/finally로 감쌈)
  - `vector_store.py`: SQL injection 방어 (table_name regex 검증), dim 유효성 검사, struct little-endian 명시
  - `cost.go`: `SetDB` nil guard + 중복 호출 guard (goroutine 누출 방지)
  - `llm.go`: `cache_hit_rate` → `cache_utilization_rate` (공식: `read/(prompt+read)`)
  - `handler_test.go`: 3개 테스트 키 이름 동기화
- **c5/auth**: polish round 4 — state 기반 CSRF + supabaseKey PKCE + ready 보호
  - `auth_device.go`: 쿠키 기반 CSRF → URL state 파라미터로 교체 (SameSite/Secure 문제 해소)
  - `auth_callback.go`: `exchangePKCEToken`에 `apikey`/`Authorization` 헤더 추가
  - `sqlite.go`: 최대 poll_count 20→60, `'ready'` 세션은 만료 방지
  - `fly.toml`: `C5_PUBLIC_URL` 환경변수 추가
- **hooks**: `curl|python3 -c` 허용 — bare 인터프리터만 차단, inline `-c` 코드는 통과

### 🧪 Tests
- **hub**: +7.1%p 커버리지 — `GetID`/`isTimeout`/`ClaimJobWithWait` (68.1% → 75.2%)
- **cdp**: +6.8%p 커버리지 — 25.3% → 32.1%
- **knowledge**: `test_reindex.py` legacy `documents` 테이블 fallback 테스트 추가
- **c5/store**: `TestDeviceSessionReadyNotExpiredByPollCount` 회귀 테스트 추가

### 📚 Documentation
- `AGENTS.md`: 테스트 수 업데이트 (Go ~2,192, Python 728, 합계 ~3,012)

---

## [v0.39.0] - 2026-02-28

### ✨ Features
- **hooks**: `c4-gate.sh` — `TaskCreate` / `TaskUpdate` 차단 추가 + MCP namespace 정규화
  - `mcp__<ns>__ToolName` 형식을 bare 이름으로 정규화 (`_BARE_TOOL`)
  - `TaskCreate` → `c4_add_todo` 대체 안내, `TaskUpdate` → `c4_submit` 대체 안내
  - `.claude/hooks/c4-gate.sh` ↔ `c4-core/cmd/c4/templates/hooks/c4-gate.sh` 동기화
- **hooks**: 워크플로우 게이트 — `git commit` / `/c4-finish` polish 미완료 차단
  - `C4_SKIP_GATE=1` 인라인 우회 지원 (export 금지)
  - `.c4/` 디렉토리 존재 여부로 C4 프로젝트 판별
- **hooks**: `c4-gate.sh` `TodoWrite` / `EnterPlanMode` 차단 기반 구축

### 🧪 Tests
- **hooktest**: `bats` 테스트 스위트 추가 (10개 테스트)
  - `test_global_antipattern.bats` (8개): pip/python/pytest 안티패턴 차단 검증
  - `test_c4gate_mcp.bats` (10개): TodoWrite/EnterPlanMode/TaskCreate/TaskUpdate + namespace 정규화

### 🔧 Chores
- **polish**: `global-antipattern.sh` echo 폴백 `\` / `"` 이스케이핑 (`_r` 패턴)
- **polish**: `python[0-9.]*` 정규식 — python3.11, python3.12 등 버전 명시형 차단 확장

### 📚 Documentation
- `docs/antipattern-hooks-install.md`: 차단 도구 테이블 갱신 + bats 테스트 수 업데이트 (4→10)
- `AGENTS.md`: Go 테스트 수 업데이트 (c4-core ~1,582 + c5 ~215 = ~1,797, 합계 ~2,581)

---

## [v0.38.0] - 2026-02-28

### ✨ Features
- **auth**: `ensureCloudAuth` Hub-aware 로그인 프롬프트
  - Hub URL 설정 시 `[y/d/N]` 프롬프트 (y=링크, d=디바이스 코드)
  - Hub 미설정 시 기존 `[y/N]` 브라우저 OAuth fallback 유지
  - `yesAll=true` + Hub 설정 → 자동으로 Link 모드 선택
  - `authLoginFunc(mode string)` — `""` (브라우저) / `"link"` / `"device"` 모드 분기
  - SSH/원격 환경에서 브라우저 없이 `d` 입력으로 Device Flow 바로 진입 가능

---

## [v0.37.0] - 2026-02-28

### ✨ Features
- **c5/auth**: RFC 8628 Device Flow 및 Direct Link Flow headless 인증 구현
  - `POST /v1/auth/device` — PKCE 기반 device session 생성 (user_code + activate_url + auth_url)
  - `GET /v1/auth/device/{state}` — 폴링 엔드포인트 (pending/ready/expired)
  - `POST /v1/auth/device/{state}/token` — PKCE 토큰 교환 (Supabase 프록시)
  - `GET /auth/activate` — user_code 입력 HTML 폼 (CSRF 보호)
  - `POST /auth/activate` — CSRF 검증 후 Supabase OAuth 리다이렉트
  - `GET /auth/callback` — auth_code 저장 + 성공 페이지
- **cli**: `cq login --device` / `cq login --link` 플래그 추가
  - `--device`: user_code 출력 → 브라우저에서 코드 입력
  - `--link`: auth URL 직접 출력 → 브라우저에서 열기
  - PKCE (S256 code_challenge) 자동 생성 및 관리
- **c5/store**: `device_sessions` 테이블 + CRUD 메서드
  - `PeekDeviceSession` — poll_count 부작용 없는 읽기
  - `IncrementTokenAttempts` — 트랜잭션으로 원자적 카운터 증가
  - `StartBackgroundCleanup` — 만료 세션 정기 삭제
- **c5/config**: `ServerConfig.PublicURL` 추가 (OAuth 리다이렉트용 외부 URL)

### 🐛 Bug Fixes (Security Polish)
- **c5/auth**: SSRF 방지 — 서버 `supabaseURL` 설정 시 클라이언트 제공 URL 무시 (HIGH)
- **c5/auth**: rate-limit off-by-one — `>= tokenAttemptLimit` 로 수정 (HIGH)
- **c5/auth**: `GetDeviceSessionByUserCode` 만료 필터 누락 수정 (HIGH)
- **c5/auth**: `IncrementTokenAttempts` UPDATE+SELECT 트랜잭션으로 원자화 (HIGH)
- **c5/auth**: 토큰 교환 시 `ds.SupabaseURL` fallback 추가 (HIGH)
- **c5/auth**: TOCTOU 해소 — `IncrementTokenAttempts` 후 세션 상태 재읽기 (MEDIUM)
- **c5/auth**: CSRF Secure 쿠키 플래그 HTTPS 시 자동 설정 (MEDIUM)
- **c5/auth**: HTML XSS — `html.EscapeString` on title/csrfToken (MEDIUM)
- **c5/auth**: CSRF 쿠키 POST 성공 후 즉시 만료 (MEDIUM)
- **c5/auth**: `code`/`state` 파라미터 길이 검증 (MEDIUM)
- **c5/auth**: `DeleteExpiredDeviceSessions` `expires_at` 기준으로 통일 (MEDIUM)
- **c4-core/cloud**: JSON key `device_auth_url` → `activate_url` 수정 (MEDIUM)
- **c5/serve**: `StartBackgroundCleanup` 미호출 수정 (LOW)

### 📚 Documentation
- **agents**: Go 테스트 수 현행화 — c4-core ~1,463 + c5 ~214 = ~1,677 (총 ~2,466) (`d31beea9`)

---

## [v0.35.1] - 2026-02-27

### 📚 Documentation
- **agents**: LOC 및 테스트 수 현행화 — c4-core ~107.7K LOC, 전체 ~179.0K LOC, Go 테스트 ~1,970 (총 ~2,759) (`541098e9`)

---

## [v0.35.0] - 2026-02-27

### ✨ Features
- **c4-tasks**: 세션 격리 — `session_id` 컬럼 추가로 다중 Claude Code 세션 간 태스크 그래프 혼합 방지 (`7ea3c2e1`)
  - `CQ_SESSION_NAME` env var 또는 `pid-<PID>` fallback으로 세션 ID 자동 부여
  - `AddTask` / `AssignTask` / `ListTasks` / `GetStatus` / `StaleTasks` 5곳에 세션 필터 적용
  - `session_id=''` 레거시 태스크는 모든 세션에서 접근 가능 (하위 호환)
  - `WithSessionID` Store 옵션 + `sessionClause/sessionArgs` 헬퍼
  - 세션 격리 테스트 5개 추가 (TestSessionIsolation_*)
  - Go 테스트: ~1,447 → ~1,452 (37 packages all pass)

---

## [v0.34.0] - 2026-02-27

### ♻️ Refactoring
- **handlers**: Wave 2b — drivehandler/cdphandler/llmhandler/eventbushandler 서브패키지 분리 (`9e41ff5d`)
- **handlers**: Wave 2c — c1handler/gpuhandler/knowledgehandler/researchhandler 서브패키지 분리 (`aab3d109`)
  - handlers/ God Package 해체 완료: 16개 서브패키지 (Wave 1a–1c + Wave 2a–2c)
  - c4-core 테스트: ~1,447 (37 packages), Go 전체: ~1,640

### 🔒 Security
- **eventsink**: bearer token 비교에 `crypto/subtle.ConstantTimeCompare` 사용 — timing side-channel 방지 (`4893cd80`)
- **eventsink**: `127.0.0.1` localhost-only binding, `MaxBytesReader(1MB)` 추가 (`cf001252`)
- **c1handler**: HTTP 응답 body에 `io.LimitReader(64KB)` 추가 — unbounded read 방지 (`cf001252`)
- **eventsink**: `ReadHeaderTimeout(10s)` + `ReadTimeout(15s)` — slow-read goroutine 방지 (`4893cd80`)

### 🐛 Bug Fixes
- **c1handler**: `httpGet` defer-in-loop 제거 + 명시적 `Body.Close()` — double-close + goroutine leak 방지 (`cf001252`)
- **researchhandler**: `UpdateProject`/`UpdateIteration` 반환 에러 체크 추가 (`cf001252`)
- **research_stub**: `research` 패키지 import 제거, `eventbus.Publisher` 타입 통일 — 빌드 태그 의미 복원 (`cf001252`)
- **knowledgehandler**: `experimentRecordNativeHandler` — `SyncAfterRecord` 호출 시 VectorStore에서 embedding 추출 전달 — Supabase pgvector 누락 수정 (`aae03b0d`)

### 📚 Documentation
- **agents**: Go 테스트 수 업데이트: c4-core ~1,447 (37 packages), 전체 ~1,640 (`3f0e2963`)

---

## [v0.33.0] - 2026-02-27

### ✨ Features
- **auth**: `cq auth login --no-browser` 플래그 추가 — headless/tunnel 환경에서 URL 출력 + SSH 포워딩 힌트, 브라우저 미오픈 (`0f8f0a08`)
- **serve**: `cq serve` 시작 요약 출력 및 `cq stop`에 OS 서비스 포함 (`d9ab13d9`)
- **archtest**: 아키텍처 테스트 인프라 — `helpers.go`, 의존성 방향 5개, 네이밍 규칙 3개, 인터페이스 패턴 3개, fmt.Errorf %w ratchet (396개 위반 베이스라인 캡처) (`1491c9b7`–`6c6823c5`)
- **arch**: 6개 인터페이스 쌍 컴파일타임 어서션 추가 (`b12e26a9`)
- **arch**: Wave 1c — mailhandler/artifacthandler 서브패키지 분리 (`146b5e5e`)

### 🐛 Bug Fixes
- **doctor**: `checkOSService`에서 `newServiceConfig` 재사용으로 `UserService` 옵션 누락 수정 — macOS/Linux user-level 서비스 설치 시 spurious WARN 방지 (`2b476afd`)
- **archtest**: `internal/knowledge`를 cloud 허용 의존성에 추가 (`47085c27`)

### ♻️ Refactoring
- **handlers**: Wave 2a — hubhandler 서브패키지 분리 (`cee7b1f3`)
- **arch**: Wave 1b — cfghandler/secrethandler 서브패키지 분리 (`33f10b13`)
- **handlers**: Wave 1a — fileops/gitops/webcontent 서브패키지 분리 (`7f2eeb20`)

### 🔧 Polish
- **auth**: `AuthClient.callbackTimeout` 필드 주입 — 테스트에서 goroutine leak 방지 (`5e635fbd`)

### 📚 Documentation
- **user**: `--no-browser` 플래그 사용법 user/README.md 2곳 추가 (`3dc64b7b`)
- **archtest**: 서브패키지 크로스 임포트 가드 테스트 추가 (`0a329888`)

---

## [v0.32.0] - 2026-02-27

### ✨ Features
- **serve**: `cq serve install` 이제 sudo 없이 user-level 설치 — macOS: `~/Library/LaunchAgents/`, Linux: `~/.config/systemd/user/` (`31572615`)
- **completion**: `cq completion [bash|zsh|fish]` 서브커맨드 추가 — 셸 탭 자동완성 스크립트 생성 (`31572615`)
- **completion**: `-t` 플래그에 named session 이름 탭 자동완성 지원, `cq init` 및 `install.sh` 에서 RC 파일 자동 추가 (`31572615`)
- **hub**: cq serve가 C5 서브프로세스에 `C5_SUPABASE_URL`/`C5_SUPABASE_KEY` 환경변수 자동 주입 (`8727fc92`)
- **c5**: `applyEnvOverrides` — `C5_SUPABASE_URL/KEY/PORT` 환경변수 오버라이드 지원 (`6bcff929`)

### 🐛 Bug Fixes
- **init**: `ensureServeRunning` + `isCQServeProcess` 추가 — `~/.c4/serve/serve.pid` 기반 serve 자동 시작·검증, `--no-serve` 플래그 지원 (`0473f82b`)
- **auth**: `ensureCloudAuth` — `yesAll` 모드 지원, solo 모드 자동 통과 (`131d389a`)

### 📚 Documentation
- **guide**: 설치 가이드에 Shell 자동완성 및 OS 서비스 등록 섹션 추가 (`4893c8ce`)
- **reference**: 명령어 레퍼런스에 cq session·completion·serve·misc 전체 섹션 추가 (`4893c8ce`)
- **install**: `install.sh` — `add_completion()` 으로 zsh/bash RC 파일에 completion 자동 등록 (`4893c8ce`)
- AGENTS.md: 테스트 수 업데이트 (~1,609/~2,350), HubComponent credential passthrough 문서 추가 (`f1b60d5a`)

### 🔧 Polish
- `serve status` PID 파일 경로 오류 처리 강화, stale pid 삭제 실패 경고 출력 (`57948926`, `2c8d321c`, `31572615`)
- **auth**: `t.Setenv` 로 env var 테스트 정리 자동화, `serveHealthURL` 상수 추출 (`bf092167`)
- **init**: HOME 격리 + `servePIDPath` 주입으로 테스트 결정론적 실행, darwin 프로세스 매칭 정밀화 (`3416a947`)

---

## [v0.31.0] - 2026-02-27

### ✨ Features
- **embed**: c5 바이너리를 cq 내부에 내장 (`c5_embed` 빌드 태그) — PATH에 c5 없으면 `~/.c4/bin/c5`로 자동 추출 (`b4066971`)
- **ci**: c5 빌드 전용 `build-c5` CI 스테이지 추가, `embed-c5` Makefile 타겟, `embeddedC5Version` ldflags 주입 (`1320a824`)
- **init**: `cq init` 시 OS 서비스 등록 프롬프트(`confirmServeInstall`) 추가, `templates/config.yaml`에 serve.hub 섹션 문서화 (`620d5ea7`)
- **session**: named session에 `tool` 라벨 및 `memo` 필드 추가 (`753b8007`)
- **session**: `cq session name`에 `--uuid` 플래그 추가 (`23edc723`)
- **skill/c4-attach**: `memo` 인수 지원 (`c76f5b7a`)
- **lsp**: Rust write 지원 활성화 (rust-analyzer), session name 충돌 가드 (`4be64649`)

### 🐛 Bug Fixes
- **embed**: 중복 `c5_embed.go`/`c5_embed_stub.go` 제거 (T-EMBED-002의 `embed_c5.go`로 대체) (`6a130355`)

### ♻️ Refactoring
- **serve**: `installServeService()` 공개 함수 추출 — `runServeInstall()`에서 위임, `confirmServeInstall()`과 공유 (`56f1a830`)

### 🔧 Polish
- ldflags 변수명 불일치 수정 (`embeddedVersion` → `embeddedC5Version`), version tmpfile `defer os.Remove` 추가, 테스트 어설션 강화 (`f6b4beb2`)
- Makefile 주석 및 `installServeService` godoc 정확도 수정 (`f5d72f82`)

### 📚 Documentation
- AGENTS.md: 테스트 수 업데이트 (~1,595/~2,345), c5 embed 패턴 섹션 추가 (`46330599`)

---

## [0.30.0] - 2026-02-27

### ✨ Features
- **serve**: `HubComponent` — C5 Hub 바이너리를 자식 프로세스로 관리 (`654da19e`)
  - `cq serve` 시작 시 `c5` 바이너리를 자동 실행 (PATH에 없으면 WARN 후 건너뜀)
  - SIGTERM → 5초 대기 → SIGKILL 우아한 종료 시퀀스
  - `GET /health` 응답에 hub 컴포넌트 상태 포함
  - 설정: `serve.hub.{enabled,binary,port}` (`.c4/config.yaml`)
- **c5**: WebSocket 메트릭 증분 조회 — `GetMetrics(minStep)` 인터페이스 (`b416a05f`)
- **c5**: GPU VRAM 매칭 + fallback 제어 — 요구 VRAM 초과 시 CPU 폴백 설정 가능 (`455c1115`)
- **c5**: Artifact 업로드 타임아웃 제거 + 설정 가능 제한 + SSE keepalive (`ab7f796d`)
  - 대용량 artifact 업로드 시 무한 대기 방지 제거 (Worker 측 30분 타임아웃으로 대체)
  - SSE 스트림 keepalive ping 추가
- **c5**: job_logs/metrics 보존 정책 — 주기적 회전 + 정리 (`23f2cdc2`)
- **c5/worker**: renewLease 에러 로깅 + 3회 연속 실패 시 WARN (`b5f1e111`)
- **gemini**: 특화 에이전트 동기화 및 워크플로우 강화 (`872c7ffe`)
- **codex**: c4 에이전트 확장 및 워크플로우 하드닝 (`2b0e74a4`)

### 🐛 Bug Fixes
- **c5/store**: heartbeat 인식 리스 만료 + 트랜잭션 안전성 (`380b80c7`)
  - 리스 만료 시 heartbeat 타임스탬프도 함께 고려하여 spurious expiry 방지
- **sessions**: `CQ_SESSION_UUID` 환경변수 우선 사용 (`87d5b24c`)
- **sessions**: 동일 UUID의 이전 이름 자동 제거 (`d60212f5`)

### 🔧 Polish
- **serve**: `hub_component` double-Wait race 수정 — 단일 reaper goroutine + `done` channel 패턴 (`e0ecf5ea`)
- **serve**: `hub_component` Stop() mutex 해제 후 대기 — Health() 블로킹 방지 (`59189e6b`)
- **serve**: Health check URL `/health` → `/v1/health` (c5 라우트 매칭) (`7a6072a5`)
- **c5**: `WriteTimeout: 0` → `10 * time.Minute` — 무한 hang 방지 (`59189e6b`)
- **c5**: dual retention 메커니즘 문서화 (row-count + time-based) (`59189e6b`)
- **hub**: `VRAMRequiredGB` 3-point 계약 동기화 (c5 model ↔ hub/models.go ↔ MCP schema) (`6cf0d074`, `b5d3ab7d`)
- **config**: `serve.hub.args` SetDefault 추가 + `gpu_worker_gpu_only` ExampleConfigYAML (`6cf0d074`, `59189e6b`)
- **c5/worker**: artifact HTTP 클라이언트 타임아웃 `0` → `30분` (`e0ecf5ea`)

### 📚 Documentation
- **agents**: Core Agent Principles 추가 — Karpathy 4원칙 내재화 (`f795982a`)
- **c4-plan**: Goal-Driven 패턴 + Assumptions starter DoD 가이드 추가 (`3995f52e`)
- **architecture**: AGENTS.md 참조 섹션 → `docs/ARCHITECTURE.md` 분리 (`e45558a6`)
- **architecture**: serve.hub 컴포넌트 문서화, Go 테스트 수 업데이트 (`f4a4591a`)
- **c4-help**: 스킬 수 19 → 22 교정 (`9a2661e0`)

---

## [0.29.0] - 2026-02-25

### ✨ Features
- **mail**: 세션 간 메일 시스템 — `c4_mail_*` MCP 도구 + `cq mail` CLI (`3542f60e`, `c9e966e4`)
  - `~/.c4/mailbox.db` SQLite 기반 전역 메일 저장소 (프로젝트 독립)
  - MCP 도구: `c4_mail_send`, `c4_mail_ls`, `c4_mail_read`, `c4_mail_rm`
  - CLI: `cq mail send <to> <body>`, `cq mail ls [--unread]`, `cq mail read <id>`, `cq mail rm <id>`
  - `cq ls` (세션 목록): 읽지 않은 메일 있으면 `[N unread]` 접미사 표시
  - 브로드캐스트(`to="*"`) 수신 지원 — `UnreadCount`·`List` 쿼리가 직접 메시지 + 브로드캐스트 합산
  - 발신자 기본값: `CQ_SESSION_NAME` 환경변수
- **sessions**: `/reboot` 플래그 기반 자동 종료 — `kill -TERM $PPID` 후 재시작 (`3c6a5e24`)

### 🔧 Polish
- **mail**: `Send()` 단일 클럭 읽기로 `created_at` 반환 — 타임스탬프 불일치 제거 (`de57611c`)
- **mail**: `NewMailStore` 에 `os.MkdirAll` + `busy_timeout=30000` 추가 (`de57611c`)
- **mail**: CLI·MCP 양쪽에서 빈 body 거부 (`de57611c`, `41c19238`)
- **mail**: `List()` 전 쿼리 변형에 `LIMIT 500` 안전 캡 추가 (`41c19238`)
- **mail**: MCP `c4_mail_send`에서 `to="*"` 거부 (CLI와 동일한 브로드캐스트 제한) (`603118f4`)
- **mail**: `c4_mail_read`·`c4_mail_rm`에서 `id<=0` 명확한 에러 반환 (`603118f4`)

---

## [0.28.3] - 2026-02-24

### ✨ Features
- **sessions**: tmux 스타일 named Claude Code 세션 (`072823f3`, `1288b234`)
  - `cq claude -t <name>`: 세션 이름 지정 — 첫 실행 시 UUID 자동 감지·저장, 재실행 시 `claude --resume` 자동 실행
  - `cq ls` (alias: `cq sessions`): tmux 스타일 목록 출력 — `name: (created DATE) [dir] uuid=XXXX (current)`
    - `CQ_SESSION_NAME` env var로 현재 세션 자동 감지 및 `(current)` 표시
  - `/reboot` 스킬: `~/.c4/.reboot` 플래그 작성 → `/exit` 후 cq가 동일 UUID로 즉시 재시작
  - reboot loop: resume도 subprocess 방식으로 전환하여 부모 프로세스 유지
  - `CQ_SESSION_NAME` / `CQ_SESSION_UUID` 환경변수를 claude 서브프로세스에 자동 주입
  - JSONL 파일 삭제 시 자동으로 새 세션 생성

---

## [0.28.2] - 2026-02-24

### 🐛 Bug Fixes
- **knowledge**: `c4_knowledge_search` / `c4_pattern_suggest` 호출 시 nil pointer dereference 패닉 수정 (`b3ffa06f`)
  - cloud 미설정 환경(solo tier)에서 nil `*cloud.KnowledgeCloudClient`를 `knowledge.CloudSyncer` interface에 직접 대입하면 typed nil interface 생성
  - `opts.Cloud != nil`이 TRUE로 평가되어 `DiscoverPublic()` 호출 시 panic → `MCP error -32000` 발생
  - Fix: concrete 포인터가 non-nil일 때만 interface 필드에 대입

---

## [0.28.0] - 2026-02-24

### ✨ Features
- **ci**: 릴리즈 바이너리에 Supabase URL/Key를 ldflags로 embed (`SUPABASE_URL`, `SUPABASE_ANON_KEY` env 주입) (`772efa73`) [T-976-0]
- **auth**: `cq auth login` 성공 시 `.c4/config.yaml` cloud 섹션 자동 패치 (`2c2031fa`) [T-977-1]
  - `enabled: true`, `url`, `anon_key`, `mode: local-first` 자동 설정
  - `.c4/` 디렉토리 없으면 graceful skip (init 전 로그인 허용)
  - 사용자 커스텀 `cloud.url`/`anon_key` 보호 (덮어쓰기 방지)
  - auth 서브커맨드가 `.c4/` 없이도 실행되도록 `PersistentPreRunE` 예외 추가
- **hub**: `hub.api_key` 미설정 시 cloud session JWT 자동 폴백 (`1267f959`) [T-978-0]
  - `SetTokenFunc(tokenFunc func() string)` — per-request token 갱신 지원
  - `mcp_init_hub.go`에서 `cloudTP.Token() != ""` 조건 충족 시 자동 주입
- **init**: `cq init` 완료 후 cloud 인증 상태 체크 + 안내 메시지 (`6400f7e9`) [T-979-0]
  - 로그인 상태: `✓ Cloud: <email> (expires in Xh)` 출력
  - 미로그인: `→ Run 'cq auth login' to enable cloud sync & hub access` 안내
  - URL 미설정(solo tier): 무출력

### 🔧 Polish
- **auth**: YAML key prefix collision 수정 — `cloudYAMLValue` + `writeCloudSectionToYAML` (`0f66e4bf`)
  - `HasPrefix(trimmed, key)` → `trimmed == key || HasPrefix(trimmed, key+" ") || HasPrefix(trimmed, key+"\t")` (doctor.go 패턴 통일)
- **hub**: cloud token guard 강화 — `ctx.cloudTP != nil` → `ctx.cloudTP.Token() != ""` 추가

### 📚 Documentation
- **agents**: 테스트 수 현행화 (~1,368 → ~1,375, 온보딩 배치 +21 테스트)

---

## [0.27.0] - 2026-02-24

### ✨ Features
- **serve**: `StaleChecker` 컴포넌트 — 주기적으로 stale(`in_progress`) 태스크를 `pending`으로 리셋 (`f20526b8`)
  - `serve.stale_checker.enabled`, `threshold_minutes`(기본 30), `interval_seconds`(기본 60) 설정
  - `StaleTaskStore` 인터페이스 + `*handlers.SQLiteStore` 구현체 연결
  - EventBus 연동: 리셋 성공 시 `task.stale` 이벤트 발행 (`task_id`, `worker_id`, `stale_minutes`)
  - `WithCloser(db)` 패턴: `Stop()` 시 `*sql.DB` 자동 닫힘 (리소스 누수 방지)
  - `tickerFn` 주입으로 단위 테스트 완전 제어 가능 (`controlledTicker` 패턴)
- **serve**: OS 서비스 자동 시작 통합 — macOS LaunchAgent / Linux systemd / Windows Service (`f20526b8`)
  - `cq serve install` / `cq serve uninstall` / `cq serve status` 서브커맨드
  - `kardianos/service v1.2.4` 기반 크로스플랫폼 지원
  - `cq doctor` `os-service` 항목: 설치/실행 상태 + PID liveness 확인
- **init**: WebFetch, WebSearch를 기본 `permissions.allow` 목록에 추가 (`83163008`)

### 🐛 Bug Fixes
- **serve**: `serve_service.go` 병합 충돌 해결 (`051bb341`)

### 🔧 Polish
- **stale_checker**: `run()` goroutine에 `defer recover()` 추가 (panic 전파 방지) (`6b03348c`)
- **stale_checker**: `runServeStatus`, `checkOSService`의 PID liveness check (`Signal(0)`) 추가 (`3d7258ed`)
- **stale_checker**: `TestStaleChecker_WithCloser_ClosedOnStop`, `TestStaleChecker_ResetTask_PartialFailure` 테스트 추가

### 📚 Documentation
- **agents**: StaleChecker + OS 서비스 내용 추가, 테스트 수 현행화 (~1,355 → ~1,368, +13) (`6d208c1f`)

---

## [0.26.0] - 2026-02-23

### ✨ Features
- **knowledge**: `KnowledgeHitTracker` — 세션별 지식 검색 히트/미스 카운터 (`ab9f53d5`)
  - `enrichWithKnowledge()` 호출마다 결과 수 기록 (hits: >0, misses: 0)
  - `c4_status` 응답에 `knowledge_search_stats` 필드 추가 (`total_searches`, `hits`, `misses`, `hit_rate`)
  - `llm_gateway` 빌드 태그 없이도 접근 가능 (CostTracker와 동일한 패턴)
  - O(1) 메모리 카운터 방식 (`hits/misses int` — 슬라이스 아님), `sync.Mutex` 동시성 안전

### 📚 Documentation
- **agents**: 테스트 수 현행화 — KnowledgeHitTracker +7 (~1,348 → ~1,355) (`3efb506b`)

---

## [0.25.0] - 2026-02-23

### ✨ Features
- **llm**: LLM 캐시 히트율 경보 — `GlobalCacheHitRate`가 설정 임계값 미만으로 최초 진입 시 `llm.cache_miss_alert` EventBus 이벤트 발행 + slog.Warn 출력, 회복 시 상태 리셋 (`0f18951f`)
  - `CacheAlertPublisher` 인터페이스 (circular import 방지, eventbus.Publisher로 구현)
  - `Gateway.SetCacheAlert(threshold, pub, projectID)` API
- **llm**: Anthropic tools 목록 캐싱 — `ChatRequest.CacheTools=true` 설정 시 마지막 tool에 `cache_control: {type:"ephemeral"}` 자동 부착 (`3e11da4a`)
  - `Tool` struct + `ChatRequest.Tools []Tool` + `ChatRequest.CacheTools bool` 필드 추가
  - beta 헤더 조건 확장: `CacheSystemPrompt || CacheTools`
- **skill**: WORKER_PROMPT prefix 안정화 — worker_id를 첫 줄에서 `## Identity` 섹션(끝)으로 이동 (`66e72d89`)
  - 동시에 스폰된 워커들이 공통 프롬프트 prefix를 공유 → Anthropic 프롬프트 캐싱 효율 향상

### 🐛 Bug Fixes
- **llm**: `checkCacheAlert` 이중 nil-check 버그 수정 — lock 획득 후 local 변수 `mon` 대신 `g.alert` 필드 직접 재읽기 (`4c959d43`)

### 📚 Documentation
- **llm**: `LLM-설정.md` 캐시 효율 가이드라인 추가 (`9b002e40`)
  - 올바른/잘못된 파라미터 설계 패턴 (dynamic 데이터를 system에 넣으면 캐시 무효화)
  - 캐시 포인트 표 (system 프롬프트·tools·messages), 실측 임계값 2048토큰 기록
- **agents**: Go 테스트 수 ~1,339 → ~1,348 (캐싱 배치 +9) (`67c159d7`)

---

## [0.24.0] - 2026-02-23

### ✨ Features
- **hooks**: 프로젝트 단위 PreToolUse + PermissionRequest 2-layer hook 재설계 (`82179903`)
  - 기존 전역 `~/.claude/hooks/` 방식 제거 (절대경로 고정 → 다른 사용자/컴퓨터 동작 불가 문제 해결)
  - `cq init` 시 `{project}/.claude/hooks/c4-gate.sh` + `c4-permission-reviewer.sh` 배포
  - `$CLAUDE_PROJECT_DIR` 사용 → 경로 하드코딩 없이 어느 환경에서도 동작
  - **c4-gate.sh** (PreToolUse): 룰 기반 게이트. allow/block 패턴 매칭, gray-zone은 PermissionRequest로 위임
  - **c4-permission-reviewer.sh** (PermissionRequest): Haiku API 판단 (Bash|Read|Edit|Write|... 전체 커버)
  - Edit/Write 보안 우선순위: built-in allow(`.c4/`, `/tmp/`) → user allow → user block → built-in block

### 🔧 Chores
- **init**: `patchHookEvent` 3-phase 재작성 — baseName scan(Phase1) → matcher match(Phase2) → append(Phase3)
  이전 버전의 다른 matcher로 등록된 stale entry를 중복 없이 in-place 업데이트 (`23fb393d`)
- **init**: stale-path 업데이트 시 전체 hookEntry 교체 — timeout 필드 유실 방지 (`23fb393d`)
- **hooks**: fallback echo 경로에서 단일따옴표 sanitize (`${reason//\'/}`) — jq 미설치 환경 대비 (`807ec145`)

### 🧪 Tests
- **init**: `TestPatchProjectSettings_StaleEntry` 추가 — Phase 1 stale-entry 업그레이드 경로 검증 (`807ec145`)

### 📚 Documentation
- **AGENTS.md**: hook 설치 항목 업데이트 — 구 전역 경로 → 신 프로젝트 단위 경로 (`23fb393d`)

---

## [0.23.0] - 2026-02-22

### ✨ Features
- **serve/agent**: A2UI 응답 라우팅 — 버튼 클릭 시 `cq serve agent`가 감지하고 `claude -p` 호출 (`a7857afa`)
  - `msgRequest` struct으로 `actionID` 전달 (positional args 대체)
  - `fetchChannelContext` — Supabase REST로 채널 최근 메시지 조회 (context 전파, 10s timeout)
  - `buildA2UIPrompt` — 원본 A2UI 메시지 기반 컨텍스트 포함 프롬프트 빌드 (라벨 폴백)
  - loop prevention: `sender_type == "agent"/"system"` 필터 A2UI 경로에도 적용

### 🔧 Chores
- **init**: `patchClaudeSettings` 3-arg 확장 — edit hook 지원 (`editHookPath` 추가), empty-path guard (`468d5dc8`)
- **init**: edit hook 설치 시 `hookNeedsUpdate` boolean을 write 전 캡처 — "hooks up-to-date" 오탐 수정 (`f8d61cc3`)
- **security**: `.c4/supabase/.temp/` git 추적 제거 (`7e76fa43`)
- **submodule**: `user/` README one-liner install 업데이트 (`23a1031b`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,330 → ~1,339 (A2UI routing +9 tests) (`7fa713d6`)
- **templates**: `c4-edit-security-hook.sh` 추가 (`7fa713d6`)

## [0.22.1] - 2026-02-22

### 🐛 Bug Fixes
- **worker**: `autoRecordFailurePattern()` proxy fallback 누락 수정 — `knowledgeWriter` nil 시 `proxy`로 폴백 (기존 `autoRecordKnowledge` 패턴 일치) (`e2308ae`)
- **worker**: `autoRecordFailurePattern()` native-writer 경로에 10s timeout 추가 — goroutine 무한 대기 방지 (`e96e6f9`)
- **worker**: `autoRecordFailurePattern()` 제목에 `task.Title` 사용 / 빈 Scope 태그 guard — 빈 scope 시 trailing space 및 빈 태그 오염 방지 (`e2308ae`)
- **worker**: `enrichWithReviewContext()` `sql.ErrNoRows` spurious stderr 억제 — parent T 미존재 시 정상 케이스를 에러 로그로 출력하던 문제 수정 (`e2308ae`)
- **worker**: `autoRecordFailurePattern()` body Markdown 형식으로 변경 — `## Failure Pattern:` 헤더 추가 (`e96e6f9`)

### 🧪 Tests
- **handlers**: `store_review_test.go` dead `setupReviewTask` stub 제거 (`e96e6f9`)
- **handlers**: `sqlite_store_enrich_test.go` `db.Exec` 에러 체크 추가 (`e96e6f9`)

---

## [0.22.0] - 2026-02-22

### ✨ Features
- **worker**: `enrichWithReviewContext()` Evidence 주입 — R- 태스크 assign 시 parent T의 handoff JSON에서 `HandoffEvidence[]` 파싱, `ReviewContext.Evidence` 필드에 주입하여 리뷰어가 스크린샷·로그·테스트결과 아티팩트를 직접 참조 가능 (`e08d4b4`)
- **worker**: `autoRecordFailurePattern()` — `MarkBlocked()` 호출 시 `failureSignature` 비어있지 않으면 goroutine + 10s timeout으로 `CreateExperiment` 자동 호출, `scope/signature/last_error` 구조화된 실패 패턴을 Knowledge 실험으로 기록 (`e08d4b4`)
- **review**: `RequestChanges()` RPR DoD에 Past Solutions 첨부 — `searchPastSolutions()` 가 Knowledge Base에서 관련 패턴 최대 3개 조회 (2s context timeout, 150 rune 트런케이션), RPR 태스크 DoD 하단에 `## Past Solutions` 섹션 자동 첨부 (`af7f413`)
- **knowledge**: `KnowledgeRecord()` pgvector 업로드 — 문서 임베딩 생성 후 `infra/supabase` pgvector 테이블에 자동 업로드, Supabase 미연결 시 graceful no-op (`f38c38c`)

### 🧪 Tests
- **handlers**: `TestEnrichWithReviewContext_IncludesEvidence_WhenPresent`, `TestEnrichWithReviewContext_EmptyEvidence_WhenAbsent` (enrich_test.go 신규) (`e08d4b4`)
- **handlers**: `TestMarkBlocked_AutoRecordsFailurePattern_WhenSignatureNonEmpty`, `TestMarkBlocked_NoKnowledgeRecord_WhenSignatureEmpty`, `TestAutoRecordFailurePattern_ContentContainsScope` (auto_test.go 신규) (`e08d4b4`)
- **handlers**: `TestRequestChanges_RPR_AppendsPastSolutions_WhenFound`, `TestRequestChanges_RPR_NoPastSolutions_WhenNotFound`, `TestRequestChanges_RPR_PastSolutionsFormat_TruncatesLongBody` (`af7f413`)
- Tests: 1,322 → 1,330 (+8)

---

## [0.21.0] - 2026-02-22

### ✨ Features
- **agent**: `AgentConfig.ProjectDir` — `claude -p --dir <projectDir>` 전달로 올바른 프로젝트 컨텍스트 보장; `cq serve`·MCP-embedded 모드 모두 적용 (`081f783`)
- **agent**: `updateMemberPresence(status)` 비동기 알림 — `@cq` claim 직후 `c1_members.status="typing"` PATCH로 C1 Messenger에 즉시 피드백, 응답 완료 후 `"online"` 복원 (`081f783`)

### 🧪 Tests
- **agent**: `TestAgent_UpdateMemberPresence_{Success,NoMemberID,ServerError}`, `TestAgent_ProjectDirConfig` +4개 (1,318 → 1,322) (`081f783`)

---

## [0.20.0] - 2026-02-22

### ✨ Features
- **store**: `superseded_by` 컬럼 추가 — `RequestChanges()` 시 이전 R- 태스크를 superseded 마킹, `reassignStaleOrFindPendingTask()` superseded 태스크 자동 필터, scope 상속 — 신규 T-/R- 태스크가 부모 T- 태스크의 `scope` 필드 계승 (`39011c7`)
- **handoff**: `HandoffEvidence` 구조체 + 타입 검증 — `SubmitTask()`에서 `screenshot/log/test_result` 외 타입 거부, `evidence` 필드 누락 시 backward-compatible 처리 (`45260d0`)

### 🧪 Tests
- **store**: `TestRequestChanges_SetsSupersededBy`, `TestRequestChanges_ScopeInherited`, `TestAssignTask_SkipsSupersededReviews`, `TestAssignTask_SkipsSupersededPendingReviews` (4개) (`39011c7`)
- **handoff**: `TestSubmitTask_Evidence_Valid`, `TestSubmitTask_Evidence_InvalidType`, `TestSubmitTask_Evidence_NoEvidence`, `TestSubmitTask_Evidence_EmptySlice` (4개) (`45260d0`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,310 → ~1,318 반영 (`f69e5b9`)

---

## [0.19.0] - 2026-02-22

### ✨ Features
- **handlers**: `classifyTaskRisk` + `AssignTask` risk routing — R- 태스크에 scope 기반 모델 자동 선택 (high/low/default 티어) (`4867fa9`)
- **config**: `RiskRoutingConfig` 스키마 추가 — `risk_routing.enabled`, `paths.high/low`, `models.high/low/default` 필드 + viper defaults + `GetRiskRouting()` value return (`16603af`)

### 🐛 Bug Fixes
- **c2/workspace**: `slug[:40]` byte slice → `string([]rune(slug)[:40])` rune slice — 한국어 title truncation 시 UTF-8 경계 파괴 수정 (`b95ba44`)
- **c2/handlers**: `workspaceSaveHandler`/`profileSaveHandler` — `json.Unmarshal` 에러 무시 → 명시적 에러 반환으로 "invalid arguments" 원인 노출 (`a3e26f6`)
- **c2/persona**: `RunPersonaLearn` — `LoadProfile` 반환 nil 프로파일에 nil guard 추가 + malformed YAML 시 `slog.Warn` 로그, autoApply panic 방지 (`bfb6e39`)
- **c2/persona**: `detectToneSoftening` hardcoded 어조 단어 → `profile.yaml` `learned_patterns.tone_assertive/tone_soft` 외부화, `AnalyzeEdits` 공개 API 시그니처 유지 (`bfb6e39`)

### 🧪 Tests
- **c2**: `TestParseDiscoverSection_KoreanSlug` — 40/41 rune 경계, UTF-8 유효성 검증 (`b95ba44`)
- **c2/handlers**: `TestWorkspaceSaveHandler_InvalidJSON`, `TestProfileSaveHandler_InvalidJSON` 추가 (`a3e26f6`)
- **c2/persona**: `TestAnalyzeEditsWithWords_CustomDict`, `TestRunPersonaLearn_ProfileDict`, `TestRunPersonaLearn_NoProfile`, `TestRunPersonaLearn_MalformedYAMLAutoApply` 추가 (`bfb6e39`)
- **handlers**: `TestClassifyTaskRisk` (11 케이스) + `TestAssignTask_RiskRouting_*` (6 케이스) — directory/glob/substring 매칭 + AssignTask 통합 (`4867fa9`)
- **config**: `TestRiskRouting` 4 서브테스트 — defaults, YAML 파싱, EconomicMode 독립성, value return 안전성 (`16603af`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,293 → ~1,310 반영 (`1677b01`)

---

## [0.18.0] - 2026-02-22

### ✨ Features
- **cloud**: `cloud.mode` config 필드 + `cq cloud mode get/set` CLI — `"local-first"(기본, HybridStore)` vs `"cloud-primary"(CloudPrimaryStore)` 전환 지원 (`96992ae`)
- **cloud**: `CloudPrimaryStore` — Supabase 우선 쓰기 + `context.WithoutCancel` 기반 local async 미러 + compile-time `var _ store.Store` assertion (`96992ae`)

### 🔧 Chores
- **user**: submodule 업데이트 — user-facing workflow 문서 정리 (`529f1cb`)
- **docs**: 테스트 수 현행화 — c4-core ~1,293개, 28 패키지 (`57f7ece`)

---

## [0.17.0] - 2026-02-22

### ✨ Features
- **session**: 신규 내부 패키지 `c4-core/internal/session` — PID lock 파일 기반 활성 MCP 세션 추적, 다중 세션 감지 지원 (`d3fcf15`)

### 🐛 Bug Fixes
- **doctor**: `sectionYAMLValue` 추가 — cross-section 격리 YAML 파싱 (hub.url과 cloud.url 혼용 방지), 정확한 키 토큰 매칭으로 prefix collision 수정 (`f3d978b`, `a2866ec`, `59d5d38`)
- **gate**: `mcp_init_gate.go` Subscribe 실패 시 `context.Canceled` 필터 + cancel() 호출로 goroutine 누수 방지 (`f3d978b`, `a2866ec`)
- **guard**: PRAGMA busy_timeout 30000ms로 통일 — 메인 DB와 일관성 확보 (`f3d978b`)
- **lsp**: `native_lsp.go`에 `filepath.Clean()` 추가 — 상대 경로/trailing separator 처리 (`f3d978b`)
- **hook**: `--force-with-lease` 허용 로직 — if/elif 명시 가드로 ERE prefix 매칭 버그 수정 (`a2866ec`)
- **observe**: `MiddlewareWithPublisher` dead publisher block 제거, unused param 정리 (`f3d978b`)

### 🧪 Tests
- **guard**: `TestEngine_AuditOnly_NoPublish` 추가 — AuditOnly 시 PublishAsync 미발행 + audit log 기록 동시 검증 (`f3d978b`)
- **doctor**: `TestDoctor_SectionYAMLValue` — cross-section 격리 + prefix collision 회귀 테스트 추가 (`a2866ec`, `59d5d38`)
- **lsp**: `TestLanguageGuardPythonFile` nil map/interface 비교 버그 수정 (`f3d978b`)

### ♻️ Refactoring
- **skills**: `/c4-refine` → `/c4-polish` 통합 — plan→run→finish 3-step 워크플로우 완성, Plan Critique → Plan Refine 명명 개선 (`37ff4b8`, `e3b3e2a`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,271 → ~1,294 반영 (`1b26131`)
- **user**: submodule 업데이트 (plan→run→finish 워크플로우 문서 단순화)

---

## [0.16.0] - 2026-02-22

### ✨ Features
- **lsp**: `c4_find_symbol` 결과의 `symbols[]` 각 항목에도 `_edit_hint` 주입 — Agent가 심볼 항목을 꺼내 처리할 때도 편집 제약 인식 (`f0f023b`)

### 🐛 Bug Fixes
- **lsp**: `get_symbols_overview` overview 그룹 항목(`functions[]`/`methods[]`/`structs[]` 등)에 `_edit_hint` 누락 수정 (`9ae2846`)
- **lsp**: Dart `handleDartSymbolsOverview` 키 목록을 `dartast.kindGroup` 실제 키와 일치시킴 — `typedefs` 누락 + 존재하지 않는 키 제거 (`bfc5c0b`)
- **lsp**: `languageGuardedProxy` 응답에 `success:false` 추가 — 차단 응답을 성공과 명시적으로 구분 (`bfc5c0b`)
- **hook**: `.c4/` 디렉토리 walk-up 탐지 + fallback 패턴 + doctor --fix hook 업데이트 (`6d6f101`)

### 🧪 Tests
- **lsp**: Go overview 테스트에 `structs`/`methods`/`constants` 카테고리 검증 추가 (`ed8385b`)
- **lsp**: Dart overview 테스트에 `classes[]`/`functions[]` 항목 순회 검증 추가 (`bfc5c0b`)

### 📚 Documentation
- **skills**: polish 누락 방지 — run/refine/plan 스킬 3곳에 플로우 명시 (`1371548`)
- **skills**: TDD 원칙 강화 — 3-layer 안전망 + C1 테스트 커버리지 인프라 (`3cdbbe1`)

### 🔧 Chores
- **user**: submodule 업데이트 (workflow page cross-links, checkpoint explanation, workflow overview)

---

## [0.15.0] - 2026-02-22

### ✨ Features
- **lsp**: Go/Dart native handler 응답에 `_edit_hint` 필드 주입 — Agent가 find/overview 결과에서 편집 도구 제약을 즉시 인식 (`76d80cb`)

### 🐛 Bug Fixes
- **lsp**: `languageGuardedProxy` 도구명 PascalCase 버그 수정 — `toolName` 명시적 snake_case 전달, 미사용 `rootDir` 파라미터 제거 (`8dbfc5c`)

### 🧪 Tests
- **c1**: C1 Messenger-first 재설계 신규 컴포넌트 테스트 추가 (`d4efa3f`)
- **handlers**: `language_guard_test.go` (5개) + `lsp_hint_test.go` (4개) = Go +9 tests (`76d80cb`, `8dbfc5c`)

### 📚 Documentation
- **agents**: Go 테스트 수 ~1,262 → ~1,271 반영 (`59ca0df`)

### 🔧 Chores
- **user**: submodule 업데이트 (README user scenarios, distributed experiment scenario, Examples section, refine+polish workflow pages)

---

## [0.14.0] - 2026-02-22

### ✨ Features
- **eventbus**: C7 Observe → C3 EventBus 연결 — package-level publisher setter (`SetEventBus`), `tool.called` 이벤트 발행 (`c2f217d`)
- **eventbus**: C6 Guard → C3 EventBus 연결 — ActionDeny 시 `guard.denied` 이벤트 발행 (`2ce1df8`)
- **eventbus**: C8 Gate → C3 EventBus bridge — `task.completed`/`hub.job.completed` 구독 후 WebhookManager 트리거 (`adc7cdc`)
- **workflow**: c4-finish quality gate system — DB 기반 polish gate 검증, 세션 메모리 의존 제거 (`91192ff`)

### 📚 Documentation
- **agents**: C3 EventBus 이벤트 종류 16종 → 18종 (`tool.called`, `guard.denied` 추가) (`de3cf84`)

### 🔧 Chores
- **user**: submodule 업데이트 (docs: cloud config, polish 설치/사용법, 전체 문서 리뷰 수정)

---

## [0.13.0] - 2026-02-22

### ✨ Features

#### C1 Messenger — Messenger-first UI/UX 완전 재설계 (14 tasks)

- **3-column 레이아웃**: `WorkspaceNav`(48px 아이콘 네비) + `ChannelListArea`(240px) + `ContentArea`(flex-grow)
  - `WorkspaceNav.tsx`: 5개 워크스페이스 모드 아이콘 (`messenger/documents/settings/team/search`)
  - `MainLayout.tsx`: BEM 3-column CSS 레이아웃, `main-layout__nav/channel-list/content`
- **`channel_type` 스키마 정립** (migration `00024_c1_channel_types.sql`):
  - 5가지 타입: `general | project | knowledge | session | dm` (CHECK constraint)
  - `c1_channel_pins` 테이블: ProductView 버전 히스토리용
  - `agent_work_id` 컬럼: AgentThread 그루핑용
  - session 채널 전용 UNIQUE INDEX
- **`ChannelListSidebar v2`**: 5개 섹션 (General `#` / Projects `📂` / Knowledge `🧠` / Sessions `💬` / Direct `✉`)
  - 섹션별 collapse/expand 토글, 빈 섹션 표시
- **`ChannelContent`**: `productSlot` + `ConversationArea` 2-column 레이아웃 컴포넌트
- **`ProductView`**: 채널 핀 마크다운 렌더링 + 버전 히스토리 드롭다운
  - `useChannelPins(channelId)` 훅, Rust `create/list/delete_channel_pin` 명령어
- **`AgentThread`**: `agent_work_id` 기반 메시지 그루핑 컴포넌트
  - in-progress → 자동 확장, completed/failed/cancelled → 자동 접기
  - `MessageList.tsx`: `groupMessages()` 알고리즘으로 연속 동일 `agent_work_id` 묶음
- **`@mention` UI**: `@` 트리거 → `MentionPopup` 자동완성 (↑↓ 키보드 네비)
  - `metadata.mention = {agent, task: ''}` 메타데이터 주입
- **A2UI (Agent-to-UI) 시스템**: 에이전트가 UI 액션 버튼을 주입하는 프로토콜
  - `types/a2ui.ts`: `A2UISpec`, `A2UIAction`, `isA2UISpec()` 타입 가드
  - `A2UIRenderer.tsx`: primary/secondary/danger 스타일 액션 버튼 렌더링
- **`sync_session_channels`**: 로컬 Claude 세션을 Supabase session 채널로 자동 동기화 (Rust)
  - 이름 형식: `claude-{MMDD}-{session_uuid_8}`
  - Supabase REST upsert (`Prefer: resolution=merge-duplicates`)
  - `useSessionChannels()` 훅: `channel_type === 'session'` 필터링

#### MCP

- **`c4_task_list` `include_dod` 파라미터** 추가: 기본 `false` (brief listing), `true`일 때만 DoD 필드 포함

### 🐛 Bug Fixes

- **`c4_task_list`**: `include_dod` 기본값 `true` → `false`로 수정 (대용량 DoD로 응답 크기 급증 방지)

### 📚 Documentation

- AGENTS.md: Rust 테스트 수 85 → 92 (C1 재설계 +7 tests)
- AGENTS.md: LSP 언어별 지원 범위 명확화 — Go/Dart native 추가, `c4_find_symbol` 사용 원칙 섹션

---

## [0.12.0] - 2026-02-21

### ✨ Features

- **`skills_embed` 빌드 태그**: 배포 바이너리에 `.claude/skills/` 임베딩 지원
  - `make embed-skills` → `cmd/c4/skills_src/` 복사 (symlink deref) + SHA 버전 파일 생성
  - `//go:build skills_embed`: `embed.FS → fs.FS` 래퍼 + `!skills_embed` stub (기본 빌드 무영향)
  - 3단계 폴백: 소스 루트 심링크(개발) → 임베디드 추출(설치) → graceful skip
  - 버전 인식 추출: `~/.c4/skills/.version` SHA 비교, 동일 버전 재추출 생략
- **LLM Gateway API 키 보안 강화**: `config.yaml`에서 `api_key`/`api_key_env` 필드 제거
  - 키 해석 우선순위: `secrets.db (<provider>.api_key)` → 환경 변수 (`ANTHROPIC_API_KEY` 등)
  - 구버전 YAML 설정 감지 시 `slog.Warn` deprecation 경고 출력 (`config.Manager.IsSet()` 활용)
  - Ollama 예외 처리: 키 없어도 활성화 유지 (`name != "ollama"` 조건)
- **공개 배포 서브모듈** (`user/` → [PlayIdea-Lab/cq](https://github.com/PlayIdea-Lab/cq)):
  - `install.sh`: POSIX sh, `--tier solo|connected|full` (기본: solo), `--dry-run` 지원
  - `configs/solo.yaml`, `configs/connected.yaml`, `configs/full.yaml` 샘플 설정 포함
- **GitLab CI 크로스 컴파일 파이프라인** (`.gitlab-ci.yml`):
  - 3단계: `check-env` → 9개 병렬 빌드 → `release-github`
  - 9개 바이너리: 3 tier (solo/connected/full) × 3 platform (linux/darwin/windows-amd64)
  - `gh release create` → `PlayIdea-Lab/cq` GitHub Releases 자동 업로드

### 🔧 Build

- **Makefile**: `TIER_TAGS_*` 변수 + `build-cross` 타겟 추가
  - `make build-cross GOOS=linux GOARCH=amd64 TIER=solo` → `dist/cq-solo-linux-amd64`
  - `build-solo/connected/full/nightly` 타겟이 `embed-skills`에 의존 + `skills_embed` 태그 포함

### 📚 Documentation

- AGENTS.md: `user/` submodule, `.gitlab-ci.yml`, LLM key 변경 사항 반영

---

## [0.11.0] - 2026-02-21

### ✨ Features

- **`secrets`: AES-256-GCM 암호화 시크릿 스토어** (`~/.c4/secrets.db`)
  - 마스터 키 자동 생성 (`~/.c4/master.key`, 0400) / CI: `C4_MASTER_KEY=<64 hex>` env var 지원
  - CLI: `cq secret set/get/list/delete` — `set`은 stdin echo off (ioctl 기반)
  - MCP: `c4_secret_set`, `c4_secret_get`, `c4_secret_list`, `c4_secret_delete`
  - LLM Gateway 키 자동 해석: `config.yaml api_key` > `api_key_env` > `secrets.db (name.api_key)` 순

### 🛡️ Security (secrets store 강화 — 5라운드 polish)

- **O_EXCL 원자 키 생성** — `os.WriteFile` 대신 `O_EXCL|O_CREATE` + 실패 시 `os.Remove` 정리
- **TOCTOU 방지** — `os.Open` FD 후 동일 FD의 `Stat()` 호출로 파일 교체 경쟁 제거
- **EACCES vs ENOENT 구분** — 권한 오류 시 즉시 전파, ErrNotExist 시만 키 생성
- **마스터 키 zeroing** — `Close()`에서 인메모리 키 바이트 초기화
- **`io.LimitReader(f, 33)`** — 마스터 키 파일 읽기 크기 바운드
- **이중 shutdown 방지** — `sync.Once`로 signal + stdin EOF 동시 발생 시 race 제거
- `c4_secret_get`: plaintext 반환 경고 문구 handler description에 명시
- MCP 핸들러: key 길이 상한(256B) + value 크기 상한(64KB) 적용

### 🧪 Tests

- `secrets`: 10개 테스트 — `TestCorruptMasterKey`, `TestWrongKeyDecryptionFailure`, `TestMasterKeyEnvVarPersistence` 신규 추가

### 📚 Documentation

- Go 테스트 수 현행화: c4-core ~1,262 + c5 174 = ~1,436

---

## [0.10.0] - 2026-02-21

### ✨ Features

- **`hub`: Worker standby 자동 lease 갱신** — standby 루프에서 lease를 주기적으로 자동 갱신, 장시간 대기 시 lease 만료 방지
- **`llm`: `api_key` 직접 값 지원** — `LLMProviderConfig`에 환경변수 외 인라인 API 키 설정 가능

### 🐛 Bug Fixes

- **`task`**: Task ID 문법 regex + `ReviewID` last-hyphen split 수정 (CR-027)
- **`submit`**: `validation_results` 정책 통합 — optional, non-empty, status enum 검증 일관성 (CR-013/014)
- **`store`**: `files_changed` 컬럼 누락 수정 — `c4_tasks` 테이블에 추가 (CR-017)
- **`review`**: `max_revision` 경계 조건 수정 (`>` → `>=`) 및 정책 문서화
- **`security`**: `SubmitTask`에서 worker ownership 항상 검증 (CR-012)
- **`checkpoint`**: `APPROVE_FINAL`을 유효한 결정 값으로 추가
- **`handlers`**: `MarkBlocked` not-found 에러 계약 명확화 및 테스트 동기화 (CR-021)

### 🛡️ Security (CDP element-ref API 강화)

- `config.yaml` allow_patterns에 `( |$)` 끝 앵커 추가 — compound 명령 우회(`cmd&&evil`) 방지
- `find` 패턴을 두 개로 분리 — `./../../` 경로 순회 공격 차단
- `TypeByRef` 응답에서 `Value` 필드 제거 — 자격증명 에코 방지
- JS text sanitization에 C1 제어문자(U+0080–U+009F) 추가
- `TypeByRef` 라이브러리 레이어 empty-text 가드 — 브라우저 연결 전 조기 검증

### 🧪 Tests

- `test(worker)`: `handleWorkerComplete` status enum 검증 테스트 추가 (CR-006)
- `test(worktree)`: `SubmitTask` worktree 자동 cleanup 테스트 추가
- CDP ref-based API 검증 오류 테스트 — remote URL, invalid ref, empty text, credentials

### 📚 Documentation

- Go 테스트 수 업데이트: ~1,651개 (c4-core ~1,477 + c5 174), 26 패키지

---

## [0.9.2] - 2026-02-21

### ✨ Features

- **`c4_cdp_action` MCP 도구 추가** — Element-ref 기반 DOM 인터랙션
  - `scan_elements`: DOM 스캔 → `data-cdp-ref` 속성 부여 → `ElementRef` 배열 반환
  - `click` / `type` / `get_text`: ref ID로 요소 조작 (해상도 독립)
  - SPA에서 ref가 DOM 업데이트 후에도 유지되어 raw JS보다 안정적
  - Chrome 미연결 시 명확한 오류 메시지 + `CDP_DEBUG_URL` 환경변수 안내

- **`c4_run_validation` → `config.yaml` SSOT 연결** (T-SSOT-002)
  - `validation.lint` / `validation.unit` 설정이 `c4_run_validation`에 즉시 반영
  - `SetValidationConfig` 패턴: init-time 주입, 기존 파일 기반 자동 탐지를 fallback으로 유지
  - `strings.Fields` 파싱: `parts[0]` → Command, `parts[1:]` → Args (shell 미경유 → injection 안전)

### 🔧 Chores

- **`resolveHookModel` → `llm.ResolveAlias` 위임** (T-SSOT-001)
  - 별도 alias 테이블 제거, 모델 ID가 `llm/models.go` 단일 소스에서 관리됨
  - 이전 하드코딩: `sonnet-4-5`, `opus-4-5` → 현재: `sonnet-4-6`, `opus-4-6`

- **`.mcp.json` gitignore** (T-SSOT-004)
  - `infra/supabase/.mcp.json` → `**/.mcp.json` (전역 패턴)
  - `.mcp.json`은 개발자별 절대 경로를 포함하므로 버전 관리에서 제외

- **`config.yaml` 템플릿 개선** (T-SSOT-003)
  - `serve` 섹션에 v0.9.0 신규 컴포넌트 추가: `eventsink`, `sse_subscriber`, `agent`
  - `validation` 섹션에 따옴표 포함 인자 미지원 주의사항 주석 추가
  - `allow_patterns` 기본값 추가: git/go/uv/ls/grep/cq 등 안전한 개발 명령 즉시 허용

### 📚 Documentation

- **`AGENTS.md`**: `.mcp.json` 개발자별 파일 안내 추가 (`git rm --cached` 마이그레이션 가이드)
- **`mcp_init.go`**: `validCfg` 스냅샷 특성 주석 추가 (c4_config_set 변경은 재시작 후 반영)

## [0.9.1] - 2026-02-21

### 🐛 Bug Fixes

- **`ssesubscriber`**: X-API-Key 헤더 사용 (`Authorization: Bearer` → `X-API-Key`) — C5 auth 스펙 준수
- **`ssesubscriber`**: 백오프 지수 오버플로우 방지 (`exp > 30` 상한 설정)
- **`ssesubscriber`**: `bufio.Scanner` 토큰 버퍼 1MiB로 확장 (기본 64KiB → 대용량 SSE 페이로드 처리)
- **`ssesubscriber`**: `http.Client` 재사용 (reconnect loop마다 재생성 → 구조체 필드로 유지)
- **`eventsink` / `hubpoller`**: NoopPublisher → `eb.Publisher()` 연결 수정 (pub wiring)
- **`detect.go`**: `isServeRunningWith` 코드 중복 제거 → `isServeRunningWithCtx` 위임

### 📚 Documentation

- **AGENTS.md**: `cq serve` 컴포넌트 테이블에 `ssesubscriber` 항목 추가
  - 활성화 조건 (`serve.ssesubscriber.enabled: true`, `c5_hub && c3_eventbus` 빌드 태그) 명시

## [0.9.0] - 2026-02-21

### ✨ Features

- **`permission_reviewer` Config SSOT 완성**
  - `.c4/config.yaml` → `.c4/hook-config.json` → hook 스크립트 전체 파이프라인 연결
  - `mode` (`hook` / `model`), `auto_approve`, `allow_patterns`, `block_patterns` 필드 추가
  - `PermissionReviewerConfig` Go struct에 `Mode` 필드 추가 — 하드코딩 제거
  - `hookConfigFromC4Config()`: config에서 모든 필드 읽도록 전환

- **`cq init` 기본 `config.yaml` 자동 생성**
  - 신규 사용자가 별도 설정 없이 hook이 즉시 동작 (`enabled: true`, `mode: hook`)
  - 기존 파일 보존 (덮어쓰지 않음)
  - optional 섹션 (cloud, hub, llm_gateway) 주석 처리로 포함 — 필요 시 uncomment

- **C5 SSE 구독 컴포넌트 (`SSESubscriberComponent`)** (T-924-0)
  - `cq serve`에서 C5 Hub SSE 스트림 구독 지원
  - C5 → C4 이벤트 실시간 수신 경로 구현

- **Agent lazy-start MCP lifecycle 통합** (T-925-0)
  - `cq serve` Agent 컴포넌트를 MCP 서버 시작 시 자동 초기화
  - Supabase Realtime 구독 → `claude -p` 디스패치

- **`cq serve` 컴포넌트 안정화**
  - lazyPublisher 패턴: EventSink / HubPoller에 EventBus publisher 지연 초기화 (T-930-0)
  - EventBusComponent Health 5s TTL 캐시 추가 (T-934-0)
  - GPU handler `/daemon/` prefix로 serve mux에 마운트 (T-931-0)

### 🐛 Bug Fixes

- **`fix(hook)`**: `2>/dev/null` false positive 수정 (T-hook)
  - `>/dev/null`, `>/dev/stderr`, `>/dev/stdin`, `>/dev/stdout`, `>/dev/fd` 를 안전한 리다이렉션으로 허용
  - 실제 위험 패턴 (`>/dev/sda` 등 블록 디바이스)만 차단 유지
- **`fix(serve)`**: `Agent.Stop()` ctx 취소 미반영 수정 (T-933-0)
  - `wg.Wait()` → `select { case <-done: case <-ctx.Done(): }` 교체 — graceful shutdown 보장
- **`fix(serve)`**: 데드 코드 `status.go` 상수 제거 (T-932-0)

### 🔧 Chores

- `.mcp.json` command path `~/.local/bin/cq` (운영 바이너리)로 통일

### 📚 Documentation

- `AGENTS.md`: `permission_reviewer` 전체 스키마 + mode별 동작 + 설정 변경 절차 문서화

---

## [0.8.0] - 2026-02-21

### ✨ Features

- **멀티 세션 격리 개선 3종** (T-ISO-001~003)
  - **State Machine BEGIN IMMEDIATE**: `Transition()` 에 `db.Conn` + `BEGIN IMMEDIATE` 트랜잭션 적용
    - `ErrStateChanged` / `ErrInvalidTransition` / `ErrDatabase` 에러 3종 분류
    - 동시성 테스트 3개 추가 (`-race` 통과): `TestTransitionBeginImmediate`, `TestTransitionConcurrentStateChange`, `TestTransitionConcurrentRecovery`
  - **Advisory Phase Lock** (`c4_phase_lock_acquire` / `c4_phase_lock_release`, 신규 MCP 도구 2개)
    - `.c4/phase_locks/{phase}.lock` JSON 파일 기반, `polish` / `finish` 단계 보호
    - Stale 판정 5개 시나리오 (PID 생존/사망/EPERM + cross-host 2h 임계)
    - `/c4-polish` Phase 0 · `/c4-finish` Step 1에 lock check 통합
  - **Makefile atomic install**: `install-*` 타겟 `cp` → `cp .tmp && mv || rm .tmp` 원자적 교체

- **`cq serve` 컴포넌트 연동 완성**
  - EventBus / GPU Scheduler / EventSink / HubPoller config 로드 + 자동 등록
  - HubPoller: `NoopPublisher` fallback으로 nil publisher panic 제거

- **`c4-plan` Phase 4.5 Worker 기반 Plan Critique Loop**
  - 인라인 자가 비판 → 매 라운드 새 Worker(Task agent) 스폰 (confirmation bias 제거)
  - 수렴 조건: CRITICAL == 0 AND HIGH == 0 (최대 3라운드)

- **`c4-run` R-task 리뷰 Worker 자동 스폰**
  - 준비된 R- 태스크는 `review` 모델(opus)로 별도 Worker 할당

- **Hook Config SSOT 전환** (`.c4/hook-config.json`)
  - `permission_reviewer` 설정을 MCP 서버 시작 시 자동 내보내기
  - `hook-config.json` 우선, `~/.claude/hooks/*.conf` fallback
  - `AutoApprove` / `AllowPatterns` / `BlockPatterns` 필드 추가

### 🐛 Bug Fixes

- **`serve/realtime.go`**: 클린 disconnect 시 backoff가 계속 증가하는 버그 수정
  - backoff 증가를 error 분기 내로 이동 → 클린 재연결은 항상 1s로 리셋
- **`serve.go`**: `config.New()` 실패 시 `cfgMgr` nil 역참조 패닉 방지 (nil guard 추가)
- **`handlers/phase_lock.go`**: phase 파라미터 서버 측 allowlist 검증 누락 수정
- **`state/phase_lock.go`**: `lockFile()` path traversal 취약점 — `validPhase()` allowlist 강제
- **`fix(hook)`**: `mapfile` → `while-read` 교체 (bash 3.2 호환)
- **`fix(review)`**: R-task auto-cascade 제거 — 실제 리뷰 Worker가 처리하도록

### ♻️ Refactoring

- `cq init`: `.conf` embed/생성 제거, `hook-config.json` 패턴으로 통일

### 📚 Documentation

- MCP 도구 수: 154 → 156 (`c4_phase_lock_acquire/release` +2)
- Go 테스트 수: ~1,423 → ~1,430, 합계: ~2,205 → ~2,212

---

## [0.7.0] - 2026-02-21

### ✨ Features

- **`cq serve` 데몬 명령어**: ComponentManager 기반 상시 실행 서버
  - PID 락 파일, `/health` 엔드포인트, 컴포넌트 라이프사이클 관리
  - EventBus, EventSink, HubPoller, GPU Scheduler, Agent 컴포넌트 wrapping
  - Agent 컴포넌트: Supabase Realtime WebSocket → `@cq` mention 감지 → `claude -p` 디스패치
  - `cq serve` 실행 중이면 데몬 내장 컴포넌트 자동 skip (중복 방지)

- **Tiered Build System** (`solo` / `connected` / `full` / `nightly`)
  - Build tag 기반 컴포넌트 선택적 포함 (stub 파일 패턴)
  - `cq init --tier solo|connected|full` → `.c4/config.yaml` tier 저장
  - Makefile: `build-solo/connected/full/nightly`, `install-*` 타겟
  - C7 Observe / C6 Guard / C8 Gate 조건부 빌드 (nightly 태그)

- **C7 Observe**: Logger(slog) + Metrics + Registry Middleware 자동 계측
- **C6 Guard**: RBAC + Audit + Policy + Middleware (`c6_guard` 빌드 태그)
- **C8 Gate**: Webhook + Scheduler + Connector (`c8_gate` 빌드 태그)

- **Registry Middleware 체인**: `Registry.Use()` / `UseContextual()` 지원
  - `ToolNameFromContext()` 로 도구 이름 접근 가능

- **C5 Hub Artifact Pipeline**:
  - Signed Upload/Download (Supabase Storage), Worker input/output artifact 흐름
  - `LocalBackend` HTTP 핸들러 (`/v1/storage/upload/{path}`)
  - Long-poll Lease Acquire (20s server-side)

- **LLM Gateway 캐싱 개선**:
  - Anthropic Prompt Caching (ephemeral cache_control 블록)
  - `cache_hit_rate` + `cache_savings_rate` 노출 (`c4_llm_costs`)
  - `cache_by_default` 설정 지원

- **`cq doctor` 자가진단**: 8개 항목 (binary, .c4, .mcp.json, CLAUDE.md, hooks, Python, C5, Supabase)
- **c4-bash-security hook UX 개선**: 차단 메시지 가독성 향상

### 🐛 Bug Fixes

- `serve/agent.go`: `a.Status()` → `a.status` (미존재 메서드 수정)
- `serve/agent_test.go`: 중복 `mockComponent` 선언 제거, 잘못된 API 테스트 제거
- `c5`: LocalBackend HTTP 핸들러 디렉토리 리스팅 차단, partial file 정리
- LLM body limit `io.LimitReader(resp.Body, 1<<22)` 적용 (4 MiB)
- `guard/gate` SQLite + scheduler 마이너 수정

### 📚 Documentation

- `CLAUDE.md` / `AGENTS.md`: Tiered Build System, `cq serve` 섹션, C7/C6/C8 문서화
- `README.md`: CQ 브랜딩 통일, `cq doctor` 섹션, 154 tools / 2,205 tests / 141.7K LOC 수치 현행화

---

## [0.6.0] - 2026-01-26

### Added

- **DDD-CLEANCODE Worker Packet (Phase 6)**: 구조화된 태스크 명세
  - `Goal`: 완료 조건 + 범위 외 명시
  - `ContractSpec`: API 계약 (input/output/errors) + 테스트 요구사항 (success/failure/boundary)
  - `BoundaryMap`: DDD 레이어 제약 (target_domain, allowed/forbidden_imports)
  - `CodePlacement`: 파일 위치 명세 (create/modify/tests)
  - `QualityGate`: 검증 명령어 (name/command/required)
  - `CheckpointDefinition`: CP1/CP2/CP3 마일스톤 정의
  - `c4_add_todo` MCP 도구에 DDD-CLEANCODE 필드 지원 추가
  - `dod` 필드 deprecated 경고 (goal 미사용 시)
  - 12개 통합 테스트로 저장/로드 검증 완료

- **c4-plan/c4-submit 스킬 강화**
  - Worker Packet 구조화된 입력 UI (4.1~4.5절)
  - 경계 검증 자동 실행 (forbidden imports)
  - 작업 분해 검증 (max 2일, max 3 APIs)
  - ContractSpec 검증 (테스트 커버리지)

- **Semantic Search Engine (Phase 6.5)**: TF-IDF 기반 자연어 코드 검색
  - `SemanticSearcher`: 자연어 쿼리로 코드 검색
  - 프로그래밍 동의어 확장 (auth → authentication, db → database 등)
  - 범위 지정 검색 (symbols, docs, code, files)
  - 관련 심볼 찾기 및 타입별 검색

- **Call Graph Analyzer (Phase 6.5)**: 함수 호출 관계 분석
  - `CallGraphAnalyzer`: 호출자/피호출자 분석
  - 함수 간 호출 경로 찾기
  - 호출 그래프 통계 (핫스팟, 진입점, 고립 함수)
  - Mermaid 다이어그램 생성

- **Enhanced MCP Tools (Phase 6.5)**: 12개 새 코드 분석 도구
  - `c4_semantic_search`: 자연어 코드 검색
  - `c4_find_related_symbols`: 관련 심볼 찾기
  - `c4_search_by_type`: 타입별 심볼 검색
  - `c4_get_callers`: 호출자 찾기
  - `c4_get_callees`: 피호출자 찾기
  - `c4_find_call_paths`: 호출 경로 찾기
  - `c4_call_graph_stats`: 호출 그래프 통계
  - `c4_call_graph_diagram`: Mermaid 다이어그램
  - `c4_find_definition`: 심볼 정의 찾기
  - `c4_find_references`: 참조 찾기
  - `c4_analyze_file`: 파일 심볼 분석
  - `c4_get_dependencies`: 의존성 분석

- **Long-Running Worker Detection (Phase 6.5)**: Heartbeat 기반 이상 탐지
  - Worker heartbeat 모니터링
  - 장기 실행 태스크 자동 감지
  - Stale worker 복구 메커니즘

- **Team Collaboration (Phase 6)**: Supabase 기반 팀 협업 지원
  - `SupabaseStateStore`: 분산 프로젝트 상태 관리
  - `SupabaseLockStore`: 분산 잠금 (RLS 적용)
  - `TeamService`: 팀 생성/수정/삭제, 멤버 RBAC
  - `CloudSupervisor`: 팀 전체 리뷰 및 체크포인트 관리
  - `TaskDispatcher`: 우선순위 기반 태스크 분배
  - 6개 Supabase 마이그레이션 (`00001` ~ `00006`)

- **Branding Middleware**: 화이트라벨 커스텀 도메인 지원
  - `BrandingMiddleware`: Host 헤더 기반 브랜딩 적용
  - `BrandingCache`: TTL 캐시 (기본 60초)
  - 팀별 로고, 색상, 도메인 설정

- **Code Analysis Engine**: Python/TypeScript 코드 분석
  - `PythonParser`: Python AST 분석
  - `TypeScriptParser`: TypeScript 구문 분석
  - 심볼 테이블, 의존성 그래프, 호출 관계

- **Documentation Server (MCP)**: 문서화 자동화
  - `query_docs`: 문서 검색/쿼리
  - `create_snapshot`: 코드베이스 스냅샷
  - `get_usage_examples`: 사용 예시 추출
  - Context7 스타일 REST API (`/api/docs`)

- **Gap Analyzer (MCP)**: 명세-구현 매핑
  - `analyze_spec_gaps`: EARS 요구사항 갭 분석
  - `generate_tests_from_spec`: 명세→테스트 생성
  - `link_impl_to_spec`: 구현-명세 연결
  - `verify_spec_completion`: 완료 검증

- **GitHub Integration 강화**
  - `GitHubClient`: 팀 권한 동기화
  - `GitHubAutomation`: 자동 PR/Issue 생성
  - 웹훅 이벤트 처리

- **Review-as-Task**: 리뷰가 태스크로 관리됩니다
  - 태스크 ID에 버전 번호 추가 (T-XXX → T-XXX-0)
  - 구현 태스크 완료 시 자동으로 리뷰 태스크(R-XXX-N) 생성
  - REQUEST_CHANGES 시 다음 버전 태스크 자동 생성 (T-XXX-1)
  - `max_revision` 설정으로 최대 수정 횟수 제한 (기본값: 3)
  - **리뷰 태스크 자동 라우팅**: `task_type="review"` 설정으로 `code-reviewer` 에이전트 자동 할당

- **Checkpoint-as-Task**: 체크포인트가 태스크로 처리됩니다
  - Phase의 모든 리뷰가 APPROVE되면 CP-XXX 태스크 자동 생성
  - Worker가 체크포인트 검증 수행 (E2E, HTTP 등)
  - APPROVE 시 Phase 완료 및 main 머지
  - REQUEST_CHANGES 시 문제 태스크의 다음 버전 생성
  - `checkpoint_as_task: true` 설정으로 활성화

- **TaskType Enum**: 태스크 유형 구분
  - `IMPLEMENTATION`: 구현 태스크 (T-XXX-N)
  - `REVIEW`: 리뷰 태스크 (R-XXX-N)
  - `CHECKPOINT`: 체크포인트 태스크 (CP-XXX)

- **Task Model 확장**
  - `base_id`: 기본 태스크 ID ("001")
  - `version`: 버전 번호 (0, 1, 2...)
  - `type`: TaskType enum
  - `task_type`: 스킬 매칭용 태스크 유형 ("review", "debug", "security" 등)
  - `phase_id`: Phase 식별자
  - `required_tasks`: CP가 검증할 태스크 목록
  - `review_decision`: 리뷰 결정 (APPROVE/REQUEST_CHANGES)

- **GraphRouter.use_legacy_fallback** 속성 추가
  - skill matcher와 rule engine 미설정 시 도메인 기반 라우팅만 사용

- **GitLab Integration**: GitLab MR 웹훅 및 AI 코드 리뷰
  - `GitLabClient`: REST API 클라이언트 (diff 조회, 노트/토론 생성, 라벨)
  - `GitLabProvider`: 통합 프로바이더 (OAuth, 웹훅 검증)
  - `MRReviewService`: AI 기반 코드 리뷰 서비스 (LiteLLM/Anthropic)
  - `/webhooks/gitlab` 엔드포인트 추가
  - X-Gitlab-Token 헤더 기반 웹훅 검증
  - 환경 변수: `GITLAB_PRIVATE_TOKEN`, `GITLAB_WEBHOOK_SECRET`, `GITLAB_URL`

### Changed

- MCP 도구 25개 이상으로 확장
- `c4_add_todo`가 정규화된 태스크 ID 반환 (T-XXX → T-XXX-0)
- SupervisorLoop이 `checkpoint_as_task` 모드에서 큐 항목만 정리 (직접 처리 안함)
- 체크포인트 검증이 Design 단계 요구사항 기반으로 DoD 자동 생성

### Fixed

- `test_add_todo` 관련 10개 테스트 수정 (정규화된 ID 사용)
- GraphRouter 누락 속성 `use_legacy_fallback` 추가

## [0.1.0] - 2026-01-15

### Added

- 초기 릴리스
- State Machine 워크플로우 (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- MCP Server 통합 (19개 도구)
- Multi-Worker SQLite WAL 기반 병렬 실행
- Agent Routing (도메인별 에이전트 자동 선택)
- EARS 요구사항 수집 (5가지 패턴)
- Multi-LLM Provider 지원 (LiteLLM 기반 100+)
- Checkpoint Gates (단계별 리뷰 포인트)
- Auto-Validation (자동 lint/test 실행)
- 다중 플랫폼 지원 (Claude Code, Cursor, Codex CLI, Gemini CLI, OpenCode)

## v0.93.1 — 2026-03-10

### Fixed
- **c5 worker**: Fix Docker socket access check — `os.OpenFile` fails on Unix sockets (ENXIO) even with correct permissions; replaced with `net.DialTimeout`
- **c5 hub**: `MarkStaleWorkers` now includes `busy` workers (zombie GC for stuck-busy workers)
- **c5 hub**: `.dockerignore` allows `docs/*.md` and `llms.txt` for go:embed (Fly.dev deploy fix)

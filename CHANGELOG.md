# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

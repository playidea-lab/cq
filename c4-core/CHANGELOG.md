# Changelog

## [v0.80.1] - 2026-03-08

### 🐛 Bug Fixes
- **heartbeat**: `WorkerHeartbeat` updated_at RFC3339 통일 (`datetime('now')` → Go-side `time.Now().UTC()`)
- **heartbeat**: `errors.New` 사용으로 archtest ratchet 준수 (`fmt.Errorf` without `%w` 제거)
- **heartbeat**: `HeartbeatIntervalSec` omitempty 제거 — 계약 필드 항상 JSON 직렬화

### 🧪 Tests
- **heartbeat**: `TestWorkerHeartbeat` T-002-0 시드 추가로 nil-모호성 해소 (stale 보호 vs 태스크 없음 구분)

---

## [v0.80.0] - 2026-03-08

### ✨ Features
- **research/Level4**: CQ Research Loop Level 4 — 완전 자율 연구 루프
  - `c4_research_loop_start`: 가설 ID로 자율 루프 시작, budget gate (cost/iteration 상한) 내장
  - `c4_research_loop_stop`: 루프 중단 + TypeDebate(`trigger_reason: loop_stopped`) 기록
  - `c4_research_intervene`: steering(방향 주입) / injection(새 가설 병렬 분기) / abort(즉시 취소) 3종 사람 개입
  - `LoopOrchestrator` serve.Component: Hub 잡 완료 감지 → Debate 자동 트리거 → verdict 분기 → 다음 가설 자동 등록 → Hub 잡 재제출
  - `LineageBuilder`: 가설 lineage 조회 (TypeDebate 히스토리, round 정렬, 최근 5회), Debate context 자동 주입
  - `runDebate` lineage_context 파라미터 추가 — Optimizer/Skeptic 프롬프트에 lineage 자동 주입
  - null_result N회 연속(기본 2회) → 강제 explore 플래그 자동 활성화
- **research/watch**: `cq research watch` CLI — 메트릭 레이어(val_loss/test_metric + ▼▲ 트렌드) + 컨텍스트 레이어(verdict 히스토리, null_result streak) 동시 표시, 개입 타이밍 시그널
- **hub**: `HubClient` 인터페이스 + `MockHubClient` 정의 (c4-core/internal/hub/client.go)
- **heartbeat**: `c4_worker_heartbeat` MCP 도구 — staleTimeoutMin=3분 explicit heartbeat

### 🐛 Bug Fixes
- **research**: `serve_loop_orchestrator_jobdone.go` runDebate 7번째 인자(lineageContext) 누락 수정

### 🔧 Chores
- **archtest**: researchhandler allowedDeps에 internal/knowledge 추가, fmt.Errorf 카운트 업데이트
- **handlers_test**: RegisterAll 도구 카운트 12→13 (`c4_worker_heartbeat` 반영)
- **docs**: Go 테스트 수 업데이트 — c4-core ~2,472 (research 태그 포함)

---

## [v0.79.0] - 2026-03-08

### ✨ Features
- **c5/llmclient**: Anthropic Claude provider 추가 — `chatProvider` 인터페이스 + `anthropicProvider` 구현
  - `NewAnthropic()`: `x-api-key` 인증, `/v1/messages` 엔드포인트, `anthropic-version: 2023-06-01` 헤더
  - 기존 OpenAI-compat `New()` 완전 하위 호환
- **c5/config**: `llm.provider` 필드 추가 (`openai`|`anthropic`), anthropic은 `base_url` 불필요
  - API key 우선순위: `C5_ANTHROPIC_API_KEY` > `C5_LLM_API_KEY` > config.yaml
- **c5/dooray**: `/cq` Dooray 봇을 Claude 기반 대화형 어시스턴트로 전환
  - 하드코딩된 프로젝트 경로(hmr_postproc) 제거, 범용 팀 어시스턴트 역할
  - 대화 히스토리 활용 명시, 액션 라우팅 조건 명확화

설정 방법:
```yaml
# C5 config.yaml
llm:
  provider: anthropic
  model: claude-haiku-4-5-20251001
# API key: C5_ANTHROPIC_API_KEY 환경변수 또는 config.yaml api_key
```

---

## [v0.77.1] - 2026-03-08

### 🐛 Bug Fixes
- **hub_poller**: `cq serve`에서 Hub Poller가 secrets.db/cloud JWT 대신 config.yaml의 API key만 사용하여 발생하는 401 오류 수정 — `initHub()`에서 이미 구성된 `hub.Client`를 HubPollerComponent에 직접 주입

### 🔧 Chores
- **serve/hub**: hubpoller stub 파일 시그니처를 `any` 파라미터로 동기화

---

## [v0.77.0] - 2026-03-08

### ✨ Features
- **notify**: `internal/notify` 패키지 — NotificationProfile + per-channel Sender (Dooray/Discord/Slack/Teams)
- **notifyhandler**: `c4_notification_set` / `c4_notification_get` / `c4_notify` MCP 도구
- **skills**: c4-plan/c4-run/c4-finish 마일스톤에 `c4_notify` 자동 호출

### 🐛 Bug Fixes
- **hypothesis**: status 필드 통일 — poll()과 MCP 핸들러 생성 경로 간 `status`/`hypothesis_status` 불일치 수정
- **hypothesis**: cleanup()에서 body 덮어쓰기 버그 수정 (Update body=nil)
- **hypothesis**: 마크다운 펜스 제거 패턴 수정 (```JSON 대소문자 처리)
- **notifyhandler**: internal/notify 위임, 파일 권한 octal 수정

### 🧪 Tests
- **hypothesis**: CrossComponent 테스트 추가 (status 필드 통일 검증)
- **hypothesis**: CleanupPreservesBody 테스트 추가 (body 보존 검증)

---

## [v0.43.0] - 2026-03-01

### ✨ Features
- **gemini**: Gemini 3.0 에이전트 업그레이드 + C1 "Hands" 브리지 (WebSocket 기반 네이티브 셸 실행)
- **enforce**: 3-layer deprecated 스킬 강제 시스템
  - Layer 1: c4-gate.sh Hook Gate (c4-polish/c4-refine → c4-finish 차단)
  - Layer 2: Arch Test (TestDeprecatedSkillsAreStubs / TestFinishSkillsHaveKnowledgeGate / TestPlanSkillsHaveKnowledgeRead)
  - Layer 3: Skill Linter (`scripts/lint-skills.sh`, `make lint-skills`)
- **c3/eventbus**: Dooray 양방향 응답 — `c4_dooray_respond` MCP 도구 + `dooray_respond_llm` action type + LLM caller
- **skills**: plan-run-finish 3단계 워크플로우 + C9 지식 게이트 패턴 (finish=기록, plan=조회)

### 🐛 Bug Fixes
- **reboot**: `.reboot` 파일에 UUID 기록하여 올바른 세션 복구 보장
- **archtest**: C3 EventBus dooray_respond 추가에 따른 ratchet allowlist 업데이트

### ♻️ Refactoring
- **skills**: c4-polish(336→17줄), c4-refine(339→17줄) deprecated stub으로 축약

### 🔧 Chores
- **dooray**: WebhookGateway Stop() mutex 해제 후 Shutdown 패턴 + HTTPS 강제 + 보안 모델 문서화
- **bats**: c4gate hooktest 케이스 14→19개 (deprecated 차단 4 + c4-finish 긍정 케이스 1 추가)

---

## v2.0.0-beta (2026-02-08)

### Highlights

C4 v2.0.0-beta introduces the **Go Hybrid Architecture**: a high-performance
Go core (`c4-core`) that handles MCP protocol, configuration, and validation,
while Python continues to manage task orchestration and agent routing.

### New Features

- **Go MCP Server**: All 10 core MCP tool handlers implemented in Go
  - State: `c4_status`, `c4_start`, `c4_clear`
  - Tasks: `c4_get_task`, `c4_submit`, `c4_add_todo`, `c4_mark_blocked`
  - Tracking: `c4_claim`, `c4_report`, `c4_checkpoint`
- **Go Config Manager**: YAML config loading via viper with env var overrides
- **Go Validation Runner**: os/exec-based command runner with timeout support
- **gRPC Bridge**: Protobuf-defined Go <-> Python communication channel
- **Performance Benchmarks**: testing.B benchmark suite for Go/No-Go decision

### Performance

| Metric | Python | Go Core | Improvement |
|--------|--------|---------|-------------|
| MCP server start | 500-1000ms | 0.005ms | ~100,000x |
| c4_status response | 5-20ms | 57ns | ~100,000x |
| Worker creation | 0.1-1ms | 61ns | ~1,600x |
| Memory at rest | 50-100MB | 14MB | ~3-7x |
| Binary size | ~50MB (venv) | <20MB | ~2.5x |

### Breaking Changes

- **MCP Server**: Go binary replaces Python MCP server as the primary endpoint
- **Config location**: Configuration remains at `.c4/config.yaml` (no change)
- **gRPC requirement**: Python daemon now communicates via gRPC bridge instead
  of in-process calls

### Migration Guide

1. Install Go binary:
   ```bash
   # macOS (Apple Silicon)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-darwin-arm64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core

   # macOS (Intel)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-darwin-amd64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core

   # Linux (amd64)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-linux-amd64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core
   ```

2. Update Claude Code MCP settings (`~/.claude/mcp.json`):
   ```json
   {
     "mcpServers": {
       "cq": {
         "command": "c4-core",
         "args": ["mcp", "--project", "/path/to/project"]
       }
     }
   }
   ```

3. Python daemon runs alongside (started automatically by c4-core):
   ```bash
   # Verify installation
   c4-core version    # Should show v2.0.0-beta
   c4-core status     # Should respond in <100ms
   ```

### Build from Source

```bash
cd c4-core
go build -o c4-core ./cmd/c4

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o c4-core-darwin-arm64 ./cmd/c4
GOOS=darwin GOARCH=amd64 go build -o c4-core-darwin-amd64 ./cmd/c4
GOOS=linux GOARCH=amd64 go build -o c4-core-linux-amd64 ./cmd/c4
```

### Test Results

- Go tests: 55+ tests across 4 packages (config, handlers, validation, benchmark)
- Python tests: 358+ tests passing (no regression)
- Frontend tests: 29 vitest tests passing
- Rust tests: 16 cargo tests passing

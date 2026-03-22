<!-- 이 파일은 AGENTS.md에서 분리된 아키텍처 레퍼런스입니다 -->

# CQ Architecture Reference

> 이 문서는 AGENTS.md에서 분리된 아키텍처 레퍼런스입니다.
> 에이전트 행동 규칙은 AGENTS.md를 참조하세요.

---

## Go Core (c4-core/) — Primary MCP Server

> Go 기반 MCP 서버. ~45.0K LOC(src) + ~38.7K LOC(test). ~1,950개 테스트, 37 패키지.

### 아키텍처
```
Claude Code → Go MCP Server (stdio, 118 base + 26 Hub = 144 tools)
                ├→ Go native (28): 상태/설정, 태스크, 파일, git, validation, config, health, eventbus rules
                ├→ Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                ├→ Soul/Persona/Twin (10): soul_evolve, soul_check, soul_sync, persona_learn, persona_analyze, persona_diff, whoami, reflect
                ├→ LLM Gateway (3): llm_call, llm_providers, llm_costs
                ├→ CDP Runner + WebMCP (5): cdp_run, cdp_list, webmcp_discover, webmcp_call, webmcp_context
                ├→ WebContent (1): web_fetch (content negotiation, SSRF, HTML→MD) — c2/webcontent
                ├→ C1 Messenger (5): search, mentions, briefing, send_message, update_presence + ContextKeeper
                ├→ Drive (6): upload, download, list, delete, info, mkdir
                ├→ Go Native — Tier 1 (18): Research (5) + C2 (7) + GPU (6) + Soul Evolution (1)
                ├→ Go Native — Tier 2 (13): Knowledge (Store+FTS5+Vector+Embedding+Usage+Ingest+Sync+Publish)
                ├→ C7 Observe (4, c7_observe 조건부): observe_metrics, observe_logs, observe_config, observe_health
                ├→ C6 Guard (5, c6_guard 조건부): guard_check, guard_audit, guard_policy_set/list, guard_role_assign
                ├→ C8 Gate (6, c8_gate 조건부): gate_webhook_register/list/test, gate_schedule_add/list, gate_connector_status
                ├→ Hub Client (26, 조건부): job, worker, DAG, edge, deploy, artifact
                ├→ Worker Standby (3, Hub 조건부): standby, complete, shutdown
                ├→ EventSink (1): HTTP POST /v1/events/publish 수신 → C3 EventBus 전달
                ├→ HubPoller (1): 30s 간격 C5 RUNNING jobs 상태 감시 → hub.job.completed/failed 발행
                └→ JSON-RPC proxy (10) → Python Sidecar (LSP 7 + C2 Doc 2 + Onboard 1)
```

### 패키지 구조
```
c4-core/
├── cmd/c4/           # CLI (cobra) + MCP server 진입점
├── internal/
│   ├── mcp/          # Registry + stdio transport
│   │   └── handlers/ # 도구별 핸들러 (sqlite_store, files, git, proxy, ...)
│   ├── bridge/       # Python sidecar 관리 (JSON-RPC/TCP, lazy start)
│   ├── task/         # TaskStore (SQLite, Memory, Supabase)
│   ├── state/        # State machine (INIT→...→COMPLETE)
│   ├── worker/       # Worker manager
│   ├── validation/   # Validation runner (go test, pytest, cargo test 자동 감지)
│   ├── config/       # Config manager (YAML, env, economic presets)
│   ├── cloud/        # Auth (OAuth), CloudStore, HybridStore, TokenProvider (auto-refresh)
│   ├── hub/          # PiQ Hub REST+WS client (26 tools)
│   ├── daemon/       # 로컬 작업 스케줄러 (Store+Scheduler+Server+GPU)
│   ├── eventbus/     # C3 EventBus v4 (gRPC, WS bridge, DLQ, filter v2)
│   ├── knowledge/    # C9 Knowledge (Store+FTS5+Vector+Embedding+Usage+Chunker+Ingest+Sync)
│   ├── research/     # Research iteration store (paper+experiment loop)
│   ├── c2/           # C2 Workspace/Profile/Persona + webcontent (fetch, HTML→MD, llms.txt)
│   ├── drive/        # C0 Drive client (Supabase Storage)
│   ├── llm/          # LLM Gateway (Anthropic, OpenAI, Gemini, Ollama)
│   ├── cdp/          # Chrome DevTools Protocol runner + WebMCP + CDP auto-discovery
│   ├── observe/      # C7 Observe: Logger(slog) + Metrics + Middleware (c7_observe build tag)
│   ├── guard/        # C6 Guard: RBAC + Audit + Policy + Middleware (c6_guard build tag)
│   ├── gate/         # C8 Gate: Webhook + Scheduler + Connectors (c8_gate build tag)
│   └── worker/       # Worker shutdown signal store (SQLite)
└── test/benchmark/   # 벤치마크
```

### 빌드/테스트/설치

```bash
# 빌드 + 테스트
cd c4-core && go build ./... && go test ./...

# 사용자 설치 (CRITICAL — .mcp.json이 이 경로를 참조)
cd c4-core && go build -o ~/.local/bin/cq ./cmd/c4/

# 개발용 바이너리 (CI/로컬 테스트)
cd c4-core && go build -o bin/cq ./cmd/c4/

# 환경 진단
cq doctor              # 8개 항목 건강 체크
cq doctor --json       # CI/자동화용 JSON 출력
```

### 바이너리 관리 규칙 (CRITICAL)

| 경로 | 용도 | 갱신 시점 |
|------|------|----------|
| `~/.local/bin/cq` | **운영 바이너리** — `.mcp.json`이 참조, Claude Code가 실행 | 코드 변경 후 반드시 재빌드 |
| `c4-core/bin/cq` | 개발/테스트용 | `go build ./...` 시 자동 |

**필수 규칙**:
1. **코드 수정 후 `~/.local/bin/cq` 재빌드 필수** — 안 하면 구 바이너리가 계속 실행됨
2. **`cp` 복사 금지** — macOS ARM64에서 코드 서명 무효화. 반드시 `go build -o` 사용
3. **재빌드 후 세션 재시작** — Claude Code가 세션 시작 시 MCP 서버를 로드하므로
4. **`c4-finish` 스킬에서 자동 설치** — 릴리스 루틴에 `go build -o ~/.local/bin/cq` 포함 권장

### cq init 자동 설치 항목 (`cq claude/codex/cursor` 실행 시)

| 항목 | 대상 경로 | 확인 | 설명 |
|------|----------|------|------|
| `.c4/` 디렉토리 | `{project}/.c4/` | 자동 | C4 데이터 디렉토리 |
| `.mcp.json` | `{project}/.mcp.json` | 자동 | MCP 서버 설정 |
| `CLAUDE.md` | `{project}/CLAUDE.md` | 자동 | C4 override 규칙 |
| skills symlinks | `{project}/.claude/skills/` | 자동 | C4 스킬 심볼릭 링크 |
| **hook 파일** | `{project}/.claude/hooks/c4-gate.sh` | **대화형** | PreToolUse 룰 기반 게이트 |
| **hook 파일** | `{project}/.claude/hooks/c4-permission-reviewer.sh` | **대화형** | PermissionRequest Haiku 심사 |
| **settings.json 생성** | `{project}/.claude/settings.json` | **대화형** | `$CLAUDE_PROJECT_DIR` 경로로 훅 등록 |

- `.mcp.json`은 **per-developer 파일** — 절대경로(`/Users/...`)가 포함되므로 git에 커밋하지 않음. clone 후 `cq init` 실행 시 자동 생성됨. 기존에 추적 중인 경우: `git rm --cached .mcp.json`
- hook/settings 설치는 **대화형 확인** 필요 — 사용자가 N 입력 시 건너뜀 (C4 핵심 기능에 영향 없음)
- `--yes` / `-y` 플래그: 모든 대화형 확인을 자동 승인 (CI/자동화 환경용)
- hook 파일은 바이너리에 embed되어 있어 소스 없이도 설치 가능
- **전역 `~/.claude/` 수정 없음** — 훅은 프로젝트 단위(`.claude/hooks/`)에 설치되며 `$CLAUDE_PROJECT_DIR`로 경로를 해소함
- **hook 설정 SSOT**: `.c4/config.yaml`의 `permission_reviewer` 섹션
- hook 설정 변경 시 MCP 서버 재시작 필요 (`.c4/hook-config.json`이 재생성됨)

#### permission_reviewer 전체 스키마

```yaml
# .c4/config.yaml
permission_reviewer:
  enabled: true          # false → hook 즉시 통과 (비활성화)
  mode: hook             # "hook": 정규식 패턴만 / "model": LLM API 호출
  model: haiku           # model mode용: haiku, sonnet, opus (또는 full model ID)
  api_key_env: ANTHROPIC_API_KEY
  fail_mode: ask         # model mode 실패 시: "ask" (사용자 확인) / "allow" (자동 승인)
  auto_approve: true     # true: 안전 판정 시 사용자 확인 없이 자동 실행
  timeout: 10            # model mode API 타임아웃 (초)
  allow_patterns: []     # 항상 허용할 정규식 패턴 (모든 mode에서 최우선)
  block_patterns: []     # 항상 차단할 정규식 패턴 (hook mode + model fallback)
```

**흐름**: `.c4/config.yaml` → (MCP 서버 시작 시) → `.c4/hook-config.json` → hook 스크립트

**hook 실행 우선순위 (4단계)**:
1. `allow_patterns` 매칭 → 즉시 allow (API 호출 없음)
2. `mode: model` → Haiku API 판단 (allow_patterns 미매칭 명령만)
3. API 실패 시 → `block_patterns` + 내장 위험 패턴으로 폴백
4. `hook-config.json` 자체가 없을 때 → 내장 safe 패턴(hook mode)으로 폴백

**`.c4/` 탐색**: hook은 `$PWD`에서 루트 방향으로 올라가며 `.c4/`를 탐색.
서브디렉토리에서 Claude Code를 열거나 monorepo 구조에서도 올바른 프로젝트 config를 자동 인식.

| mode | 동작 | 권장 상황 |
|------|------|----------|
| `model` | allow_patterns 선필터 → Haiku API (정확) | 보안 민감 프로젝트 [권장] |
| `hook` | 정규식 패턴 매칭만 (빠름, 오프라인) | 오프라인 환경 |

### cq doctor (자가진단)

프로젝트 환경의 건강 상태를 8개 항목으로 진단합니다.

```bash
cq doctor              # 전체 진단
cq doctor --json       # JSON 출력 (CI/자동화용)
cq doctor --fix        # 자동 수정 가능한 문제 해결 시도
```

| 체크 항목 | 검사 내용 |
|----------|----------|
| cq binary | 바이너리 존재 여부 + 버전 |
| .c4 directory | `.c4/` 존재 + DB 파일 (tasks.db 또는 c4.db) |
| .mcp.json | JSON 유효성 + 참조된 바이너리 경로 존재 |
| CLAUDE.md | 파일 존재 + symlink 유효성 |
| hooks | hook 파일 존재 + 버전(SHA256) 체크 + settings.json 등록 |
| Python sidecar | `uv` 존재 + pyproject.toml |
| C5 Hub | hub 설정 + health 엔드포인트 |
| Supabase | 클라우드 설정 + 연결 확인 |

- non-CQ 디렉토리에서도 실행 가능 (누락 항목을 FAIL로 표시)
- `--fix`: broken symlink 제거, **outdated hook 자동 갱신** 등 안전한 자동 수정 (수정 후 WARN으로 표시)
- `--json`: 구조화된 JSON 배열 출력 (name, status, message, fix 필드)

### cq serve (통합 데몬)

`cq daemon`의 후계자. GPU/CPU 작업 스케줄러를 포함한 여러 서비스 컴포넌트를 단일 프로세스로 실행합니다.

```bash
cq serve               # 기본 포트 :4140 에서 시작
cq serve --port 4141   # 포트 지정
```

| 컴포넌트 | 활성화 조건 | 설명 |
|----------|------------|------|
| `GET /health` | 항상 | 전체 컴포넌트 상태 JSON |
| `eventbus` | `serve.eventbus.enabled: true` | C3 gRPC 이벤트 버스 |
| `eventsink` | `serve.eventsink.enabled: true` + `c3_eventbus` 빌드 태그 | C5→C4 HTTP 이벤트 수신 (:4141) |
| `gpu` | `serve.gpu.enabled: true` | GPU/CPU 작업 스케줄러 (daemon 패키지 래핑) |
| `agent` | `serve.agent.enabled: true` + `cloud.url` + `cloud.anon_key` 설정 | Supabase Realtime @cq mention → claude -p 디스패치; claim 직후 `c1_members.status="typing"` 비동기 알림, 완료 시 `"online"` 복원; `claude -p --dir <projectDir>` |
| `ssesubscriber` | `serve.ssesubscriber.enabled: true` + `hub && c3_eventbus` 빌드 태그 | Hub SSE 스트림 구독 → EventBus 전달 |
| `stale_checker` | `serve.stale_checker.enabled: true` | 주기적 stale 태스크(in_progress stuck) 감지 → pending 리셋 + `task.stale` 이벤트 발행 |
| `hub` | `serve.hub.enabled: true` | Supabase 기반 Hub 작업 큐 연동 (cloud.url + cloud.anon_key 필요) |

**컴포넌트 활성화** (`.c4/config.yaml`):
```yaml
serve:
  eventbus:
    enabled: true
  gpu:
    enabled: true
  eventsink:
    enabled: true   # c3_eventbus 빌드 태그 필요
  agent:
    enabled: true   # cloud.url + cloud.anon_key 필요
  ssesubscriber:
    enabled: true   # hub && c3_eventbus 빌드 태그 필요; hub.enabled: true 필요
  stale_checker:
    enabled: true
    threshold_minutes: 30   # 이 시간 이상 in_progress이면 stale 판정
    interval_seconds: 60    # 체크 주기
  hub:
    enabled: false          # true 시 Supabase 기반 Hub 연동 활성화 (cloud.url + cloud.anon_key 필요)
```

**PID 파일**: `~/.c4/serve/serve.pid` (포트 `:4140`)

**마이그레이션 가이드** (`cq daemon` → `cq serve`):

| 기존 | 대체 |
|------|------|
| `cq daemon` | `cq serve` |
| `cq daemon --port 7123` | `cq serve --port 7123` |
| `cq daemon stop` | `cq serve stop` (예정) 또는 `POST /serve/stop` |
| `cq daemon --data-dir` | `cq serve --data-dir` |

- `cq daemon`은 하위 호환을 위해 유지되지만, `cq serve`가 실행 중이면 시작 시 경고를 출력합니다.
- **감지 기준**: `~/.c4/serve/serve.pid` 존재 + 프로세스 생존 + `localhost:4140/health` 응답 200

**OS 서비스 자동 시작** (macOS LaunchAgent / Linux systemd / Windows Service):

```bash
cq serve install    # OS 서비스 등록 (부팅 시 자동 시작)
cq serve uninstall  # OS 서비스 제거
cq serve status     # OS 서비스 상태 + 수동 실행 PID 확인
```

- `kardianos/service` 기반 크로스플랫폼 지원
- `cq doctor`의 `os-service` 항목에서 설치/실행 상태 확인 가능

### 주요 설정 섹션 (.c4/config.yaml)

| 섹션 | 설명 |
|------|------|
| `hub` | C5 Hub 연결 (enabled, url, api_key) |
| `llm_gateway` | LLM 프로바이더 설정 — API 키는 `cq secret set <provider>.api_key <value>` (config.yaml의 api_key/api_key_env 필드는 deprecated) |
| `eventsink` | EventSink HTTP 서버 설정 (enabled, port, token) |
| `worktree` | Worktree 관리 (auto_cleanup: true/false) |
| `observe` | C7 관측성 (enabled, log_level, log_format) — c7_observe 빌드 태그 필요 |
| `guard` | C6 접근 제어 (default_action: allow/deny, policies[]) — c6_guard 빌드 태그 필요 |
| `gate` | C8 외부 연동 (connectors.slack.*, connectors.github.*) — c8_gate 빌드 태그 필요 |

- **`eventsink`**: C5 → C4 이벤트 수신용 HTTP 엔드포인트 (기본 포트 `:4141`). `POST /v1/events/publish`로 수신한 이벤트를 C3 EventBus에 전달.
- **`worktree.auto_cleanup`**: `true`(기본값)이면 `SubmitTask()` 성공 시 worktree를 즉시 자동 제거. Worktree 자동 생성은 AssignTask에서, 자동 제거는 SubmitTask 성공 시 수행.

---

## Python Sidecar (c4/)

> Python 기반 보조 서버. Go MCP 서버에서 JSON-RPC/TCP로 호출. ~22.9K LOC.

### 역할 (Tier 1+2 마이그레이션 후 축소)
```
Go MCP Server ──JSON-RPC/TCP──→ Python Sidecar (10 tools)
                                  ├→ LSP (7): find_symbol, get_overview, replace_body,
                                  │          insert_before/after, rename, find_refs
                                  │          ※ Python/JS/TS only (Jedi+multilspy)
                                  │          ※ Go/Rust → c4_search_for_pattern 대체
                                  ├→ C2 Doc (2): parse_document, extract_text
                                  └→ Onboard (1): c4_onboard
```

### 마이그레이션 이력
| Tier | 도구 수 | 대상 | Go 패키지 |
|------|---------|------|-----------|
| Tier 1 | 17 → Go | Research (5) + C2 (6) + GPU (6) | `research/`, `c2/`, `daemon/` |
| Tier 2 | 12 → Go | Knowledge (12) | `knowledge/` |
| 남은 Proxy | 10 | LSP (7) + C2 Doc (2) + Onboard (1) | — |

### 특성
- **Lazy Start**: 첫 proxy 호출 시에만 sidecar 시작
- **Health Check**: Exponential backoff로 연결 확인
- **Python 미설치 시**: Graceful fallback (LSP/Doc 도구만 비활성)

---

## C1 Channel Adapter (c1/)

> 채널 기반 메시징 브릿지. PlatformAdapter 인터페이스로 메신저 → Claude Code 연결.
> Tauri 데스크톱 앱에서 전환 (2026-03). 기본 진입점은 Telegram 봇으로 이전 예정.

### 아키텍처
```
메신저 (Telegram/Dooray/...)
  ↕ PlatformAdapter
MCP Channel Server (stdio)
  ↕
Claude Code
```

### 구조
- `core/` — PlatformAdapter 인터페이스, MCP Channel 서버, auth (allowlist/pairing)
- `adapters/dooray/` — Dooray Messenger 어댑터 (Hub polling 방식)
- `index.ts` — 진입점
- 런타임: Bun

### 빌드/테스트
```bash
cd c1 && bun install && bun test
```

---

## Infra (infra/supabase/)

> PostgreSQL 마이그레이션 21개 (00001~00021). Supabase 기반 클라우드 레이어.

### 주요 테이블
- `c4_tasks`, `c4_documents`, `c4_projects` — C4 핵심 데이터
- `c1_channels`, `c1_messages`, `c1_participants`, `c1_channel_summaries` — C1 메시징
- `c1_members` — 통합 멤버 모델 (user/agent/system + presence)
- RLS 정책 (migration 00014: 보안 픽스)

---

## Knowledge Pipeline (지식 피드백 루프)

> 프로젝트 전체에서 "왜(why)"를 기록하고, 축적된 지식으로 다음 시도를 고도화하는 4-layer 파이프라인.

### 파이프라인 흐름
```
Plan (knowledge_search) → Task DoD (Rationale 포함) → Worker (knowledge_context 주입)
     ↑                                                        ↓
pattern_suggest ← distill ← autoRecordKnowledge ← Worker 완료 (handoff)
```

### Layer 1: Write (기록 강화)
- **autoRecordKnowledge**: 태스크 완료 시 handoff JSON을 파싱하여 discoveries/concerns/rationale 추출
- **handoff 구조**: `{summary, files_changed, discoveries, concerns, rationale}`
- **Worker가 기록**: c4_submit 시 handoff에 구조화된 데이터 전달 → 자동 knowledge 기록

### Layer 2: Read (조회 통합)
- `/c4-plan` Phase 0.1: `c4_knowledge_search` + `c4_pattern_suggest` 자동 호출
- `/c4-refine` Phase 0.5: 과거 refine 패턴 조회
- DoD에 **Rationale** 섹션 필수 포함

### Layer 3: Inject (주입)
- `AssignTask`에서 `enrichWithKnowledge` → `TaskAssignment.knowledge_context`에 관련 지식 주입
- Worker는 과거 패턴/인사이트를 참조하여 구현

### Layer 4: Converge (수렴)
- `/c4-finish`에서 `c4_knowledge_distill` 자동 호출 (docs ≥ 5건)
- `/c4-refine`에서 반복 이슈 패턴을 pattern으로 자동 기록
- `c4_knowledge_publish` / `c4_knowledge_pull`로 프로젝트 간 공유

### 핵심 규칙
- **c4_submit 시 handoff에 reasoning 포함**: discoveries, concerns, rationale 필드 활용
- **계획 시 과거 지식 조회 필수**: `/c4-plan` Phase 0.1에서 knowledge_search 수행
- **Refine 루프에서 교훈 기록**: 반복 이슈 → pattern 자동 승격

---

## C3 EventBus (internal/eventbus/)

> gRPC UDS daemon + WebSocket bridge + DLQ. 78 테스트.

### 기능
- **v1**: gRPC daemon (UDS), rules YAML, Store/Dispatcher
- **v2**: Python sidecar response piggyback (grpcio 의존성 제거)
- **v3**: ToggleRule, ListLogs, GetStats, ReplayEvents, Embedded auto-start
- **v4**: correlation_id, DLQ, Filter v2 ($eq/$ne/$gt/$lt/$in/$regex/$exists), WebSocket bridge, HMAC-SHA256 webhook

### 이벤트 종류 (18종)
```
task.completed, task.updated, task.blocked, task.created
checkpoint.approved, checkpoint.rejected
review.changes_requested
validation.passed, validation.failed
knowledge.recorded, knowledge.searched
hub.job.completed, hub.job.failed, hub.worker.started, hub.worker.offline
tool.called       ← C7 Observe가 발행 (tool name, latency_ms, error bool)
guard.denied      ← C6 Guard가 발행 (ActionDeny 시)
```

---

## Hub (Supabase 기반)

> Hub 기능은 Supabase + c4-core로 이전 완료. Worker Pull + Lease 모델.
> 설정: `cloud.url` + `cloud.anon_key` 필요. `serve.hub.enabled: true` 활성화.

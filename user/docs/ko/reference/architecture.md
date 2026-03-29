# 아키텍처 레퍼런스

CQ는 **AI를 위한 외장 두뇌**입니다 — 모든 AI 대화가 영구적인 지식이 되고, 품질 게이트가 코드 무결성을 보장하며, 분산 실행이 원격 GPU 학습을 가능하게 하는 시스템입니다. 이 문서는 핵심 컴포넌트를 설명합니다.

---

## 시스템 개요

```
+------------------+          +----------------------------+
| 로컬 (Thin Agent) |  JWT    | 클라우드 (Supabase)          |
|                   |<------->|                             |
| 손발:             |         | 두뇌:                        |
|  +- Files / Git  |         |  +- Tasks (Postgres)        |
|  +- Build / Test |         |  +- Knowledge (pgvector)    |
|  +- LSP analysis |         |  +- LLM Proxy (Edge Fn)    |
|  +- MCP bridge   |         |  +- Quality Gates           |
|                   |         |  +- Hub (distributed jobs)  |
| Service (cq serve)|   WSS  |                             |
|  +- Relay --------+-------->|  Relay (Fly.io)             |
|  +- EventBus     |         |  +- NAT traversal            |
|  +- Token refresh|         |                             |
+------------------+          | External Brain (CF Worker)  |
                              |  +- OAuth 2.1 MCP proxy     |
Any AI (ChatGPT,   --- MCP -->|  +- Knowledge record/search |
 Claude, Gemini)              |  +- Session summary         |
                              +----------------------------+

solo:       모든 것이 로컬 (SQLite + 본인 API 키)
connected:  두뇌는 클라우드 + relay (로그인 + serve)
full:       connected + GPU Worker + Research Loop
```

---

## 배포 티어

| 티어 | 데이터 SSOT | LLM | 설정 |
|------|-----------|-----|------|
| **solo** | 로컬 SQLite | 사용자 API 키 | `config.yaml` 필수 |
| **connected** | Supabase (클라우드 우선) | PI Lab LLM Proxy | `cq auth login` + `cq serve` |
| **full** | Supabase (클라우드 우선) | PI Lab LLM Proxy | connected + GPU Worker |

- 클라우드 장애 시 SQLite로 폴백 (읽기 전용)
- 약 70개 도구는 클라우드 우선, 약 48개 도구는 로컬 필요 (파일/git/빌드)
- External Brain: ChatGPT/Claude/Gemini가 OAuth MCP를 통해 연결 (로컬 설치 불필요)

---

## Go MCP 서버 (c4-core/)

기본 MCP 서버. stdio 전송으로 217개 도구를 제공합니다.

```
Claude Code -> Go MCP Server (stdio, 217 도구)
                +-> Go 네이티브 (28): state, tasks, files, git, validation, config
                +-> Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                +-> Soul/Persona/Twin (10): soul_evolve, persona_learn, twin_record, ...
                +-> LLM Gateway (3): llm_call, llm_providers, llm_costs
                +-> CDP + WebMCP (5): cdp_run, webmcp_discover, web_fetch, ...
                +-> Drive (6): upload, download, list, delete, info, mkdir
                +-> File Index (2): fileindex_search, fileindex_status
                +-> Session (3): session_index, session_summarize, session_snapshot
                +-> Memory (1): memory_import
                +-> Relay (2): cq_workers, cq_relay_call
                +-> Knowledge (13): record, search, distill, ingest, sync, publish, ...
                +-> Hub Client (19, conditional): job, worker, DAG, artifact, cron
                +-> Worker Standby (3, Hub): standby, complete, shutdown
                +-> C7 Observe (4, build tag): metrics, logs, trace, status
                +-> C6 Guard (5, build tag): check, audit, policy, deny
                +-> C8 Gate (6, build tag): webhook, schedule, slack, github, ...
                +-> EventSink (1) + HubPoller (1)
                +-> JSON-RPC proxy (10) -> Python Sidecar
```

### 도구 등급

- **Core** (40개 도구): 항상 로드됨, 즉시 사용 가능
- **Extended** (177개 도구): 필요 시 로드됨, 초기화 후 사용 가능
- **Conditional**: Hub 도구는 `serve.hub.enabled: true` 필요; C7/C6/C8은 빌드 태그 필요

### 패키지 구조

```
c4-core/
+-- cmd/c4/           # CLI (cobra) + MCP 서버 진입점
+-- internal/
    +-- mcp/          # Registry + stdio 전송
    |   +-- apps/     # MCP Apps ResourceStore + 내장 위젯 HTML
    |   +-- handlers/ # 도구별 핸들러
    +-- bridge/       # Python sidecar 매니저 (JSON-RPC/TCP, lazy start)
    +-- task/         # TaskStore (SQLite, Memory, Supabase)
    +-- state/        # 상태 머신 (INIT -> COMPLETE)
    +-- worker/       # Worker 매니저 + 생존성 (watchdog, safeGo)
    +-- validation/   # 검증 실행기 (go test, pytest, cargo test 자동 감지)
    +-- config/       # 설정 매니저 (YAML, env, 경제적 프리셋)
    +-- cloud/        # Auth (OAuth), CloudStore, HybridStore, TokenProvider
    +-- hub/          # Hub REST+WS 클라이언트 (26개 도구)
    +-- daemon/       # 로컬 작업 스케줄러 (GPU 인식)
    +-- eventbus/     # C3 EventBus v4 (gRPC, WS bridge, DLQ, filter v2)
    +-- knowledge/    # Knowledge (FTS5 + Vector + Embedding + Sync)
    +-- research/     # 연구 반복 저장소
    +-- c2/           # Workspace/Profile/Persona + webcontent
    +-- drive/        # Drive 클라이언트 (TUS 재개 가능 업로드)
    +-- fileindex/    # 크로스 디바이스 파일 검색
    +-- session/      # 세션 추적 + LLM 요약기
    +-- memory/       # ChatGPT/Claude 세션 임포트 파이프라인
    +-- relay/        # WebSocket relay 클라이언트 (자동 재시작)
    +-- llm/          # LLM Gateway (Anthropic, OpenAI, Gemini, Ollama)
    +-- cdp/          # Chrome DevTools Protocol + WebMCP
    +-- observe/      # C7 Observe (c7_observe 빌드 태그)
    +-- guard/        # C6 Guard (c6_guard 빌드 태그)
    +-- gate/         # C8 Gate (c8_gate 빌드 태그)
```

### 빌드 및 설치

```bash
# 빌드 + 설치 (중요 -- 반드시 make install 사용)
cd c4-core && make install

# 테스트
cd c4-core && go test ./...

# 환경 진단
cq doctor
```

---

## Worker 생존성 (v1.44-v1.46)

Worker는 운영자 개입 없이 충돌, 네트워크 장애, 과부하에서 자가 복구하도록 설계되었습니다.

### OS Watchdog

Worker는 `--watchdog` 플래그와 함께 시스템 서비스로 등록됩니다. OS 서비스 매니저(systemd/launchd)가 프로세스 종료 시 자동으로 재시작합니다.

```
ExecStart=/usr/local/bin/cq serve --watchdog
Restart=always
RestartSec=5
```

### safeGo

모든 goroutine은 `safeGo`를 통해 실행됩니다. `safeGo`는 패닉을 복구하여 프로세스를 종료시키는 대신 RingBuffer에 로깅하는 래퍼입니다.

```
safeGo(func() {
    // goroutine 본문 -- 패닉이 잡혀서 로깅되며, 프로세스가 절대 종료되지 않음
})
```

### Heartbeat Circuit Breaker

Worker는 Supabase에 주기적으로 heartbeat를 전송합니다. heartbeat가 반복적으로 실패하면 circuit breaker가 열리고 Worker는 조용히 계속 실패하는 대신 재연결 루프로 진입합니다.

```
Heartbeat tick
    |-- 성공 -> 실패 횟수 초기화
    |-- 실패 -> 횟수 증가
    +-- 횟수 >= 임계값 -> circuit 열림 -> 지수 백오프 -> 재시도
```

### 충돌 로그 수집

고정 크기 `RingBuffer`가 마지막 N개의 로그 줄을 메모리에 저장합니다. 충돌 또는 패닉 복구 시 `UploadCrashLog`가 버퍼를 Supabase에 전송하여 사후 분석을 지원합니다.

### 429 적응형 백오프

Hub 또는 LLM Gateway가 HTTP 429를 반환하면 Worker는 `Retry-After` 헤더를 읽고 고정 재시도 간격 대신 정확히 해당 시간만큼 대기합니다.

### Relay WebSocket 자동 재시작

Relay WebSocket 연결은 스스로를 모니터링합니다. 연결이 끊기면(네트워크 장애, relay 재시작) 지수 백오프로 자동 재연결합니다 — `cq serve` 재시작이 필요 없습니다.

---

## Growth Loop / 페르소나 시스템 (v1.40-v1.42)

페르소나 시스템은 사용자 교정에서 학습하고 안정적인 패턴을 공유 지식으로 승격합니다.

```
사용자 교정 / 명시적 피드백
    |
    v
PreferenceLedger (선호도별 횟수)
    |
    v
GrowthMetrics (세션 교정 + 트렌드)
    |
    v
RulePromoter
    |-- 횟수 >= 3 -> hint로 승격 (CLAUDE.md hint 섹션)
    |-- 횟수 >= 5 -> rule로 승격 (.claude/rules/<topic>.md)
    |
    v
GlobalPromoter (비개인화)
    +-> 커뮤니티 지식 풀 (GlobalPromoter를 통해 모든 사용자와 공유)
```

### 컴포넌트

| 컴포넌트 | 역할 |
|---------|------|
| `PreferenceLedger` | 각 사용자 선호도를 발생 횟수와 함께 추적 |
| `GrowthMetrics` | 세션별 교정 횟수 + 멀티 세션 트렌드 |
| `RulePromoter` | 횟수 임계값에서 hint를 rule로 졸업 (3→hint, 5→rule) |
| `GlobalPromoter` | 개인 컨텍스트를 제거하고 패턴을 커뮤니티 지식에 게시 |

### 출력 파일

- Hint와 rule은 `CLAUDE.md`와 `.claude/rules/`에 직접 작성됩니다
- Rule은 다음 Claude Code 세션 로드 시 즉시 적용됩니다
- GlobalPromoter 출력은 크로스 유저 공유를 위해 `c4_knowledge_publish`에 공급됩니다

---

## TUI 대시보드 (v1.44-v1.46)

BubbleTea 기반으로 구축된 세 가지 터미널 UI 명령.

### `cq jobs`

풀 기능 작업 모니터:
- **상세 패널**: 선택된 작업의 스펙, 로그, 메트릭을 표시하는 사이드 패널
- **적응형 멀티행 차트**: 터미널 높이에 따라 행이 확장되는 메트릭 차트
- **Compare 모드**: 두 작업을 선택하여 메트릭을 나란히 비교

### `cq workers`

Worker Connection Board — 모든 등록된 Worker의 상태, 어피니티 점수, 마지막 heartbeat, 현재 작업 할당을 표시합니다.

### `cq dashboard`

통합 보드 메뉴 — `jobs`, `workers`, 또는 프로젝트 상태 뷰로 라우팅하는 진입점.

```
cq dashboard
    +-- [j] Jobs 모니터     (cq jobs)
    +-- [w] Workers 보드    (cq workers)
    +-- [s] 프로젝트 상태   (cq status)
```

---

## 세션 인텔리전스 (v1.39-v1.41)

### /done vs /exit 분리

| 명령 | 캡처 깊이 | 사용 시점 |
|------|---------|---------|
| `/done` | 전체 — 구조화된 요약, 지식 추출, 페르소나 업데이트 | 실제 작업 완료 시 |
| `/exit` | 경량 — 최소한의 메타데이터만 | 중단하거나 빠르게 닫을 때 |

### 요약 프롬프트

`/done`은 실행 가능한 지식을 추출하도록 설계된 심층 프롬프트를 사용합니다:
- 내린 결정과 근거
- 발견된 패턴
- 발생한 문제와 해결 방법
- 구체적인 시작 지점이 포함된 다음 단계

### 폴백 처리

- **글로벌 DB 폴백**: Supabase를 사용할 수 없는 경우 세션 요약을 로컬 SQLite에 기록
- **LLM 실패 메타데이터**: 요약 LLM 호출이 실패하면 원시 세션 텍스트를 `llm_failed=true`와 함께 저장하고 다음 연결 시 재시도

---

## MCP 앱 (위젯 시스템)

`format=widget`으로 도구를 호출하면 응답에 `_meta.ui.resourceUri`가 포함됩니다. MCP 클라이언트는 `resources/read`를 통해 HTML을 가져와 샌드박스 iframe에 렌더링합니다.

```
Tool call (format=widget)
  -> handler가 {data: {...}, _meta: {ui: {resourceUri: "ui://cq/..."}}} 반환
  -> 클라이언트가 resources/read("ui://cq/...") 호출
  -> ResourceStore가 내장 HTML 반환
  -> 클라이언트가 샌드박스 iframe에 렌더링
```

| 위젯 URI | 도구 | 설명 |
|---------|------|------|
| `ui://cq/dashboard` | `c4_dashboard` | 프로젝트 상태 요약 |
| `ui://cq/job-progress` | `c4_job_status` | 작업 진행률 |
| `ui://cq/job-result` | `c4_job_summary` | 작업 결과 |
| `ui://cq/experiment-compare` | `c4_experiment_search` | 실험 비교 |
| `ui://cq/task-graph` | `c4_task_graph` | 태스크 의존성 그래프 |
| `ui://cq/nodes-map` | `c4_nodes_map` | 연결된 노드 맵 |
| `ui://cq/knowledge-feed` | `c4_knowledge_search` | 지식 피드 |
| `ui://cq/cost-tracker` | `c4_llm_costs` | LLM 비용 추적기 |
| `ui://cq/test-results` | `c4_run_validation` | 테스트 결과 |
| `ui://cq/git-diff` | `c4_diff_summary` | Git diff 뷰어 |
| `ui://cq/error-trace` | `c4_error_trace` | 에러 트레이스 뷰어 |

---

## 지식 시스템

4계층 파이프라인: 모든 태스크 결정이 향후 태스크를 위한 검색 가능한 지식이 됩니다.

```
Plan (knowledge_search) -> Task DoD (Rationale) -> Worker (knowledge_context 주입)
     ^                                                       |
pattern_suggest <- distill <- autoRecordKnowledge <- Worker complete (handoff)
```

- **FTS5**: 모든 지식 레코드에 대한 전문 검색
- **pgvector**: OpenAI 1536-dim 임베딩 (또는 Ollama 768-dim nomic-embed-text)
- **3-way RRF**: FTS + vector + 인기도 점수의 순위 융합
- **Auto-distill**: 지식 수 >= 5일 때 `/finish`에 의해 트리거
- **클라우드 동기화**: 로컬 SQLite <-> Supabase pgvector 동기화
- **크로스 프로젝트**: 공유를 위한 `c4_knowledge_publish` / `c4_knowledge_pull`

### 지식 handoff (c4_submit)

Worker는 태스크와 함께 구조화된 handoff를 제출합니다:

```json
{
  "summary": "구현한 내용",
  "files_changed": ["src/feature.go"],
  "discoveries": ["패턴 X가 Y보다 효과적"],
  "concerns": ["엣지 케이스 Z 미처리"],
  "rationale": "접근 방식 A를 선택한 이유"
}
```

이것은 자동으로 파싱되어 향후 Worker를 위한 지식으로 기록됩니다.

---

## Hub (분산 실행)

Hub는 Supabase PostgreSQL 기반의 분산 작업 큐입니다. Worker는 임대 모델로 작업을 가져갑니다.

```
개발자 (노트북)
  +-- c4_job_submit(spec, routing={tags: ["gpu"]}) -->+
                                                      |
                                    Supabase: hub_jobs INSERT
                                              | pg_notify('new_job')
                                              v
                                    Worker (원격 GPU 서버)
                                      +- ClaimJob (임대)
                                      +- 실행
                                      +- 아티팩트 업로드
                                      +- CompleteJob
```

### DAG 파이프라인

```
c4_hub_dag_create (노드 + 엣지)
    |
    v (위상 정렬 -> 루트 노드 자동 제출)
    v
Worker가 노드 완료 -> advance_dag -> 다음 레이어 해제
    |
    v
모든 노드 완료 -> DAG 완료 이벤트
```

### Worker 어피니티

Worker는 어피니티 점수를 기반으로 자동 라우팅됩니다:

```
affinity_score = project_match * 10 + tag_match * 3 + recency * 2 + success_rate * 5
```

어피니티 점수 확인: `cq hub workers` (`AFFINITY` 컬럼 표시).

---

## Relay (NAT 통과)

Relay는 외부 MCP 클라이언트가 NAT를 통해 로컬 Worker에 접근할 수 있게 합니다.

```
외부 MCP 클라이언트 (Cursor / Codex / Gemini CLI)
    | HTTPS (MCP over HTTP)
    v
cq-relay.fly.dev  [Go relay 서버]
    ^ WSS (아웃바운드, Worker가 먼저 연결)
cq serve  [로컬 / 클라우드 Worker]
    |
    v
Go MCP Server (stdio) + Python Sidecar
```

인증 흐름:
1. `cq auth login` -> Supabase Auth -> JWT 발급 + relay URL 자동 설정
2. `cq serve` 시작 -> relay WSS 연결 (Authorization: Bearer JWT)
3. Relay가 토큰 검증, Worker 터널 등록
4. 외부 클라이언트 -> `https://cq-relay.fly.dev/<worker-id>` -> relay -> WSS -> Worker

Relay WebSocket은 연결 끊김 시 자동 재시작합니다 (위의 Worker 생존성 참조).

---

## EventBus (C3)

gRPC UDS 데몬, WebSocket 브리지 포함. 18가지 이벤트 타입. 테스트 78개.

```
EventBus (gRPC UDS)
    |-- 규칙 엔진 (YAML 라우팅)
    |-- DLQ (dead letter queue)
    |-- WebSocket 브리지 (외부 구독자)
    +-- HMAC-SHA256 webhook 전달
```

이벤트 타입:

| 카테고리 | 이벤트 |
|---------|--------|
| 태스크 | `task.completed`, `task.updated`, `task.blocked`, `task.created` |
| 체크포인트 | `checkpoint.approved`, `checkpoint.rejected` |
| 리뷰 | `review.changes_requested` |
| 검증 | `validation.passed`, `validation.failed` |
| 지식 | `knowledge.recorded`, `knowledge.searched` |
| Hub | `hub.job.completed`, `hub.job.failed`, `hub.worker.started`, `hub.worker.offline` |
| 관측성 | `tool.called` (C7), `guard.denied` (C6) |

---

## External Brain (Cloudflare Worker)

OAuth 2.1 MCP 프록시. 모든 AI(ChatGPT, Claude web, Gemini)가 로컬 설치 없이 CQ 지식에 접근할 수 있습니다.

External Brain을 통해 노출되는 도구:

| 도구 | 설명 |
|------|------|
| `c4_knowledge_record` | AI가 능동적으로 지식 저장 (도구 설명의 5가지 조건 트리거) |
| `c4_knowledge_search` | Vector + FTS + ilike 3단계 폴백 검색 |
| `c4_session_summary` | 대화 종료 시 완전한 세션 요약 캡처 |
| `c4_status` | 현재 프로젝트 상태 읽기 |

---

## Python Sidecar (c4/)

Go MCP 서버가 JSON-RPC/TCP를 통해 10개 도구를 Python에 위임합니다 (지연 시작).

```
Go MCP Server -- JSON-RPC/TCP --> Python Sidecar (10개 도구)
                                    +-> LSP (7개): find_symbol, get_overview, replace_body,
                                    |          insert_before/after, rename, find_refs
                                    |          (Python/JS/TS 전용 -- Go/Rust: c4_search_for_pattern 사용)
                                    +-> Doc (2개): parse_document, extract_text
                                    +-> Onboard (1개): c4_onboard
```

- **Lazy Start**: 첫 번째 프록시 도구 호출 시에만 Sidecar 시작
- **점진적 폴백**: Python/uv를 사용할 수 없으면 LSP/Doc 도구가 비활성화됨 (충돌 아님)

---

## 상태 머신

```
INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> REFINE -> POLISH -> COMPLETE
                                          |
                                          +-> HALTED (재개 가능)
```

| 상태 | 의미 |
|------|------|
| INIT | 프로젝트 생성됨, 태스크 없음 |
| DISCOVERY | 요구사항 수집 중 (c4-plan Phase 1) |
| DESIGN | 아키텍처 결정 중 (c4-plan Phase 2) |
| PLAN | 태스크 생성됨, 실행 준비 완료 |
| EXECUTE | Worker 활성, 태스크 점유 중 |
| CHECKPOINT | 페이즈 게이트 도달, 리뷰 진행 중 |
| HALTED | 실행 일시 중단, `/run`으로 재개 가능 |
| COMPLETE | 모든 태스크 완료, `/finish` 준비 완료 |

---

## 보안: Permission Hook

모든 도구 사용 및 셸 실행에 대한 2계층 게이트:

```
PreToolUse 이벤트
    |
    v
c4-gate.sh (패턴 매칭)
    |-- allow_patterns -> 즉시 허용
    |-- model 모드 -> Haiku API 결정
    |-- block_patterns -> 차단 (감사 로그 포함)
    +-- 폴백 -> 내장 안전 패턴

PermissionRequest 이벤트
    |
    v
c4-permission-reviewer.sh (Haiku 분류)
```

설정 (`.c4/config.yaml`):

```yaml
permission_reviewer:
  enabled: true
  mode: hook        # "hook" (정규식 전용) 또는 "model" (Haiku API)
  auto_approve: true
  allow_patterns: []
  block_patterns: []
```

---

## Supabase 스키마 (주요 테이블)

| 테이블 | 용도 |
|--------|------|
| `c4_tasks` | 태스크 큐 (상태, 할당, 커밋 SHA) |
| `c4_documents` | 지식 레코드 (내용, 임베딩, FTS) |
| `c4_projects` | 프로젝트 레지스트리 (소유자, 설정) |
| `hub_jobs` | 분산 작업 큐 (스펙, 상태, 임대) |
| `hub_workers` | 등록된 Worker (기능, 어피니티) |
| `hub_dags` | DAG 파이프라인 정의 |
| `hub_cron_schedules` | Cron 작업 정의 |
| `c4_drive_files` | Drive 파일 메타데이터 (해시, URL, 버전) |
| `c4_datasets` | 콘텐츠 주소 지정 버전 관리를 포함한 데이터셋 레지스트리 |
| `c1_messages` | 세션 간 및 메시징 채널 메시지 |
| `notification_channels` | Telegram/Dooray 알림 설정 |

마이그레이션 52개, 사용자 대상 테이블 전체에 RLS 정책 적용, 임베딩용 pgvector 확장 사용.

---

## 스킬 (v1.46)

연구, ML, 데이터, 오케스트레이션 도메인에 걸친 42개의 스킬. 스킬은 `.claude/skills/`에서 로드되며 `/skill-name`으로 호출합니다.

| 카테고리 | 수 | 예시 |
|---------|-----|------|
| 연구 & 실험 | 8 | `c9-loop`, `c9-survey`, `research-loop`, `experiment-workflow` |
| ML / 데이터 사이언스 | 13 | `eda-profiler`, `pytorch-model-builder`, `transfer-learning-expert` |
| 오케스트레이션 | 9 | `plan`, `run`, `finish`, `quick`, `pi` |
| 개발 품질 | 7 | `tdd-cycle`, `spec-first`, `debugging`, `company-review` |
| 기타 | 5 | `release`, `incident-response`, `standby`, `pdf` |

스킬은 CQ MCP 도구와 직접 상호작용합니다 — 스킬은 별도의 바이너리가 아닌 MCP 서버를 구동하는 구조화된 프롬프트입니다.

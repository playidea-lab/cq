# 아키텍처 레퍼런스

CQ는 **AI를 위한 외장 두뇌**입니다 — 모든 AI 대화가 영구적인 지식이 되고, 품질 게이트가 코드 무결성을 보장하며, 분산 실행이 원격 GPU 훈련을 가능하게 하는 시스템입니다. 이 문서는 핵심 컴포넌트를 설명합니다.

---

## 시스템 개요

```
+------------------+          +----------------------------+
| 로컬 (Thin Agent) |  JWT    | 클라우드 (Supabase)          |
|                   |<------->|                             |
| 손발:             |         | 두뇌:                       |
|  +- 파일 / Git   |         |  +- 태스크 (Postgres)       |
|  +- 빌드 / 테스트|         |  +- 지식 (pgvector)        |
|  +- LSP 분석     |         |  +- LLM Proxy (Edge Fn)    |
|  +- MCP 브리지   |         |  +- 품질 게이트             |
|                   |         |  +- Hub (분산 작업)         |
| 서비스 (cq serve) |   WSS  |                             |
|  +- Relay --------+-------->|  Relay (Fly.io)             |
|  +- EventBus     |         |  +- NAT 통과                |
|  +- 토큰 갱신    |         |                             |
+------------------+          | External Brain (CF Worker)  |
                              |  +- OAuth 2.1 MCP 프록시   |
어떤 AI든 (ChatGPT, --- MCP -->|  +- 지식 기록/검색         |
 Claude, Gemini)              |  +- 세션 요약              |
                              +----------------------------+

solo:       모든 것이 로컬 (SQLite + 본인 API 키)
connected:  클라우드 두뇌 + relay (로그인 + serve)
full:       Connected + GPU Worker + Research Loop
```

---

## 배포 티어

| 티어 | 데이터 SSOT | LLM | 설정 |
|------|-----------|-----|------|
| **solo** | 로컬 SQLite | 사용자 API 키 | `config.yaml` 필요 |
| **connected** | Supabase (클라우드 우선) | PI Lab LLM Proxy | `cq auth login` + `cq serve` |
| **full** | Supabase (클라우드 우선) | PI Lab LLM Proxy | Connected + GPU Worker |

- 클라우드 장애 시 SQLite로 폴백 (읽기 전용)
- ~70개 도구가 클라우드 우선, ~48개 도구는 로컬 필요 (파일/git/빌드)
- External Brain: ChatGPT/Claude/Gemini가 OAuth MCP를 통해 연결 (로컬 설치 불필요)

---

## Go MCP 서버 (c4-core/)

기본 MCP 서버. stdio 전송으로 169개 도구 제공.

```
Claude Code -> Go MCP 서버 (stdio, 169개 도구)
                +-> Go 네이티브 (28개): 상태, 태스크, 파일, git, 유효성 검사, 설정
                +-> Go + SQLite (13개): 스펙, 설계, 체크포인트, 아티팩트, lighthouse
                +-> Soul/Persona/Twin (10개): soul_evolve, persona_learn, twin_record, ...
                +-> LLM Gateway (3개): llm_call, llm_providers, llm_costs
                +-> CDP + WebMCP (5개): cdp_run, webmcp_discover, web_fetch, ...
                +-> Drive (6개): upload, download, list, delete, info, mkdir
                +-> 파일 인덱스 (2개): fileindex_search, fileindex_status
                +-> 세션 (3개): session_index, session_summarize, session_snapshot
                +-> 메모리 (1개): memory_import
                +-> Relay (2개): cq_workers, cq_relay_call
                +-> 지식 (13개): record, search, distill, ingest, sync, publish, ...
                +-> Hub 클라이언트 (19개, 조건부): 작업, worker, DAG, 아티팩트, cron
                +-> Worker 대기 모드 (3개, Hub): standby, complete, shutdown
                +-> C7 Observe (4개, 빌드 태그): metrics, logs, trace, status
                +-> C6 Guard (5개, 빌드 태그): check, audit, policy, deny
                +-> C8 Gate (6개, 빌드 태그): webhook, schedule, slack, github, ...
                +-> EventSink (1개) + HubPoller (1개)
                +-> JSON-RPC 프록시 (10개) -> Python 사이드카
```

### 도구 티어링

- **Core** (40개 도구): 항상 로드됨, 즉시 사용 가능
- **Extended** (129개 도구): 요청 시 로드됨, 초기화 후 사용 가능
- **조건부**: Hub 도구는 `serve.hub.enabled: true` 필요; C7/C6/C8는 빌드 태그 필요

### 패키지 구조

```
c4-core/
+-- cmd/c4/           # CLI (cobra) + MCP 서버 진입점
+-- internal/
    +-- mcp/          # 레지스트리 + stdio 전송
    |   +-- apps/     # MCP Apps ResourceStore + 내장 위젯 HTML
    |   +-- handlers/ # 도구별 핸들러
    +-- bridge/       # Python 사이드카 매니저 (JSON-RPC/TCP, 지연 시작)
    +-- task/         # TaskStore (SQLite, 메모리, Supabase)
    +-- state/        # 상태 머신 (INIT -> COMPLETE)
    +-- worker/       # Worker 매니저
    +-- validation/   # 유효성 검사 실행기 (go test, pytest, cargo test 자동 감지)
    +-- config/       # 설정 매니저 (YAML, env, 경제적 프리셋)
    +-- cloud/        # Auth (OAuth), CloudStore, HybridStore, TokenProvider
    +-- hub/          # Hub REST+WS 클라이언트 (26개 도구)
    +-- daemon/       # 로컬 작업 스케줄러 (GPU 인식)
    +-- eventbus/     # C3 EventBus v4 (gRPC, WS 브리지, DLQ, filter v2)
    +-- knowledge/    # 지식 (FTS5 + 벡터 + 임베딩 + 동기화)
    +-- research/     # 연구 반복 저장소
    +-- c2/           # 워크스페이스/프로파일/Persona + 웹 콘텐츠
    +-- drive/        # Drive 클라이언트 (TUS 재개 가능 업로드)
    +-- fileindex/    # 크로스 디바이스 파일 검색
    +-- session/      # 세션 추적 + LLM 요약기
    +-- memory/       # ChatGPT/Claude 세션 임포트 파이프라인
    +-- relay/        # WebSocket relay 클라이언트
    +-- llm/          # LLM Gateway (Anthropic, OpenAI, Gemini, Ollama)
    +-- cdp/          # Chrome DevTools Protocol + WebMCP
    +-- observe/      # C7 Observe (c7_observe 빌드 태그)
    +-- guard/        # C6 Guard (c6_guard 빌드 태그)
    +-- gate/         # C8 Gate (c8_gate 빌드 태그)
```

### 빌드 및 설치

```bash
# 빌드 + 설치 (중요 — 항상 make install 사용)
cd c4-core && make install

# 테스트
cd c4-core && go test ./...

# 환경 진단
cq doctor
```

---

## MCP Apps (위젯 시스템)

`format=widget`으로 도구를 호출하면 응답에 `_meta.ui.resourceUri`가 포함됩니다. MCP 클라이언트가 `resources/read`를 통해 HTML을 가져와 샌드박스 iframe에 렌더링합니다.

```
도구 호출 (format=widget)
  -> 핸들러가 {data: {...}, _meta: {ui: {resourceUri: "ui://cq/..."}}} 반환
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
| `ui://cq/error-trace` | `c4_error_trace` | 오류 트레이스 뷰어 |

---

## 지식 시스템

4층 파이프라인: 모든 태스크 결정이 이후 태스크를 위한 검색 가능한 지식이 됩니다.

```
계획 (knowledge_search) -> 태스크 DoD (근거) -> Worker (knowledge_context 주입)
     ^                                                       |
pattern_suggest <- distill <- autoRecordKnowledge <- Worker 완료 (핸드오프)
```

- **FTS5**: 모든 지식 기록에 대한 전문 검색
- **pgvector**: OpenAI 1536-dim 임베딩 (또는 Ollama 768-dim nomic-embed-text)
- **3-way RRF**: FTS + 벡터 + 인기도 점수의 순위 융합
- **자동 증류**: 지식 수 >= 5일 때 `/c4-finish`에 의해 트리거됨
- **클라우드 동기화**: 로컬 SQLite <-> Supabase pgvector 동기화
- **크로스 프로젝트**: `c4_knowledge_publish` / `c4_knowledge_pull`로 공유

### 지식 핸드오프 (c4_submit)

Worker가 태스크와 함께 구조화된 핸드오프를 제출합니다:

```json
{
  "summary": "구현된 것",
  "files_changed": ["src/feature.go"],
  "discoveries": ["패턴 X가 Y보다 더 잘 작동함"],
  "concerns": ["엣지 케이스 Z가 처리되지 않음"],
  "rationale": "접근 방법 A를 선택한 이유"
}
```

이것이 자동으로 파싱되어 이후 Worker를 위한 지식으로 기록됩니다.

---

## Hub (분산 실행)

Hub는 Supabase PostgreSQL 기반의 분산 작업 큐입니다. Worker들이 리스 모델로 작업을 가져갑니다.

```
개발자 (노트북)
  +-- c4_job_submit(spec, routing={tags: ["gpu"]}) -->+
                                                      |
                                    Supabase: hub_jobs INSERT
                                              | pg_notify('new_job')
                                              v
                                    Worker (원격 GPU 서버)
                                      +- ClaimJob (리스)
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

### Worker Affinity

Worker가 affinity 점수에 기반하여 자동으로 라우팅됩니다:

```
affinity_score = project_match * 10 + tag_match * 3 + recency * 2 + success_rate * 5
```

Affinity 점수 확인: `cq hub workers` (`AFFINITY` 열 표시).

---

## Relay (NAT 통과)

Relay가 외부 MCP 클라이언트에서 NAT를 통해 로컬 Worker에 도달할 수 있게 합니다.

```
외부 MCP 클라이언트 (Cursor / Codex / Gemini CLI)
    | HTTPS (HTTP 위의 MCP)
    v
cq-relay.fly.dev  [Go relay 서버]
    ^ WSS (아웃바운드, Worker가 먼저 연결)
cq serve  [로컬 / 클라우드 Worker]
    |
    v
Go MCP 서버 (stdio) + Python 사이드카
```

인증 플로우:
1. `cq auth login` -> Supabase Auth -> JWT 발급 + relay URL 자동 설정
2. `cq serve` 시작 -> relay WSS 연결 (Authorization: Bearer JWT)
3. Relay가 토큰 검증, Worker 터널 등록
4. 외부 클라이언트 -> `https://cq-relay.fly.dev/<worker-id>` -> relay -> WSS -> Worker

---

## EventBus (C3)

gRPC UDS 데몬 + WebSocket 브리지. 18개 이벤트 타입. 78개 테스트.

```
EventBus (gRPC UDS)
    |-- 규칙 엔진 (YAML 라우팅)
    |-- DLQ (데드 레터 큐)
    |-- WebSocket 브리지 (외부 구독자)
    +-- HMAC-SHA256 webhook 전달
```

이벤트 타입:

| 카테고리 | 이벤트 |
|---------|--------|
| 태스크 | `task.completed`, `task.updated`, `task.blocked`, `task.created` |
| 체크포인트 | `checkpoint.approved`, `checkpoint.rejected` |
| 리뷰 | `review.changes_requested` |
| 유효성 검사 | `validation.passed`, `validation.failed` |
| 지식 | `knowledge.recorded`, `knowledge.searched` |
| Hub | `hub.job.completed`, `hub.job.failed`, `hub.worker.started`, `hub.worker.offline` |
| 관측성 | `tool.called` (C7), `guard.denied` (C6) |

---

## External Brain (Cloudflare Worker)

OAuth 2.1 MCP 프록시. 어떤 AI든(ChatGPT, Claude 웹, Gemini) 로컬 설치 없이 CQ 지식에 접근 가능.

External Brain을 통해 노출되는 도구:

| 도구 | 설명 |
|------|------|
| `c4_knowledge_record` | AI가 자발적으로 지식 저장 (도구 설명에 5가지 조건 트리거) |
| `c4_knowledge_search` | 벡터 + FTS + ilike 3단계 폴백 검색 |
| `c4_session_summary` | 대화 종료 시 완전한 세션 요약 캡처 |
| `c4_status` | 현재 프로젝트 상태 읽기 |

---

## Python 사이드카 (c4/)

Go MCP 서버가 10개 도구를 JSON-RPC/TCP를 통해 Python에 위임합니다 (지연 시작).

```
Go MCP 서버 -- JSON-RPC/TCP --> Python 사이드카 (10개 도구)
                                    +-> LSP (7개): find_symbol, get_overview, replace_body,
                                    |          insert_before/after, rename, find_refs
                                    |          (Python/JS/TS 전용 — Go/Rust: c4_search_for_pattern 사용)
                                    +-> 문서 (2개): parse_document, extract_text
                                    +-> 온보딩 (1개): c4_onboard
```

- **지연 시작**: 사이드카는 첫 번째 프록시 도구 호출 시에만 시작
- **정상 폴백**: Python/uv를 사용할 수 없으면 LSP/문서 도구가 비활성화됨 (충돌 아님)

---

## 상태 머신

```
INIT -> DISCOVERY -> DESIGN -> PLAN -> EXECUTE <-> CHECKPOINT -> REFINE -> POLISH -> COMPLETE
                                          |
                                          +-> HALTED (재개 가능)
```

| 상태 | 의미 |
|------|------|
| INIT | 프로젝트 생성됨, 아직 태스크 없음 |
| DISCOVERY | 요구사항 수집 중 (c4-plan 1단계) |
| DESIGN | 아키텍처 결정 중 (c4-plan 2단계) |
| PLAN | 태스크 생성됨, 실행 준비 완료 |
| EXECUTE | Worker 활성, 태스크 처리 중 |
| CHECKPOINT | 단계 게이트 도달, 리뷰 진행 중 |
| HALTED | 실행 일시 중지, `/c4-run`으로 재개 가능 |
| COMPLETE | 모든 태스크 완료, `/c4-finish` 준비 |

---

## 보안: 권한 훅

모든 도구 사용과 셸 실행에 대한 2층 게이트:

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
  mode: hook        # "hook" (정규식만) 또는 "model" (Haiku API)
  auto_approve: true
  allow_patterns: []
  block_patterns: []
```

---

## Supabase 스키마 (주요 테이블)

| 테이블 | 목적 |
|-------|------|
| `c4_tasks` | 태스크 큐 (상태, 할당, 커밋 SHA) |
| `c4_documents` | 지식 기록 (내용, 임베딩, FTS) |
| `c4_projects` | 프로젝트 레지스트리 (소유자, 설정) |
| `hub_jobs` | 분산 작업 큐 (스펙, 상태, 리스) |
| `hub_workers` | 등록된 Worker (능력, affinity) |
| `hub_dags` | DAG 파이프라인 정의 |
| `hub_cron_schedules` | Cron 작업 정의 |
| `c4_drive_files` | Drive 파일 메타데이터 (해시, URL, 버전) |
| `c4_datasets` | 내용 주소 지정 버전 관리가 있는 데이터셋 레지스트리 |
| `c1_messages` | 세션 간 및 메시징 채널 메시지 |
| `notification_channels` | Telegram/Dooray 알림 설정 |

52개 마이그레이션, 모든 사용자 대면 테이블에 RLS 정책, 임베딩을 위한 pgvector 확장.

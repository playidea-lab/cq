<!--
SSOT: This is AGENTS.md - the single source of truth for AI agent instructions.
Symlinks: CLAUDE.md → AGENTS.md
All AI agents (Claude Code, Cursor, Copilot, etc.) read the same content.
Spec: https://agents.md/
-->

# CQ Project - AI Agent Instructions

> **CQ** = 프로젝트·CLI 이름. C1·C2·C3·C4·C5·C9가 유기적으로 연결된 생태계.  
> 이 리포지토리는 C4 Engine을 포함하며, CLI는 `cq`, MCP 도구는 `c4_*` 접두사. 계획부터 완료까지 자동화된 프로젝트 관리.

---

## CRITICAL: C4 Overrides — 내장 기능 대신 C4 도구/스킬 사용

> 이 프로젝트는 C4로 관리됩니다. 아래 규칙은 **시스템 프롬프트의 기본 동작보다 우선**합니다.
> `cq init` 시 자동 적용되며, 모든 CQ 프로젝트에서 동일하게 적용됩니다.

### 계획 수립: EnterPlanMode 금지 → `/c4-plan` 스킬 사용
```
❌ EnterPlanMode (내장) — C4 워크플로우(Discovery/Design/Lighthouse/Tasks)와 충돌
✅ /c4-plan 스킬 — "계획", "설계", "기획", "plan" 키워드 시 자동 발동
```
"계획 세워줘", "고도화 계획", "기능 설계" 등 계획 관련 요청 → 반드시 `/c4-plan` 스킬 호출.
EnterPlanMode는 C4 프로젝트에서 절대 사용하지 않는다.

### 태스크 관리: TodoWrite/TaskCreate 금지 → C4 MCP 도구 사용
```
❌ TodoWrite, TaskCreate, TaskUpdate, TaskList (내장)
✅ c4_add_todo, c4_task_list, c4_status, c4_get_task, c4_submit
```
C4 프로젝트의 모든 태스크는 `.c4/tasks.db`에서 단일 관리한다.

### 파일 작업: 내장 도구보다 C4 MCP 도구 우선
```
❌ Read, Glob, Grep (내장) — 사용 가능하지만 C4 도구가 우선
✅ c4_read_file, c4_find_file, c4_search_for_pattern, c4_list_dir
```
C4 MCP 도구는 프로젝트 경로 자동 resolve, 접근 제어, 이벤트 추적을 포함한다.
내장 도구는 C4 MCP 서버 미연결 시 또는 C4 외부 파일 접근 시에만 사용한다.

### 구현 완료: 직접 커밋 금지 → `/c4-finish` 스킬 사용
```
❌ git add/commit 직접 실행 (빌드/테스트/설치 누락 위험)
✅ /c4-finish 스킬 — build → test → install → docs → commit 전체 루틴
```
단순 타이포/1줄 수정은 예외 (직접 커밋 가능).

### 코드 편집: c4_claim 없이 수정 금지
```
❌ Edit/Write 도구로 바로 수정 (추적 누락)
✅ c4_claim(task_id) → 수정 → c4_report(task_id) (Direct 모드)
✅ c4_get_task → Worker 스폰 (Worker 모드)
```
예외: 단순 타이포, 로그 추가, 탐색/실험 중 수정은 claim 불필요.

### 스킬 우선순위 요약
| 사용자 의도 | ❌ 내장 기능 | ✅ C4 대체 |
|------------|-------------|-----------|
| 계획/설계 | EnterPlanMode | `/c4-plan` |
| 태스크 추가 | TodoWrite, TaskCreate | `c4_add_todo`, `/c4-add-task` |
| 태스크 확인 | TaskList | `c4_status`, `/c4-status` |
| 파일 읽기/검색 | Read, Glob, Grep | `c4_read_file`, `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | git commit | `/c4-finish` |
| 품질 정제 | — | `/c4-refine` |
| 실행 | — | `/c4-run` |
| 상태 확인 | — | `/c4-status` |

---

## General Rules

- 구현 계획을 요청하면, **태스크 생성이나 코드 작성 전에 반드시 접근 방식을 먼저 논의**한다. 바로 구현에 뛰어들지 않는다.
- 기존 결과/커밋을 보거나 검토하라는 요청이면, **조회만** 한다 — 실험을 재실행하거나 재구현하지 않는다. 기존 출력을 볼 것인지 새로 생성할 것인지 불명확하면 확인한다.
- 복잡한 작업 시작 전, **3-4줄 접근 계획을 먼저 보여주고** 실행한다. 접근 방식이 불확실하면 실행 전에 확인한다.

## Project Context

Primary languages: **Go, Python, TypeScript, Rust**. 변경 시 각 언어의 기존 패턴을 따른다. YAML과 Markdown은 설정·문서용.

---

## Project Overview

### C 시리즈 생태계 (CQ)
C1·C2·C3·C4·C5·C9가 유기적으로 연결된 모양새.
```
C0 Drive    — 클라우드 파일 스토리지 (Supabase Storage)
C1 Messenger — Tauri 2.x 통합 대시보드 메신저 (4-탭 뷰)
C2 Docs     — 문서 라이프사이클 (파싱/워크스페이스/프로필)
C3 EventBus — gRPC 이벤트 버스 (UDS + WebSocket + DLQ)
C4 Engine   — MCP 오케스트레이션 엔진 (이 리포)
C5 Hub      — 분산 작업 큐 서버 (Worker Pull 모델, Lease 기반)
C9 Knowledge — 지식 관리 (FTS5 + pgvector + Embedding + Usage + Ingestion)
```

### 코드베이스 규모
| 언어 | 소스 | 테스트 | 합계 |
|------|------|--------|------|
| Go (`c4-core/`) | ~40.2K LOC | ~32.7K LOC | ~72.9K |
| Go (`c5/`) | ~5.6K LOC | ~3.5K LOC | ~9.1K |
| Python (`c4/`) | ~24.4K LOC | ~11.6K LOC | ~36.0K |
| Rust (`c1/src-tauri/`) | ~9.5K LOC | (내장) | ~9.5K |
| TypeScript (`c1/src/`) | ~6.6K LOC | | ~6.6K |
| SQL (`infra/`) | ~1.1K LOC | | ~1.1K |
| **합계** | ~87.4K | ~47.8K | **~135.2K LOC** |

### 테스트 현황
| 언어 | 테스트 수 | 패키지/모듈 |
|------|----------|------------|
| Go | **1,321** | 23 packages (all pass) — c4-core 1,201 + c5 120 |
| Python | **750** | tests/unit/ |
| Rust | **85** | src-tauri |
| **합계** | **~2,156** | |

### Monorepo 구조
```
c4/
├── c4-core/          # Go MCP 서버 (Primary)
├── c4/               # Python Sidecar (LSP, Doc parsing)
├── c5/               # Go 분산 작업 큐 서버 (Hub)
├── c1/               # Tauri 2.x 데스크톱 앱
├── .claude/skills/   # Claude Code Skills (20개, 자동 발동 워크플로우)
├── infra/supabase/   # PostgreSQL 마이그레이션 (18개)
├── docs/             # ROADMAP, guides
├── scripts/          # 유틸리티 스크립트
├── tests/            # Python 테스트
├── .mcp.json         # MCP 서버 설정 → ~/.local/bin/cq
├── CLAUDE.md → AGENTS.md  # AI 에이전트 지침 (SSOT)
└── pyproject.toml    # Python 프로젝트 설정
```

---

## Documentation SSOT Rules (CRITICAL)

- **DO NOT CREATE**: `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md`, `*_SUMMARY.md`
- **Task tracking**: `.c4/tasks.db` via `c4_add_todo` (NOT TodoWrite)
- **Roadmap**: `docs/ROADMAP.md` (human-managed)

---

## C4 사용 규칙

### 두 가지 실행 모드

| 모드 | 언제 | 도구 |
|------|------|------|
| **Worker** | 독립적, 병렬 가능한 태스크 | `c4_get_task` → `c4_submit` |
| **Direct** | 파일 간 의존성 높은 작업 | `c4_claim` → `c4_report` |

### Quick Start
```
/c4-plan "기능 설명"    # 계획 수립 + 태스크 생성
/c4-run                 # Worker 스폰 → 자동 실행
/c4-status              # 진행 상황 확인
```

### Direct 모드
```
c4_add_todo(mode="direct", review_required=False)
→ c4_claim(task_id)     # 시작 선언
→ 직접 작업
→ c4_report(task_id, summary, files_changed)  # 완료 보고
```

### C4 Worker 규칙
- C4 워크플로우에서 구현 태스크는 **항상 Worker를 사용**하고, 직접 구현하지 않는다.
- Worker 출력을 보고하기 전에 **실제 코드 변경(commit_sha)을 확인**한다. diff가 없으면 완료가 아니다.

### Edit OK (C4 추적 불필요)
- 단순 타이포, 로그/디버그 추가, 1줄 수정, 탐색/실험 중

---

## Agent Behavioral Rules

### Stop & Ask (모호하면 멈춰라)
- 요구사항에 **여러 해석**이 가능하면 → 가정을 나열하고 사용자에게 확인
- 더 단순한 방법이 있으면 → 제안하고 push back
- 혼란스러우면 → **멈추고**, 무엇이 불명확한지 명시한 뒤 질문
- 절대로 **모호한 요구를 추측으로 해석하고 진행하지 않는다**

### Surgical Changes (변경 범위 제한)
기존 코드 수정 시:
- 요청과 **직접 관련된 줄만** 수정한다
- 인접 코드, 주석, 포맷을 "개선"하지 않는다
- 깨지지 않은 코드를 리팩토링하지 않는다
- 기존 스타일과 다르더라도 **기존 스타일을 따른다**
- 무관한 dead code 발견 시 → 삭제 대신 **언급만** 한다
- 내 변경이 만든 orphan(미사용 import/변수/함수)만 정리한다

**기준**: 변경된 모든 줄이 사용자 요청에 직접 추적 가능해야 한다.

### No Overengineering (과잉 엔지니어링 금지)
- 요청되지 않은 기능, 설정 옵션, "유연성" 추가 금지
- 한 번만 쓰이는 코드에 추상화/헬퍼 함수 만들지 않는다
- 발생할 수 없는 시나리오에 대한 에러 처리 금지
- 비슷한 코드 3줄이 조기 추상화보다 낫다
- docstring, 주석, type annotation은 **내가 수정한 코드에만** 추가

**기준**: 시니어 엔지니어가 "과하다"고 할 만하면 → 단순화.

### Efficiency Rules (컨텍스트 절약)
- 큰 파일을 읽기 전에 **항상 line range를 제한**한다. 일부만 필요하면 전체를 읽지 않는다.
- 컨텍스트 사용을 최소화한다.

### Debugging (디버깅 원칙)
- MCP 서버나 도구 연결 문제 디버깅 시, 우회책 대신 **근본 원인(모듈 경로, config 오류 등)을 수정**한다.
- 도구/서버 연결 실패 시 설정과 모듈 경로부터 확인한다.

---

## CRITICAL: C4 Operation Pre-conditions

### 복잡한 도구 사용 전 Lighthouse 조회
Hub DAG, Hub Job, Drive, Edge/Deploy, Research 등 워크플로우가 있는 도구 사용 전:
```
c4_lighthouse get <tool_name>
```
→ 사용 패턴, 워크플로우, 예시, 주의사항, 출력 형식 확인

### c4_submit 전 필수 체크
1. `c4_status`로 태스크 상태 확인
2. 태스크가 `in_progress` 상태인지 검증
3. `pending` 상태면 → `c4_get_task`로 먼저 할당
4. 절대로 pending 상태의 태스크를 submit하지 않는다
5. 직접 DB 업데이트 금지 — MCP API만 사용

### 검증 후 진행
- Python → `uv run python -m py_compile <file>` 또는 관련 테스트
- Go → `cd c4-core && go build ./... && go vet ./...`
- Config → 형식 검증
- 검증 실패 시 → 다음 단계 진행 금지

### Bulk Operation (10개+ 파일)
1. 대상 파일 목록 나열 → 사용자 확인
2. 수정 후 전체 검증 (lint + test)

### Session Handoff
장시간 디버깅 종료 시 `c4_knowledge_record`(insight)로 기록:
- 발견한 문제 + 수정 사항
- 미해결 이슈
- 다음 세션 시작 지점

---

## MCP 도구 빠른 참조 (108개 base, Hub 활성화 시 134개)

> **도구 상세 사용법**: `c4_lighthouse get <tool_name>`으로 워크플로우, 예시, 관련 도구, 주의사항 조회

```
상태/설정(6): c4_status, c4_start, c4_clear, c4_config_get, c4_config_set, c4_health
태스크(7):  c4_add_todo, c4_get_task, c4_submit, c4_mark_blocked,
            c4_claim, c4_report, c4_task_list
리뷰(3):    c4_checkpoint, c4_request_changes, c4_ensure_supervisor
검증(1):    c4_run_validation
파일(6):    c4_find_file, c4_search_for_pattern, c4_read_file,
            c4_replace_content, c4_create_text_file, c4_list_dir
Git(4):     c4_worktree_status, c4_worktree_cleanup,
            c4_analyze_history, c4_search_commits
Discovery(8): c4_save_spec, c4_get_spec, c4_list_specs,
            c4_save_design, c4_get_design, c4_list_designs,
            c4_discovery_complete, c4_design_complete
Artifact(3): c4_artifact_save, c4_artifact_list, c4_artifact_get
LSP(7):     c4_find_symbol, c4_get_symbols_overview,  ← Python/JS/TS + Go + Dart native
            c4_replace_symbol_body, c4_insert_before_symbol,
            c4_insert_after_symbol, c4_rename_symbol,
            c4_find_referencing_symbols
            ※ Go/Dart는 native 지원, Rust → c4_search_for_pattern 사용
지식(14):   c4_knowledge_search, c4_knowledge_record, c4_knowledge_get,
            c4_knowledge_pull, c4_knowledge_delete, c4_knowledge_publish,
            c4_knowledge_discover, c4_knowledge_ingest, c4_knowledge_distill,
            c4_knowledge_stats, c4_knowledge_reindex,
            c4_experiment_record, c4_experiment_search, c4_pattern_suggest
Research(5): c4_research_start, c4_research_next, c4_research_record,
            c4_research_approve, c4_research_status
GPU(2):     c4_gpu_status, c4_job_submit
Soul(3):    c4_soul_get, c4_soul_set, c4_soul_resolve
팀(3):      c4_whoami, c4_persona_stats, c4_persona_evolve
Twin(1):    c4_reflect
온보딩(1):  c4_onboard
Lighthouse(1): c4_lighthouse (register/list/get/promote/update/remove/export_llms_txt)
LLM(3):    c4_llm_call, c4_llm_providers, c4_llm_costs
CDP(2):    c4_cdp_run, c4_cdp_list
WebMCP(4): c4_webmcp_discover, c4_webmcp_call, c4_webmcp_context, c4_web_fetch
C2(8):     c4_parse_document, c4_extract_text,
            c4_workspace_create, c4_workspace_load, c4_workspace_save,
            c4_persona_learn, c4_profile_load, c4_profile_save
Drive(6):  c4_drive_upload, c4_drive_download, c4_drive_list, c4_drive_delete,
            c4_drive_info, c4_drive_mkdir
EventBus(6): c4_event_list, c4_event_publish,
            c4_rule_add, c4_rule_list, c4_rule_remove, c4_rule_toggle
C1(5):     c1_search, c1_check_mentions, c1_get_briefing,
            c1_send_message, c1_update_presence
--- Hub (hub.enabled=true 시 추가 등록, +29) ---
Worker(3): c4_worker_standby, c4_worker_complete, c4_worker_shutdown
Hub-Job(10): c4_hub_submit, c4_hub_status, c4_hub_list,
            c4_hub_cancel, c4_hub_metrics, c4_hub_log_metrics,
            c4_hub_watch, c4_hub_summary, c4_hub_retry, c4_hub_estimate
Hub-Infra(4): c4_hub_workers, c4_hub_stats, c4_hub_upload, c4_hub_download
Hub-DAG(7): c4_hub_dag_create, c4_hub_dag_add_node, c4_hub_dag_add_dep,
            c4_hub_dag_execute, c4_hub_dag_status, c4_hub_dag_list,
            c4_hub_dag_from_yaml
Hub-Edge(5): c4_hub_edge_register, c4_hub_edge_list,
            c4_hub_deploy_rule, c4_hub_deploy, c4_hub_deploy_status
```

### 워크플로우
```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → REFINE → COMPLETE
```

### Task ID 체계
```
T-001-0:   구현 태스크 (버전 0)
R-001-0:   리뷰 태스크
RF-001-0:  리파인 태스크 (반복 리뷰-수정 루프, domain=refine)
RPR-001-0: 수정 재작업
CP-001:    체크포인트
```

- **REQUEST_CHANGES 사유**: 리뷰 태스크(R-)의 거절 사유는 `review_decision_evidence` 필드에 저장됨. `commit_sha`는 실제 커밋 SHA 전용. 조회 시 GetTask/c4_get_task 응답의 `review_decision_evidence` 사용.

---

## Go Core (c4-core/) — Primary MCP Server

> Go 기반 MCP 서버. ~37.8K LOC(src) + ~30.9K LOC(test). 1,085개 테스트, 25 패키지.

### 아키텍처
```
Claude Code → Go MCP Server (stdio, 108 base + 26 Hub = 134 tools)
                ├→ Go native (28): 상태/설정, 태스크, 파일, git, validation, config, health, eventbus rules
                ├→ Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                ├→ Soul/Persona/Twin (7): soul CRUD, persona evolve, whoami, reflect
                ├→ LLM Gateway (3): llm_call, llm_providers, llm_costs
                ├→ CDP Runner + WebMCP (5): cdp_run, cdp_list, webmcp_discover, webmcp_call, webmcp_context
                ├→ WebContent (1): web_fetch (content negotiation, SSRF, HTML→MD) — c2/webcontent
                ├→ C1 Messenger (5): search, mentions, briefing, send_message, update_presence + ContextKeeper
                ├→ Drive (6): upload, download, list, delete, info, mkdir
                ├→ Go Native — Tier 1 (13): Research (5) + C2 (6) + GPU (2)
                ├→ Go Native — Tier 2 (13): Knowledge (Store+FTS5+Vector+Embedding+Usage+Ingest+Sync+Publish)
                ├→ Hub Client (26, 조건부): job, worker, DAG, edge, deploy, artifact
                ├→ Worker Standby (3, Hub 조건부): standby, complete, shutdown
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

---

## Python Sidecar (c4/)

> Python 기반 보조 서버. Go MCP 서버에서 JSON-RPC/TCP로 호출. ~24K LOC.

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
| Tier 1 | 13 → Go | Research (5) + C2 (6) + GPU (2) | `research/`, `c2/`, `daemon/` |
| Tier 2 | 12 → Go | Knowledge (12) | `knowledge/` |
| 남은 Proxy | 10 | LSP (7) + C2 Doc (2) + Onboard (1) | — |

### 특성
- **Lazy Start**: 첫 proxy 호출 시에만 sidecar 시작
- **Health Check**: Exponential backoff로 연결 확인
- **Python 미설치 시**: Graceful fallback (LSP/Doc 도구만 비활성)

---

## C1 Messenger (c1/)

> Tauri 2.x 통합 대시보드 메신저. ~8.5K LOC(Rust) + ~6.5K LOC(TypeScript). 76개 테스트.

### 아키텍처
- **Rust 백엔드**: `src-tauri/src/{commands,models,analytics,cloud,scanner,messaging,eventbus,lib}.rs`
- **Multi-Provider**: `src-tauri/src/providers/` — Claude Code, Codex CLI, Cursor, Gemini CLI
- **React 프론트엔드**: `src/components/`, `src/hooks/`, `src/styles/`
- **CSS**: BEM 패턴 + `styles/tokens.css` 디자인 토큰
- **통합 멤버 모델**: `c1_members` — 사용자/에이전트/시스템을 동등한 멤버로 관리

### 4개 뷰
| 뷰 | 데이터 소스 | 핵심 기능 |
|-----|-------------|-----------|
| Messenger | Supabase Realtime + c1_members | 실시간 메시징, 멤버 프레즌스, 시스템 채널 |
| Documents | 로컬 파일시스템 | 문서 파싱, C2 연동 |
| Settings | `~/.claude/`, `.c4/` | 설정 파일 뷰어/편집기 |
| Team | Supabase | 팀 프로젝트, 대시보드 |

### 빌드/실행
```bash
cd c1 && pnpm install
cd src-tauri && cargo check && cargo test
pnpm build            # 프론트엔드 빌드
cargo tauri dev       # 개발 서버
```

---

## Infra (infra/supabase/)

> PostgreSQL 마이그레이션 18개. Supabase 기반 클라우드 레이어.

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

### 이벤트 종류 (16종)
```
task.completed, task.updated, task.blocked, task.created
checkpoint.approved, checkpoint.rejected
review.changes_requested
validation.passed, validation.failed
knowledge.recorded, knowledge.searched
```

Add at the top of CLAUDE.md under a ## General Rules section\n\nWhen I ask you to implement a plan, ALWAYS discuss the approach with me first before creating tasks or writing code. Do not jump straight into implementation.
Add under a ## C4 Workflow section\n\nWhen working in C4 workflow: always use C4 workers for implementation tasks, never implement directly. Verify worker output by checking for actual code changes (commit_sha) before reporting tasks as done.
Add under a ## Code Reading section or ## Efficiency Rules section\n\nBefore reading large files, always use line range constraints. Do not read entire files when only a portion is needed. Minimize context usage.
Add under ## General Rules section\n\nWhen I ask to view, inspect, or review existing results/commits, retrieve them — do NOT re-run experiments or re-implement things. Ask for clarification if unsure whether I want to view existing output or generate new output.
Add under a ## Debugging section\n\nWhen debugging MCP server or tool connection issues, fix the root cause (e.g., wrong module path, config error) instead of trying workarounds. If a tool/server fails to connect, check the configuration and module paths first.
Add under a ## Project Context section near the top of CLAUDE.md\n\nPrimary languages for this workspace: Go, Python, TypeScript, Rust. When making changes, follow existing patterns in each language. YAML and Markdown are used for configuration and documentation.

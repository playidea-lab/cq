<!--
SSOT: This is AGENTS.md - the single source of truth for AI agent instructions.
Symlinks: CLAUDE.md → AGENTS.md
All AI agents (Claude Code, Cursor, Copilot, etc.) read the same content.
Spec: https://agents.md/
-->

# C4 Project - AI Agent Instructions

> C4: AI 오케스트레이션 시스템 - 계획부터 완료까지 자동화된 프로젝트 관리

---

## Project Overview

### C 시리즈 생태계
```
C0 Drive    — 클라우드 파일 스토리지 (Supabase Storage)
C1 Desktop  — Tauri 2.x 프로젝트 탐색기 (6-탭 뷰)
C2 Docs     — 문서 라이프사이클 (파싱/워크스페이스/프로필)
C3 EventBus — gRPC 이벤트 버스 (UDS + WebSocket + DLQ)
C4 Engine   — MCP 오케스트레이션 엔진 (이 프로젝트)
C5 Hub      — 분산 작업 큐 서버 (Worker Pull 모델, Lease 기반)
C9 Knowledge — 지식 관리 (FTS5 + Vector + Cloud Sync)
```

### 코드베이스 규모
| 언어 | 소스 | 테스트 | 합계 |
|------|------|--------|------|
| Go (`c4-core/`) | ~32K LOC | ~27K LOC | ~59K |
| Go (`c5/`) | ~2.4K LOC | ~1.3K LOC | ~3.7K |
| Python (`c4/`) | ~24K LOC | (tests/ 내 포함) | ~24K |
| Rust (`c1/src-tauri/`) | ~8.5K LOC | (내장) | ~8.5K |
| TypeScript (`c1/src/`) | ~6.5K LOC | | ~6.5K |
| SQL (`infra/`) | ~0.8K LOC | | ~0.8K |
| **합계** | | | **~102.5K LOC** |

### 테스트 현황
| 언어 | 테스트 수 | 패키지/모듈 |
|------|----------|------------|
| Go | **945** | 20 packages (all pass) — c4-core 895 + c5 50 |
| Python | **751** | tests/unit/ |
| Rust | **73** | src-tauri |
| **합계** | **~1,769** | |

### Monorepo 구조
```
c4/
├── c4-core/          # Go MCP 서버 (Primary)
├── c4/               # Python Sidecar (LSP, Doc parsing)
├── c5/               # Go 분산 작업 큐 서버 (Hub)
├── c1/               # Tauri 2.x 데스크톱 앱
├── infra/supabase/   # PostgreSQL 마이그레이션 (14개)
├── docs/             # ROADMAP, guides
├── scripts/          # 유틸리티 스크립트
├── tests/            # Python 테스트
├── .mcp.json         # MCP 서버 설정 → ~/.local/bin/c4
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

---

## CRITICAL: C4 Operation Pre-conditions

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

## MCP 도구 빠른 참조 (109개 등록, Hub 활성화 시 최대 ~135개)

```
상태(3):    c4_status, c4_start, c4_clear
태스크(6):  c4_add_todo, c4_get_task, c4_submit, c4_mark_blocked,
            c4_claim, c4_report
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
LSP(7):     c4_find_symbol, c4_get_symbols_overview,
            c4_replace_symbol_body, c4_insert_before_symbol,
            c4_insert_after_symbol, c4_rename_symbol,
            c4_find_referencing_symbols
지식(7):    c4_knowledge_search, c4_knowledge_record, c4_knowledge_get,
            c4_knowledge_pull,
            c4_experiment_record, c4_experiment_search, c4_pattern_suggest
Research(5): c4_research_start, c4_research_next, c4_research_record,
            c4_research_approve, c4_research_status
GPU(2):     c4_gpu_status, c4_job_submit
Soul(3):    c4_soul_get, c4_soul_set, c4_soul_resolve
팀(3):      c4_whoami, c4_persona_stats, c4_persona_evolve
Twin(1):    c4_reflect
온보딩(1):  c4_onboard
Lighthouse(1): c4_lighthouse (register/list/get/promote/update/remove)
LLM(3):    c4_llm_call, c4_llm_providers, c4_llm_costs
CDP(2):    c4_cdp_run, c4_cdp_list
C2(8):     c4_parse_document, c4_extract_text,
            c4_workspace_create, c4_workspace_load, c4_workspace_save,
            c4_persona_learn, c4_profile_load, c4_profile_save
Drive(6):  c4_drive_upload, c4_drive_download, c4_drive_list, c4_drive_delete,
            c4_drive_info, c4_drive_mkdir
C1(3):     c1_search, c1_check_mentions, c1_get_briefing
--- Hub (hub.enabled=true 시 추가 등록) ---
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
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → COMPLETE
```

### Task ID 체계
```
T-001-0: 구현 태스크 (버전 0)
R-001-0: 리뷰 태스크
CP-001:  체크포인트
```

---

## Go Core (c4-core/) — Primary MCP Server

> Go 기반 MCP 서버. ~32K LOC(src) + ~27K LOC(test). 895개 테스트, 19 패키지.

### 아키텍처
```
Claude Code → Go MCP Server (stdio, 109 tools)
                ├→ Go native (22): 상태, 태스크, 파일, git, validation
                ├→ Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                ├→ Soul/Persona/Twin (7): soul CRUD, persona evolve, whoami, reflect
                ├→ LLM Gateway (3): llm_call, llm_providers, llm_costs
                ├→ CDP Runner (2): cdp_run, cdp_list
                ├→ C1 Context Hub (3): search, mentions, briefing + ContextKeeper
                ├→ Drive (6): upload, download, list, delete, info, mkdir
                ├→ Go Native — Tier 1 (13): Research (5) + C2 (6) + GPU (2)
                ├→ Go Native — Tier 2 (7): Knowledge (Store+FTS5+Vector+Sync)
                ├→ Hub Client (26, 조건부): job, worker, DAG, edge, deploy, artifact
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
│   ├── knowledge/    # C9 Knowledge (Store+FTS5+VectorStore+Searcher+Sync)
│   ├── research/     # Research iteration store (paper+experiment loop)
│   ├── c2/           # C2 Workspace/Profile/Persona
│   ├── drive/        # C0 Drive client (Supabase Storage)
│   ├── llm/          # LLM Gateway (Anthropic, OpenAI, Gemini, Ollama)
│   └── cdp/          # Chrome DevTools Protocol runner
└── test/benchmark/   # 벤치마크
```

### 빌드/테스트/설치

```bash
# 빌드 + 테스트
cd c4-core && go build ./... && go test ./...

# 사용자 설치 (CRITICAL — .mcp.json이 이 경로를 참조)
cd c4-core && go build -o ~/.local/bin/c4 ./cmd/c4/

# 개발용 바이너리 (CI/로컬 테스트)
cd c4-core && go build -o bin/c4 ./cmd/c4/
```

### 바이너리 관리 규칙 (CRITICAL)

| 경로 | 용도 | 갱신 시점 |
|------|------|----------|
| `~/.local/bin/c4` | **운영 바이너리** — `.mcp.json`이 참조, Claude Code가 실행 | 코드 변경 후 반드시 재빌드 |
| `c4-core/bin/c4` | 개발/테스트용 | `go build ./...` 시 자동 |

**필수 규칙**:
1. **코드 수정 후 `~/.local/bin/c4` 재빌드 필수** — 안 하면 구 바이너리가 계속 실행됨
2. **`cp` 복사 금지** — macOS ARM64에서 코드 서명 무효화. 반드시 `go build -o` 사용
3. **재빌드 후 세션 재시작** — Claude Code가 세션 시작 시 MCP 서버를 로드하므로
4. **`c4-finish` 스킬에서 자동 설치** — 릴리스 루틴에 `go build -o ~/.local/bin/c4` 포함 권장

---

## Python Sidecar (c4/)

> Python 기반 보조 서버. Go MCP 서버에서 JSON-RPC/TCP로 호출. ~24K LOC.

### 역할 (Tier 1+2 마이그레이션 후 축소)
```
Go MCP Server ──JSON-RPC/TCP──→ Python Sidecar (10 tools)
                                  ├→ LSP (7): find_symbol, get_overview, replace_body,
                                  │          insert_before/after, rename, find_refs
                                  ├→ C2 Doc (2): parse_document, extract_text
                                  └→ Onboard (1): c4_onboard
```

### 마이그레이션 이력
| Tier | 도구 수 | 대상 | Go 패키지 |
|------|---------|------|-----------|
| Tier 1 | 13 → Go | Research (5) + C2 (6) + GPU (2) | `research/`, `c2/`, `daemon/` |
| Tier 2 | 7 → Go | Knowledge (7) | `knowledge/` |
| 남은 Proxy | 10 | LSP (7) + C2 Doc (2) + Onboard (1) | — |

### 특성
- **Lazy Start**: 첫 proxy 호출 시에만 sidecar 시작
- **Health Check**: Exponential backoff로 연결 확인
- **Python 미설치 시**: Graceful fallback (LSP/Doc 도구만 비활성)

---

## C1 Desktop (c1/)

> Tauri 2.x 데스크톱 앱. ~8.5K LOC(Rust) + ~6.5K LOC(TypeScript). 73개 테스트.

### 아키텍처
- **Rust 백엔드**: `src-tauri/src/{commands,models,analytics,cloud,scanner,messaging,eventbus,lib}.rs`
- **Multi-Provider**: `src-tauri/src/providers/` — Claude Code, Codex CLI, Cursor, Gemini CLI
- **React 프론트엔드**: `src/components/`, `src/hooks/`, `src/styles/`
- **CSS**: BEM 패턴 + `styles/tokens.css` 디자인 토큰

### 6개 뷰
| 뷰 | 데이터 소스 | 핵심 기능 |
|-----|-------------|-----------|
| Sessions | 다중 프로바이더 | 세션 분석, 타임라인, 통계 |
| Dashboard | `.c4/c4.db` | 프로젝트 상태, 태스크, 검증 결과 |
| Config | `~/.claude/`, `.c4/` | 설정 파일 뷰어/편집기 |
| Documents | 로컬 파일시스템 | 문서 파싱, C2 연동 |
| Channels | Supabase Realtime | 실시간 메시징, 검색, 브리핑 |
| Events | EventBus WebSocket | 실시간 이벤트 모니터링 |

### 빌드/실행
```bash
cd c1 && pnpm install
cd src-tauri && cargo check && cargo test
pnpm build            # 프론트엔드 빌드
cargo tauri dev       # 개발 서버
```

---

## Infra (infra/supabase/)

> PostgreSQL 마이그레이션 14개. Supabase 기반 클라우드 레이어.

### 주요 테이블
- `c4_tasks`, `c4_documents`, `c4_projects` — C4 핵심 데이터
- `c1_channels`, `c1_messages`, `c1_participants`, `c1_channel_summaries` — C1 메시징
- RLS 정책 (migration 00014: 보안 픽스)

---

## C3 EventBus (internal/eventbus/)

> gRPC UDS daemon + WebSocket bridge + DLQ. 87+ 테스트.

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

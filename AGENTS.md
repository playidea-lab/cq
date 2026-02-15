<!--
SSOT: This is AGENTS.md - the single source of truth for AI agent instructions.
Symlinks: CLAUDE.md → AGENTS.md
All AI agents (Claude Code, Cursor, Copilot, etc.) read the same content.
Spec: https://agents.md/
-->

# C4 Project - AI Agent Instructions

> C4: AI 오케스트레이션 시스템 - 계획부터 완료까지 자동화된 프로젝트 관리

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

## MCP 도구 빠른 참조 (112개: Base 86 + Hub 26, Tier 1 Go Native 13개 포함)

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

> `c4-core/` — Go 기반 MCP 서버 (Primary). 112개 도구 (Base 86 + Hub 26). Python sidecar로 LSP/C2 Doc 기능 위임 (10 proxy tools).

### 아키텍처
```
Claude Code → Go MCP Server (stdio, 112 tools)
                ├→ Go native (22): 상태, 태스크, 파일, git, validation
                ├→ Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                ├→ Soul/Persona/Twin (7): soul CRUD, persona evolve, whoami, reflect
                ├→ LLM Gateway (3): llm_call, llm_providers, llm_costs
                ├→ CDP Runner (2): cdp_run, cdp_list
                ├→ C1 Context Hub (3): search, mentions, briefing + ContextKeeper
                ├→ Drive (6): upload, download, list, delete, info, mkdir
                ├→ Hub Client (26): job, worker, metrics, artifact, DAG, edge, deploy
                ├→ Go Native (Tier 1) (13): Research (5) + C2 (6) + GPU (2)
                ├→ Go Native (Tier 2) (7): Knowledge (7) — Store+FTS5+Vector+Sync
                └→ JSON-RPC proxy (10) → Python Sidecar (LSP 7 + C2 Doc 2 + Onboard 1)
                                            ├→ LSP (multilspy, Jedi, tree-sitter)
                                            └→ C2 Doc (pymupdf, python-docx)
```

### 패키지 구조
- `cmd/c4/` — CLI (cobra), MCP server (Registry-based)
- `internal/mcp/` — Registry + handlers
- `internal/mcp/handlers/` — sqlite_store, files, git, discovery, artifacts, proxy, validation, llm, hub, c2, c1, drive
- `internal/hub/` — PiQ Hub REST+WS client (job, worker, DAG, edge, deploy, artifact, stream)
- `internal/bridge/` — Python sidecar 관리 (JSON-RPC/TCP)
- `internal/task/` — TaskStore (SQLite, Memory, Supabase)
- `internal/state/` — State machine
- `internal/worker/` — Worker manager
- `internal/validation/` — Validation runner

### 빌드/테스트
```bash
cd c4-core && go build ./... && go test ./...
# Binary: c4-core/bin/c4 (12MB)
```

---

## C1 (Multi-LLM Project Explorer)

> `c1/` — Tauri 2.x 데스크톱 앱. Multi-LLM 프로젝트 탐색기.

### 아키텍처
- **Rust 백엔드**: `src-tauri/src/{commands,models,analytics,cloud,scanner,messaging,lib}.rs`
- **Multi-Provider**: `src-tauri/src/providers/` — Claude Code, Codex CLI, Cursor, Gemini CLI
- **React 프론트엔드**: `src/components/`, `src/hooks/`, `src/styles/`
- **CSS**: BEM 패턴 + `styles/tokens.css` 디자인 토큰

### 5개 뷰
| 뷰 | 데이터 소스 | Rust 커맨드 |
|-----|-------------|-------------|
| Sessions | 다중 프로바이더 + Analytics | `list_providers`, `get_session_stats`, `get_provider_timeline` |
| Dashboard | `.c4/c4.db` + Timeline + Validation | `get_project_state`, `get_task_timeline`, `get_validation_results` |
| Config | `~/.claude/`, `.claude/`, `.c4/` 파일 | `list_config_files`, `read_config_file` |
| Channels | Supabase c1_* 테이블 + Realtime | `list_channels`, `send_message`, `search_messages`, `get_briefing` |
| Team | Supabase (로그인 시만 표시) | `cloud_sync_tasks`, `cloud_get_team_projects`, `cloud_get_remote_dashboard` |

### 빌드/실행
```bash
cd c1 && pnpm install
cd src-tauri && cargo check && cargo test
pnpm build            # 프론트엔드 빌드
cargo tauri dev       # 개발 서버
```

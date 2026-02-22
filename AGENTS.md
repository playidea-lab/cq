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

### 코드 분석/편집: LSP 도구 사용 원칙

**언어별 지원 범위 (CRITICAL)**
```
c4_find_symbol / c4_get_symbols_overview:
  ✅ Go (.go)          → goast native (빠름, sidecar 불필요)
  ✅ Dart/Flutter(.dart) → dartast native
  ✅ Python/JS/TS      → Jedi/multilspy sidecar
  ❌ Rust (.rs)        → c4_search_for_pattern

c4_replace_symbol_body / c4_insert_*/c4_rename_symbol:
  ✅ Python/JS/TS only
  ❌ Go/Dart/Rust      → Edit 도구 직접 사용
```

**표준 편집 체인**
```
# Python/JS/TS
1. c4_get_symbols_overview(path="파일")       # 구조 파악 — c4_read_file 전에 먼저
2. c4_find_symbol(name="Target", path="...")   # 위치 확인 (path 필수, 생략 시 timeout)
3. c4_replace_symbol_body(...)                # 편집
4. c4_find_symbol(...)                        # 결과 검증

# Go / Dart
1. c4_get_symbols_overview(path="파일")       # 구조 파악 (native, 빠름)
2. c4_find_symbol(name="Target", path="...")   # 위치 확인 (native)
3. Edit 도구                                   # 편집 (replace_symbol_body 미지원)
```

**이름 변경 (Python/JS/TS)**: `c4_rename_symbol` 우선 → 완료 후 `c4_search_for_pattern`으로 잔존 확인

**상세 가이드**: `c4_lighthouse get c4_find_symbol` 또는 Knowledge 조회 `"LSP symbol"`

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
| Go (`c4-core/`) | ~38.9K LOC | ~36.8K LOC | ~75.7K |
| Go (`c5/`) | ~6.9K LOC | ~4.8K LOC | ~11.7K |
| Python (`c4/`) | ~22.9K LOC | ~9.5K LOC | ~32.4K |
| Rust (`c1/src-tauri/`) | ~9.5K LOC | (내장) | ~9.5K |
| TS+CSS (`c1/src/`) | ~11.8K LOC | | ~11.8K |
| SQL (`infra/`) | ~1.1K LOC | | ~1.1K |
| **합계** | ~90.9K | ~50.8K | **~141.7K LOC** |

### 테스트 현황
| 언어 | 테스트 수 | 패키지/모듈 |
|------|----------|------------|
| Go | **~1,468** | 26 packages (all pass) — c4-core ~1,294 + c5 174 |
| Python | **697** | tests/unit/ |
| Rust | **92** | src-tauri |
| **합계** | **~2,257** | |

### Monorepo 구조
```
c4/
├── c4-core/          # Go MCP 서버 (Primary)
├── c4/               # Python Sidecar (LSP, Doc parsing)
├── c5/               # Go 분산 작업 큐 서버 (Hub)
├── c1/               # Tauri 2.x 데스크톱 앱
├── .claude/skills/   # Claude Code Skills (20개, 자동 발동 워크플로우)
├── infra/supabase/   # PostgreSQL 마이그레이션 (21개)
├── docs/             # ROADMAP, guides
├── scripts/          # 유틸리티 스크립트
├── tests/            # Python 테스트
├── user/             # PlayIdea-Lab/cq submodule (공개 배포 — install.sh, configs/)
├── .gitlab-ci.yml    # GitLab CI: 9-binary 크로스 컴파일 → GitHub Releases
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
/c4-polish              # 수정사항 0될 때까지 정제 (필수)
/c4-finish              # 빌드·설치·문서·커밋 마무리
/c4-status              # 진행 상황 확인
```

### Session Resume Protocol (컨텍스트 소진 후 재개 시 — 가장 먼저 실행)

**세션이 끊겨서 재개할 때, "어디까지 했나"보다 "무엇을 빠뜨렸나"를 먼저 확인한다.**

```bash
# 1. 품질 게이트 상태 확인 (DB 직접 조회 — 세션 메모리 무관)
sqlite3 .c4/c4.db \
  "SELECT gate, status, reason, completed_at FROM c4_gates ORDER BY completed_at DESC LIMIT 10;" \
  2>/dev/null || echo "⚠️ c4_gates 테이블 없음 (이전 세션에서 게이트 미기록)"
```

| 조회 결과 | 판단 |
|---------|------|
| `polish \| done` 레코드 있음 | ✅ c4-finish 진행 가능 |
| 레코드 없음 | ⛔ polish/refine 먼저 실행 필요 |
| `polish \| skipped` | ⚠️ 사유 확인 후 사용자에게 명시 |

**재개 시 선언 형식** (항상 이 형식으로 현재 상태를 명시):
```
## 재개 상태
- Task batch: T-XXX~T-YYY
- Gates: polish=[done|pending], refine=[done|pending]
- 다음 단계: [무엇부터 시작하는지]
```

### Context Compression Handoff Protocol (컨텍스트 소진 예상 시)

컨텍스트가 소진되기 전, MEMORY.md 또는 `.c4/handoff.md`에 반드시 기록:

```markdown
## [HANDOFF] 워크플로우 상태 — {날짜}
- Task batch: T-XXX~T-YYY
- Gates completed: polish=done(round 3), refine=skipped(사유)
- Gates pending: []
- finish_allowed: true
- 다음 단계: Step 6 — AGENTS.md 문서 업데이트
- 주의: [컨텍스트 소진 전 마지막으로 알고 있는 중요 상태]
```

**핵심 원칙**: 요약이 "위치"를 전달한다면, Handoff는 "게이트 상태"를 전달한다.

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

## MCP 도구 빠른 참조 (118개 base, Hub 활성화 시 144개)

> **도구 상세 사용법**: `c4_lighthouse get <tool_name>`으로 워크플로우, 예시, 관련 도구, 주의사항 조회

```
상태/설정(6): c4_status, c4_start, c4_clear, c4_config_get, c4_config_set, c4_health
Phase Lock(2): c4_phase_lock_acquire, c4_phase_lock_release
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
GPU(6):     c4_gpu_status, c4_job_submit, c4_job_list,
            c4_job_status, c4_job_cancel, c4_job_summary
Soul(3):    c4_soul_get, c4_soul_set, c4_soul_resolve
팀(3):      c4_whoami, c4_persona_stats, c4_persona_evolve
Twin(1):    c4_reflect
온보딩(1):  c4_onboard
Secrets(4): c4_secret_set, c4_secret_get, c4_secret_list, c4_secret_delete
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
--- Tiered (build tag 활성화 시 추가 등록) ---
C7 Observe(4): c4_observe_metrics, c4_observe_logs, c4_observe_config, c4_observe_health
C6 Guard(5): c4_guard_check, c4_guard_audit, c4_guard_policy_set, c4_guard_policy_list, c4_guard_role_assign
C8 Gate(6): c4_gate_webhook_register, c4_gate_webhook_list, c4_gate_webhook_test,
            c4_gate_schedule_add, c4_gate_schedule_list, c4_gate_connector_status
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
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → REFINE → POLISH → COMPLETE
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

> Go 기반 MCP 서버. ~45.0K LOC(src) + ~38.7K LOC(test). ~1,294개 테스트, 26 패키지.

### 아키텍처
```
Claude Code → Go MCP Server (stdio, 131 base + 26 Hub = 157 tools)
                ├→ Go native (28): 상태/설정, 태스크, 파일, git, validation, config, health, eventbus rules
                ├→ Go + SQLite (13): spec, design, checkpoint, artifact, lighthouse
                ├→ Soul/Persona/Twin (7): soul CRUD, persona evolve, whoami, reflect
                ├→ LLM Gateway (3): llm_call, llm_providers, llm_costs
                ├→ CDP Runner + WebMCP (5): cdp_run, cdp_list, webmcp_discover, webmcp_call, webmcp_context
                ├→ WebContent (1): web_fetch (content negotiation, SSRF, HTML→MD) — c2/webcontent
                ├→ C1 Messenger (5): search, mentions, briefing, send_message, update_presence + ContextKeeper
                ├→ Drive (6): upload, download, list, delete, info, mkdir
                ├→ Go Native — Tier 1 (17): Research (5) + C2 (6) + GPU (6)
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
| **hook 파일** | `~/.claude/hooks/c4-bash-security-hook.sh` | **대화형** | Bash 명령 Haiku 리뷰 hook |
| **settings.json 패치** | `~/.claude/settings.json` | **대화형** | PreToolUse Bash hook 등록 |

- `.mcp.json`은 **per-developer 파일** — 절대경로(`/Users/...`)가 포함되므로 git에 커밋하지 않음. clone 후 `cq init` 실행 시 자동 생성됨. 기존에 추적 중인 경우: `git rm --cached .mcp.json`
- hook/settings 설치는 **대화형 확인** 필요 — 사용자가 N 입력 시 건너뜀 (C4 핵심 기능에 영향 없음)
- `--yes` / `-y` 플래그: 모든 대화형 확인을 자동 승인 (CI/자동화 환경용)
- hook 파일은 바이너리에 embed되어 있어 소스 없이도 설치 가능
- 기존 `.conf` 파일(`c4-bash-security.conf`)은 삭제하지 않음 (하위 호환, fallback용)
- **hook 설정 SSOT**: `.c4/config.yaml`의 `permission_reviewer` 섹션 (`.conf` 파일 생성 안 함)
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
| `agent` | `serve.agent.enabled: true` + `cloud.url` + `cloud.anon_key` 설정 | Supabase Realtime @cq mention → claude -p |
| `ssesubscriber` | `serve.ssesubscriber.enabled: true` + `c5_hub && c3_eventbus` 빌드 태그 | C5 SSE 스트림 구독 → EventBus 전달 |

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
    enabled: true   # c5_hub && c3_eventbus 빌드 태그 필요; hub.enabled: true 필요
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

## C1 Messenger (c1/)

> Tauri 2.x 통합 대시보드 메신저. ~9.5K LOC(Rust) + ~11.8K LOC(TS+CSS). 92개 테스트.

### 아키텍처
- **Rust 백엔드**: `src-tauri/src/{commands,models,analytics,cloud,scanner,messaging,eventbus,lib}.rs`
- **Multi-Provider**: `src-tauri/src/providers/` — Claude Code, Codex CLI, Cursor
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
- **Tauri identifier**: `com.cq.c1` (tauri.conf.json). cq 리포에서 빌드 시 `cargo clean` 후 빌드하면 경로 캐시 이슈 없음.

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

## C5 Hub (c5/)

> Go 기반 분산 작업 큐 서버. Worker Pull 모델, Lease 기반. ~6.7K LOC.

### 빌드/실행
```bash
cd c5 && go build ./... && go test ./...
go build -o ~/bin/c5 ./cmd/c5/
```

### 설정 (c5.yaml)
```bash
# 초기 템플릿 생성
c5 serve --print-config > ~/.config/c5/c5.yaml
```

```yaml
# ~/.config/c5/c5.yaml
server:
  host: "0.0.0.0"
  port: 8585
eventbus:
  url: "http://localhost:4141"  # C4 EventSink
  token: ""
storage:
  path: "~/.local/share/c5"
```

우선순위: CLI 플래그 > 환경변수(C5_EVENTBUS_URL, C5_EVENTBUS_TOKEN) > c5.yaml > 기본값

- `eventbus.url` 미설정 시 이벤트 발행 비활성화 (graceful fallback).
- C4-core EventSink는 `:4141` 포트에서 `POST /v1/events/publish` 수신.
- 발행 이벤트: `hub.job.started`, `hub.job.completed`, `hub.job.failed`, `hub.job.cancelled`

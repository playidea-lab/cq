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

## Core Agent Principles

> Karpathy-inspired 4원칙 — 구현 전 가정 선언, 최소 코드, 정확한 범위, 목표 기반 루프.

1. **Think Before Coding** — 구현 시작 전 3줄 이내로 가정을 선언한다. 모호하지 않아도.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라. 최소 코드가 기본값이다.
3. **Surgical Changes** — 요청과 직접 연관된 줄만 수정한다. 인접 코드는 건드리지 않는다.
4. **Goal-Driven Execution** — 명령이 아닌 목표로 실행한다. 실패 테스트 → 통과가 기본 루프다.

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
| Go (`c4-core/`) | ~54.9K LOC | ~52.8K LOC | ~107.7K |
| Go (`c5/`) | ~7.3K LOC | ~6.4K LOC | ~13.7K |
| Python (`c4/`) | ~22.9K LOC | ~9.5K LOC | ~32.4K |
| Rust (`c1/src-tauri/`) | ~10.2K LOC | (내장) | ~10.2K |
| TS+CSS (`c1/src/`) | ~13.7K LOC | | ~13.7K |
| SQL (`infra/`) | ~1.3K LOC | | ~1.3K |
| **합계** | ~110.3K | ~68.7K | **~179.0K LOC** |

### 테스트 현황
| 언어 | 테스트 수 | 패키지/모듈 |
|------|----------|------------|
| Go | **~1,682** | 37 packages (all pass) — c4-core ~1,468 + c5 ~214 |
| Python | **697** | tests/unit/ |
| Rust | **92** | src-tauri |
| **합계** | **~2,466** | |

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

### Think Before Coding (구현 전 가정 선언)
- 구현 시작 전, **모호하지 않아도** 3줄 이내로 가정을 선언한다.
  예시: "가정: 이 함수가 nil을 받지 않는다 / 파일이 이미 존재한다 / 기존 테스트가 통과 상태다"
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

**기준**: 200줄이 50줄로 쓸 수 있다면 → 다시 써라. 시니어 엔지니어가 "과하다"고 할 만하면 → 단순화.

### Efficiency Rules (컨텍스트 절약)
- 큰 파일을 읽기 전에 **항상 line range를 제한**한다. 일부만 필요하면 전체를 읽지 않는다.
- 컨텍스트 사용을 최소화한다.

### Debugging (디버깅 원칙)
- MCP 서버나 도구 연결 문제 디버깅 시, 우회책 대신 **근본 원인(모듈 경로, config 오류 등)을 수정**한다.
- 도구/서버 연결 실패 시 설정과 모듈 경로부터 확인한다.

### Goal-Driven Execution (목표 기반 실행)
명령형 지시를 선언적 목표로 변환하여 실행한다. LLM은 구체적 목표를 루프로 달성하는 데 최적화되어 있다.

| 대신에... | 이렇게 |
|----------|--------|
| "X 추가해" | "X 실패 테스트 작성 → 통과시켜라" |
| "버그 수정해" | "재현 테스트 작성 → 통과시켜라" |
| "X 최적화해" | "현재 수치 측정 → 목표 달성 테스트" |

> "LLMs are exceptionally good at looping until they meet specific goals." — Karpathy

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

## Go Core (c4-core/) → [docs/ARCHITECTURE.md#go-core](docs/ARCHITECTURE.md)
주요: `go build -o ~/.local/bin/cq ./cmd/c4/` — 코드 수정 후 반드시 재빌드. `cp` 복사 금지(코드 서명 무효화).

---

## Python Sidecar (c4/) → [docs/ARCHITECTURE.md#python-sidecar-c4](docs/ARCHITECTURE.md)
주요: Lazy Start — 첫 proxy 호출 시에만 시작. LSP 도구는 Python/JS/TS only (Go/Rust는 `c4_search_for_pattern` 사용).

---

## C1 Messenger (c1/) → [docs/ARCHITECTURE.md#c1-messenger-c1](docs/ARCHITECTURE.md)
주요: Tauri 2.x + Rust 백엔드. 빌드: `cd c1 && pnpm install && cargo tauri dev`. 빌드 캐시 이슈 시 `cargo clean` 먼저.

---

## Infra (infra/supabase/) → [docs/ARCHITECTURE.md#infra-infrasupabase](docs/ARCHITECTURE.md)
주요: PostgreSQL 마이그레이션 21개. 스키마 변경은 반드시 migration 파일로 관리 (직접 DDL 금지).

---

## Knowledge Pipeline (지식 피드백 루프) → [docs/ARCHITECTURE.md#knowledge-pipeline-지식-피드백-루프](docs/ARCHITECTURE.md)
주요: `c4_submit` handoff에 `{discoveries, concerns, rationale}` 반드시 포함 — 자동 knowledge 기록의 소스.

---

## C3 EventBus (internal/eventbus/) → [docs/ARCHITECTURE.md#c3-eventbus-internaleventbus](docs/ARCHITECTURE.md)
주요: gRPC UDS + WebSocket bridge + DLQ (v4). 18종 이벤트. `c4_event_publish`로 발행, `c4_rule_add`로 구독.

---

## C5 Hub (c5/) → [docs/ARCHITECTURE.md#c5-hub-c5](docs/ARCHITECTURE.md)
주요: Worker Pull 모델, Lease 기반. `hub.enabled: true` + `hub.url` 설정 후 `c4_hub_submit`으로 잡 제출.

### cq serve 통합
`cq serve`의 `hub` 컴포넌트가 C5 바이너리를 서브프로세스로 자동 시작합니다.
```yaml
# .c4/config.yaml
serve:
  hub:
    enabled: true    # c5 바이너리를 자동 시작
    binary: "c5"     # PATH에서 찾을 바이너리명
    port: 8585       # c5 serve --port
    args: []         # 추가 CLI 인자
```
- SIGTERM → 5s 대기 → SIGKILL 종료 패턴
- Health check: `GET http://127.0.0.1:{port}/v1/health`
- **Cloud credential passthrough**: `cloud.url` / `cloud.anon_key` 설정 시 C5 서브프로세스에 `C5_SUPABASE_URL` / `C5_SUPABASE_KEY` 환경변수 자동 주입 (`HubComponentConfig.Env`)
- 바이너리 미설치 시: `c5_embed` 빌드 태그로 빌드된 경우 `~/.c4/bin/c5`로 자동 추출 후 사용, 없으면 graceful skip

#### c5 embed (c5_embed 빌드 태그)
`TIER=full` CI 빌드 시 c5 바이너리를 cq 내부에 내장합니다.
- 추출 경로: `~/.c4/bin/c5` (버전 캐시: `~/.c4/bin/.c5-version`)
- 버전 일치 시 fast-path (재추출 생략)
- CI: `build-c5` 스테이지 → `embed/c5/` 복사 → `build-cross TIER=full`
- 로컬 개발: `make embed-c5 C5_BIN=<path> C5_VERSION=<ver>` 후 `-tags c5_embed` 빌드

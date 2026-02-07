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

## CRITICAL: C4 Operation Pre-conditions

### c4_submit 전 필수 체크
1. `c4_status`로 태스크 상태 확인
2. 태스크가 `in_progress` 상태인지 검증
3. `pending` 상태면 → `c4_get_task`로 먼저 할당
4. 절대로 pending 상태의 태스크를 submit하지 않는다
5. 직접 DB 업데이트 금지 — MCP API만 사용

### 검증 후 진행
- Python → `uv run python -m py_compile <file>` 또는 관련 테스트
- Config → 형식 검증
- 검증 실패 시 → 다음 단계 진행 금지

### Bulk Operation (10개+ 파일)
1. 대상 파일 목록 나열 → 사용자 확인
2. 수정 후 전체 검증 (lint + test)

### Session Handoff
장시간 디버깅 종료 시 `c4_write_memory`에 기록:
- 발견한 문제 + 수정 사항
- 미해결 이슈
- 다음 세션 시작 지점

---

## MCP 도구 빠른 참조

```
상태: c4_status, c4_start, c4_clear
태스크: c4_add_todo, c4_get_task, c4_submit, c4_claim, c4_report
리뷰: c4_checkpoint, c4_ensure_supervisor
검증: c4_run_validation
코드: c4_find_symbol, c4_get_symbols_overview, c4_replace_symbol_body
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

## Canvas App (Project Explorer)

> `canvas-app/` — Tauri 2.x 데스크톱 앱. Claude Code 프로젝트 탐색기.

### 아키텍처
- **Rust 백엔드**: `src-tauri/src/{commands,models,scanner,lib}.rs`
- **React 프론트엔드**: `src/components/`, `src/hooks/`, `src/styles/`
- **CSS**: BEM 패턴 + `styles/tokens.css` 디자인 토큰

### 3개 뷰
| 뷰 | 데이터 소스 | Rust 커맨드 |
|-----|-------------|-------------|
| Sessions | `~/.claude/projects/{slug}/*.jsonl` | `list_sessions`, `get_session_messages` |
| Dashboard | `.c4/tasks.db` (rusqlite) | `get_project_state`, `get_tasks`, `get_task_detail` |
| Config | `~/.claude/`, `.claude/`, `.c4/` 파일 | `list_config_files`, `read_config_file` |

### 빌드/실행
```bash
cd canvas-app && pnpm install
cd src-tauri && cargo check && cargo test
pnpm build            # 프론트엔드 빌드
cargo tauri dev       # 개발 서버
```

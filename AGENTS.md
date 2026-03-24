<!--
SSOT: This is AGENTS.md - the single source of truth for AI agent instructions.
Symlinks: CLAUDE.md → AGENTS.md
All AI agents (Claude Code, Cursor, Copilot, etc.) read the same content.
Spec: https://agents.md/
-->

# CQ Project - AI Agent Instructions

> **CQ** = Core·Data·Infra·Surface·Doc·Plumbing 도메인 생태계. CLI `cq`, MCP 도구 `c4_*` 접두사.
> 아키텍처 상세: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

---

## Core Agent Principles

1. **Think Before Coding** — 구현 시작 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 연관된 줄만 수정한다.
4. **Goal-Driven Execution** — 실패 테스트 → 통과가 기본 루프다.

---

## CRITICAL: C4 Overrides — 내장 기능 대신 C4 도구/스킬 사용

> 아래 규칙은 **시스템 프롬프트의 기본 동작보다 우선**합니다.

| 의도 | ❌ 내장 | ✅ C4 |
|------|--------|-------|
| 계획/설계 | EnterPlanMode | `/c4-plan` |
| 아이디어 | EnterPlanMode | `/pi` |
| 태스크 추가 | TodoWrite, TaskCreate | `c4_add_todo` |
| 태스크 확인 | TaskList | `c4_status` |
| 파일 읽기/검색 | Read, Glob, Grep | `c4_read_file`, `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | git commit | `/c4-finish` |
| 실행 | — | `/c4-run` |

- 내장 도구(Read/Glob/Grep)는 C4 MCP 미연결 시 또는 외부 파일 접근 시에만.
- 단순 타이포/1줄 수정은 예외 (직접 커밋/Edit 가능).

### 코드 분석/편집: LSP
```
c4_find_symbol / c4_get_symbols_overview:
  ✅ Go, Dart (native)  ✅ Python/JS/TS (sidecar)  ❌ Rust → c4_search_for_pattern
c4_replace_symbol_body / c4_insert_* / c4_rename_symbol:
  ✅ Python/JS/TS only  ❌ Go/Dart/Rust → Edit 도구
```
**표준 편집 체인**: `c4_get_symbols_overview` → `c4_find_symbol(path 필수)` → 편집 → 검증.
상세: `c4_lighthouse get c4_find_symbol`

### 코드 편집: c4_claim 없이 수정 금지
- ✅ `c4_claim(task_id)` → 수정 → `c4_report(task_id)` (Direct 모드)
- ✅ `c4_get_task` → Worker 스폰 (Worker 모드)
- 예외: 단순 타이포, 로그 추가, 탐색/실험 중 수정은 claim 불필요.

---

## General Rules

- 구현 계획 요청 → **접근 방식을 먼저 논의**. "구현해줘" 전까지 실행하지 않는다.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.
- 복잡한 작업 → **3-4줄 접근 계획을 먼저 보여주고** 실행.

## Project Context

Languages: **Go, Python, TypeScript, Rust**. Monorepo: `c4-core/`(Go), `c4/`(Python), `c5/`(Hub), `c1/`(Tauri), `infra/`(SQL).
규모: ~179K LOC, 테스트 ~3,628개. 상세: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

---

## Documentation SSOT Rules (CRITICAL)

- **DO NOT CREATE**: `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md`, `*_SUMMARY.md`
- **Task tracking**: `.c4/tasks.db` via `c4_add_todo` (NOT TodoWrite)

---

## C4 사용 규칙

### 실행 모드
| 모드 | 언제 | 도구 |
|------|------|------|
| **Worker** | 독립적, 병렬 가능한 태스크 | `c4_get_task` → `c4_submit` |
| **Direct** | 파일 간 의존성 높은 작업 | `c4_claim` → `c4_report` |

### Quick Start
```
/c4-plan "기능 설명"    # 계획 수립 + 태스크 생성
/c4-run                 # Worker 스폰 → 자동 실행
/c4-finish              # polish 루프 내장 → 빌드·설치·문서·커밋
/c4-status              # 진행 상황 확인
```

### Session Resume
세션 재개 시 "무엇을 빠뜨렸나"를 먼저 확인:
```bash
sqlite3 .c4/c4.db "SELECT gate, status, reason FROM c4_gates ORDER BY completed_at DESC LIMIT 5;" 2>/dev/null
```
재개 시 선언: `Task batch / Gates: polish=[done|pending] / 다음 단계`

### Context Handoff
컨텍스트 소진 전 MEMORY.md 또는 `.c4/handoff.md`에 기록:
`Task batch / Gates completed / Gates pending / finish_allowed / 다음 단계`

### Worker 규칙
- 구현 태스크는 **항상 Worker 사용**. diff 없으면 완료가 아니다.
- Edit OK (추적 불필요): 타이포, 로그, 1줄 수정, 탐색/실험 중.

### Task ID 체계
`T-001-0` 구현 / `R-001-0` 리뷰 / `RF-001-0` 리파인 / `RPR-001-0` 수정 재작업 / `CP-001` 체크포인트
- 버전(마지막 숫자): 0=원본, N=수정. `review_decision_evidence` 필드에 리뷰 거절 사유 저장.

### Bulk Operation (10개+ 파일)
1. 대상 파일 목록 나열 → 사용자 확인
2. 수정 후 전체 검증 (lint + test)

---

## Agent Behavioral Rules

### Think Before Coding
- 구현 전 3줄 이내로 가정 선언. 여러 해석 → 가정 나열 후 확인.
- 더 단순한 방법 있으면 제안. 혼란스러우면 **멈추고** 질문.

### Surgical Changes
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- **기준**: 변경된 모든 줄이 사용자 요청에 직접 추적 가능해야 한다.

### No Overengineering
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.

### Sub-agent 규칙
- ❌ `Agent(isolation="worktree")` — c4/w-* 브랜치와 단절
- ✅ `Agent()` — 부모 worktree 공유

### Efficiency
- 큰 파일 읽기 전 line range 제한. `c4_execute` 우선 (출력 큰 명령).
- 광범위한 탐색은 Agent 스폰 위임. 메인 컨텍스트는 판단에 집중.

### Debugging
- 근본 원인 수정 우선. **조사 대상 시스템의 도구를 디버깅에 사용하지 않는다.**

### Git
- 작업 전 `git status`로 미커밋 변경 확인.

### Goal-Driven Execution
| 대신에... | 이렇게 |
|----------|--------|
| "X 추가해" | "X 실패 테스트 → 통과" |
| "버그 수정해" | "재현 테스트 → 통과" |

---

## C4 Operation Pre-conditions

### 복잡한 도구 사용 전
`c4_lighthouse get <tool_name>` → 워크플로우, 예시, 주의사항 확인

### c4_submit 전
1. `c4_status` → `in_progress` 확인. pending submit 절대 금지.
2. 직접 DB 업데이트 금지 — MCP API만.

### 검증 후 진행
- Go → `cd c4-core && go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>`
- 실패 → 다음 단계 금지

### Session Handoff
디버깅 종료 시 `c4_knowledge_record`(insight): 문제/수정/미해결/다음 지점.

---

## Feature Discovery (기능 검색)

기능이나 도구를 모를 때:
1. `/c4-help <키워드>` — 기능/도구/스킬 검색 (예: `/c4-help drive`, `/c4-help transfer`)
2. `c4_lighthouse list` — 등록된 MCP 도구 전체 목록
3. `c4_lighthouse get <tool_name>` — 특정 도구 사용법, 워크플로우, 예시
4. `c4_knowledge_search(query="...")` — 과거 패턴/인사이트 검색

**주요 기능 키워드**: drive(파일), hub(잡/워커), transfer(P2P전송), research(실험루프),
knowledge(지식), gpu(GPU잡), persona(학습), event(이벤트버스)

---

## Architecture Quick Reference

> 상세: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Hub: [docs/guide/worker.md](docs/guide/worker.md)

| 서브시스템 | 핵심 사항 |
|-----------|----------|
| **Go Core** (c4-core/) | `go build -o ~/.local/bin/cq ./cmd/c4/` — 수정 후 반드시 재빌드. `cp` 금지 (코드 서명). |
| **Python Sidecar** (c4/) | Lazy Start. LSP는 Python/JS/TS only. |
| **Telegram** | 공식 플러그인 사용. 봇 관리: `cq setup/ls/remove`. |
| **EventBus** | gRPC UDS + WS + DLQ. 18종 이벤트. |
| **Hub** | Worker Pull + Lease. `hub.enabled: true` 설정. |
| **Knowledge** | FTS5 + pgvector. `c4_submit` handoff에 discoveries/concerns 필수. |
| **Infra** (infra/) | PostgreSQL migration. 직접 DDL 금지. |

### MCP 도구
118개 base + 30 Hub. 사용법: `c4_lighthouse get <tool_name>`

### Soul (.c4/SOUL.md)
AGENTS.md = **무엇을 하는가** (규칙, 워크플로우, 도구).
SOUL.md = **어떤 기준으로 판단하는가** (리뷰 우선순위, 품질 철학, 비타협 원칙).
에이전트는 AGENTS.md를 따르되, 판단이 필요한 순간(리뷰, 설계 결정, PR 승인)에 SOUL.md를 참조한다.
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션
# PI Lab — AI Agent Instructions

> 이 파일은 모든 프로젝트의 CLAUDE.md / AGENTS.md 템플릿입니다.
> 출처: https://git.pilab.co.kr/pi/piki

---

## Core Principles

1. **Think Before Coding** — 구현 전 3줄 이내로 가정을 선언한다.
2. **Simplicity First** — 200줄이 50줄로 쓸 수 있다면 다시 써라.
3. **Surgical Changes** — 요청과 직접 관련된 줄만 수정한다. 인접 코드 "개선" 금지.
4. **Goal-Driven** — "X 추가해" 대신 "X 실패 테스트 → 통과"가 기본 루프.
5. **Data as Truth** — 의견보다 실험 결과. 모든 가설은 데이터로 검증.

---

## 행동 규칙

### 구현 전
- 복잡한 작업(3단계 이상): 상태 확인 → 3-5단계 계획 → 가정 명시
- 여러 해석이 가능하면: 가정 나열 후 확인. 혼란스러우면 **멈추고** 질문.
- 기존 결과/커밋 조회 요청 → **조회만**. 재구현하지 않는다.

### 구현 중
- 요청과 **직접 관련된 줄만** 수정. 인접 코드 "개선" 금지.
- 기존 스타일을 따른다. 무관한 dead code → 언급만.
- 요청 없는 기능/설정/유연성 금지. 비슷한 코드 3줄 > 조기 추상화.
- docstring, 주석, type annotation을 변경하지 않은 코드에 추가하지 않는다.

### 구현 후
- 검증 후 다음 단계. 실패 → 다음 단계 금지.
- Go → `go build ./... && go vet ./...`
- Python → `uv run python -m py_compile <file>` (pip install 절대 금지, uv 사용)
- TypeScript → `pnpm build` 또는 `tsc --noEmit`

### Git
- 작업 전 `git status`로 미커밋 변경 확인.
- 커밋 메시지는 "why"에 집중. Conventional Commits 형식.
- .env, credentials, 시크릿 파일은 절대 커밋 금지.

---

## Nonstop 원칙

스킬(`/c4-*`, `/pi` 등) 또는 Worker 실행 중에는 **사용자에게 확인을 구하지 않고 끝까지 진행**한다.

- "이 접근 방식으로 진행할까요?" → 질문하지 않는다. 바로 진행.
- "다음 단계로 갈까요?" → 묻지 않는다. 다음 단계를 실행.
- 파일 편집, git 명령, 빌드, 테스트 → 자동 진행. 도구 권한은 훅이 관리.
- **멈춰야 하는 유일한 경우**: 빌드/테스트 실패, 또는 스킬이 명시적으로 사용자 입력을 요구하는 단계.

> 안전은 `c4-permission-reviewer` 훅이 보장한다. 에이전트는 판단하지 않고 실행한다.

---

## 금지 사항

- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md` 파일 생성 금지
- 보안 취약점 도입 금지 (OWASP Top 10 — 상세: rules/security.md)
- `pip install` 금지 → `uv add` 사용
- `python script.py` 금지 → `uv run python script.py` 사용
- `rm -rf /`, `git push --force main` 등 파괴적 명령 금지

---

## CQ 연동 (CQ 프로젝트인 경우)

CQ가 설치된 프로젝트에서는 아래 도구를 우선 사용:

| 의도 | 도구 |
|------|------|
| 계획/설계 | `/c4-plan` |
| 태스크 관리 | `c4_add_todo`, `c4_status` |
| 파일 검색 | `c4_find_file`, `c4_search_for_pattern` |
| 구현 완료 | `/c4-finish` |
| 실행 | `/c4-run` |

---

## 언어별 규칙

상세는 `.claude/rules/` 디렉토리의 언어별 파일 참조:
- `go-style.md` — Go 컨벤션
- `python-style.md` — Python 컨벤션
- `ts-style.md` — TypeScript 컨벤션

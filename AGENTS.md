<!--
SSOT: This is AGENTS.md - the single source of truth for AI agent instructions.
Symlinks: CLAUDE.md → AGENTS.md
All AI agents (Claude Code, Cursor, Copilot, etc.) read the same content.
Spec: https://agents.md/
-->

# C4 Project - AI Agent Instructions

> C4: AI 오케스트레이션 시스템 - 계획부터 완료까지 자동화된 프로젝트 관리

---

## 🚨 Documentation SSOT Rules (CRITICAL)

**모든 AI 에이전트는 이 규칙을 반드시 따라야 합니다.**

### ❌ DO NOT CREATE these files:
- `PLAN.md`, `TODO.md`, `PHASES.md`, `DONE.md`
- `*_SUMMARY.md`, `CHECKPOINTS.md`
- Any duplicate planning/roadmap documents

### ✅ SINGLE SOURCE OF TRUTH:
| 용도 | 위치 | 관리 주체 |
|------|------|-----------|
| Project roadmap | `docs/ROADMAP.md` | Human |
| Task tracking | `.c4/tasks.db` | C4 System |
| AI instructions | **this file** | Human |
| Fixed specs | `docs/specs/` | Human |

### If you need to track work:
```bash
# Use C4 MCP tools, NOT markdown files
c4_add_todo(title, dod, ...)  # Add task
c4_status()                    # Check status
c4_get_task(worker_id)         # Get assigned task
```

### If asked to create planning docs:
1. Politely decline
2. Point to `docs/ROADMAP.md` for roadmap updates
3. Use C4 tools for task management

---

## 1. C4 시스템 개요

### 프로젝트 정의

C4 (Codex-Claude-Completion Control)는 AI 에이전트가 프로젝트를 자동으로 계획하고 실행할 수 있게 해주는 오케스트레이션 시스템입니다.

### 핵심 컴포넌트

| 컴포넌트 | 설명 |
|----------|------|
| **State Machine** | 프로젝트 상태 전이 관리 |
| **Task Store** | 태스크 큐 및 상태 관리 |
| **Worker Manager** | Worker 등록, 할당, 해제 |
| **Supervisor Loop** | 리뷰 및 체크포인트 처리 |
| **Agent Router** | 도메인별 에이전트 선택 |
| **Validation Runner** | lint, test 등 검증 실행 |
| **MCP Server** | Claude Code/Cursor와 통신 |

### 디렉토리 구조

```
.c4/
├── config.yaml       # 프로젝트 설정
├── state.json        # 현재 상태 (SQLite로 마이그레이션 중)
├── tasks.db          # 태스크 저장소 (SQLite)
├── events/           # 이벤트 로그
├── specs/            # Discovery 단계 명세
├── designs/          # Design 단계 설계
├── checkpoints/      # 체크포인트 번들
└── locks/            # Worker 파일 잠금
```

---

## 2. State Machine 규칙

### 프로젝트 상태

| 상태 | 설명 | 허용 명령어 |
|------|------|-------------|
| **INIT** | 초기 상태, 프로젝트 미초기화 | `init` |
| **DISCOVERY** | 요구사항 수집 (EARS 패턴) | `plan`, `status`, `stop` |
| **DESIGN** | 아키텍처 설계 | `plan`, `status`, `stop` |
| **PLAN** | 태스크 생성 및 계획 | `plan`, `run`, `stop`, `status` |
| **EXECUTE** | Worker가 태스크 실행 중 | `status`, `worker join`, `worker submit`, `stop` |
| **CHECKPOINT** | Supervisor 리뷰 중 | `status` |
| **COMPLETE** | 프로젝트 완료 | `status` |
| **HALTED** | 수동 중지 상태 | `run`, `plan`, `status` |
| **ERROR** | 복구 불가 오류 | `status` |

### 워크플로우 다이어그램

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → COMPLETE
                     ↑         ↑        ↓
                     └─────────┴────────┘ (replan/redesign)
```

### 상태 전이 규칙

```python
# (현재 상태, 이벤트) → 다음 상태

# INIT
(INIT, "c4_init") → DISCOVERY          # 신규 워크플로우
(INIT, "c4_init_legacy") → PLAN        # 레거시 모드

# DISCOVERY
(DISCOVERY, "discovery_complete") → DESIGN
(DISCOVERY, "skip_discovery") → PLAN   # Discovery 건너뛰기
(DISCOVERY, "c4_stop") → HALTED

# DESIGN
(DESIGN, "design_approved") → PLAN
(DESIGN, "design_rejected") → DISCOVERY  # 재분석
(DESIGN, "skip_design") → PLAN
(DESIGN, "c4_stop") → HALTED

# PLAN
(PLAN, "c4_run") → EXECUTE
(PLAN, "c4_stop") → HALTED
(PLAN, "back_to_design") → DESIGN

# EXECUTE
(EXECUTE, "gate_reached") → CHECKPOINT
(EXECUTE, "c4_stop") → HALTED
(EXECUTE, "all_done") → COMPLETE
(EXECUTE, "fatal_error") → ERROR

# CHECKPOINT
(CHECKPOINT, "approve") → EXECUTE        # 계속 실행
(CHECKPOINT, "approve_final") → COMPLETE # 최종 승인
(CHECKPOINT, "request_changes") → EXECUTE
(CHECKPOINT, "replan") → PLAN
(CHECKPOINT, "redesign") → DESIGN

# HALTED
(HALTED, "c4_run") → EXECUTE
(HALTED, "c4_plan") → PLAN
(HALTED, "c4_discovery") → DISCOVERY
```

### CRITICAL - 상태 전이 규칙

- **허용되지 않은 전이 시도 → `StateTransitionError`**
- **COMPLETE, ERROR 상태에서는 전이 불가** (수동 개입 필요)
- **상태 변경 전 invariant 검사 수행**

---

## 3. Task Lifecycle

### Task ID 체계

| 유형 | 형식 | 예시 | 설명 |
|------|------|------|------|
| **Implementation** | `T-{번호}-{버전}` | T-001-0 | 구현 태스크 |
| **Review** | `R-{번호}-{버전}` | R-001-0 | 코드 리뷰 태스크 |
| **Checkpoint** | `CP-{번호}` | CP-001 | Phase 체크포인트 |
| **Repair** | `RPR-{번호}-{깊이}` | RPR-001-1 | 수리 태스크 |

### Task 상태 전이

```
pending → in_progress → done
            ↓
         blocked (repair queue로 이동)
```

| 상태 | 설명 |
|------|------|
| `pending` | 대기 중 (의존성 미충족 또는 미할당) |
| `in_progress` | Worker에게 할당됨 |
| `done` | 완료 (검증 통과) |
| `blocked` | 차단됨 (max retry 초과) |

### Task 모델 필드

```python
class Task:
    id: str                    # T-001-0, R-001-0, CP-001
    title: str                 # 태스크 제목
    scope: str | None          # 파일/디렉토리 범위 (잠금용)
    priority: int              # 높을수록 먼저 할당 (기본: 0)
    dod: str                   # Definition of Done
    validations: list[str]     # ["lint", "unit"]
    dependencies: list[str]    # 의존 태스크 ID 목록
    status: TaskStatus         # pending, in_progress, done
    assigned_to: str | None    # Worker ID
    branch: str | None         # 작업 브랜치
    commit_sha: str | None     # 완료 커밋

    # 도메인/라우팅
    domain: str | None         # web-frontend, ml-dl, ...
    task_type: str | None      # review, debug, security

    # Review-as-Task 필드
    type: TaskType             # IMPLEMENTATION, REVIEW, CHECKPOINT
    base_id: str | None        # 기본 번호 (001)
    version: int               # 버전 (0=원본, 1+=수정)
    parent_id: str | None      # 부모 태스크 ID
    review_decision: str | None  # APPROVE, REQUEST_CHANGES, REPLAN
```

### Definition of Done (DoD)

태스크 완료 조건을 명시하는 필수 필드:

```yaml
# 좋은 DoD 예시
dod: |
  - [ ] UserService.create_user() 구현
  - [ ] 단위 테스트 3개 이상 작성
  - [ ] lint 통과
  - [ ] 기존 테스트 깨지지 않음

# 나쁜 DoD 예시
dod: "사용자 기능 구현"  # 모호함
```

---

## 4. Review-as-Task 워크플로우

### 자동 리뷰 태스크 생성

구현 태스크(T-XXX-N) 완료 시 자동으로 리뷰 태스크(R-XXX-N) 생성:

```
T-001-0 완료 → R-001-0 자동 생성
               ↓
    ┌──────────┴──────────┐
    ↓                      ↓
APPROVE                REQUEST_CHANGES
    ↓                      ↓
   완료              T-001-1 생성 → R-001-1 → ...
```

### 리뷰 결과 처리

| 결정 | 동작 |
|------|------|
| **APPROVE** | 태스크 완료, 다음 태스크로 이동 |
| **REQUEST_CHANGES** | 다음 버전 태스크(T-XXX-N+1) 생성 |
| **REPLAN** | PLAN 상태로 전이, 태스크 재구성 |

### CRITICAL - 리뷰 규칙

- **구현 태스크는 반드시 리뷰 태스크를 거쳐야 완료**
- **리뷰 태스크의 DoD는 자동 생성** (부모 태스크 참조)
- **버전 번호는 자동 증가** (T-001-0 → T-001-1)

---

## 5. Checkpoint 관리

### Checkpoint 트리거 조건

다음 조건 모두 충족 시 체크포인트 태스크(CP-XXX) 생성:

1. Phase 내 모든 구현 태스크 완료
2. 해당 구현 태스크들의 리뷰 태스크 모두 APPROVE
3. 의존성 그래프상 다음 phase로 진행 가능

### Checkpoint 프로세스

```
모든 R-XXX APPROVE
       ↓
   CP-XXX 생성
       ↓
 Supervisor 리뷰
       ↓
┌──────┼──────┬───────┐
↓      ↓      ↓       ↓
APPROVE  REQUEST  REPLAN  REDESIGN
       CHANGES
```

### 결정 옵션

| 결정 | 동작 | 전이 |
|------|------|------|
| **APPROVE** | 다음 phase 진행 | EXECUTE 유지 또는 COMPLETE |
| **REQUEST_CHANGES** | 특정 태스크 재작업 | EXECUTE 유지 |
| **REPLAN** | 태스크 재구성 | → PLAN |
| **REDESIGN** | 설계 재검토 | → DESIGN |

### Checkpoint Bundle

```
.c4/checkpoints/CP-001/
├── bundle.json       # 메타데이터
├── tasks.json        # 관련 태스크 스냅샷
├── diffs/            # 변경 사항 diff
└── review.json       # 리뷰 결과
```

---

## 6. Agent Routing

### 도메인 목록

| 도메인 | 설명 | 기본 에이전트 |
|--------|------|---------------|
| `web-frontend` | React, Vue, CSS | frontend-developer |
| `web-backend` | FastAPI, Django, Node | backend-architect |
| `fullstack` | 프론트+백엔드 | fullstack (없으면 backend) |
| `ml-dl` | ML/DL 파이프라인 | ml-engineer |
| `mobile-app` | React Native, Flutter | mobile-developer |
| `infra` | Terraform, K8s, Docker | cloud-architect |
| `library` | SDK, 패키지 개발 | backend-architect |
| `unknown` | 미분류 | general-purpose |

### 태스크 유형별 오버라이드

| task_type | 오버라이드 에이전트 |
|-----------|---------------------|
| `review` | code-reviewer |
| `debug` | debugger |
| `security` | security-auditor |
| `test` | test-automator |
| `performance` | performance-engineer |

### 라우팅 우선순위

```
1. task.domain + task.task_type → 오버라이드 매칭
2. task.domain → 도메인 기본 에이전트
3. project.domain → 프로젝트 기본 에이전트
4. "unknown" → general-purpose
```

---

## 7. MCP Tools 사용 가이드

### 상태 관리 도구

| 도구 | 설명 |
|------|------|
| `c4_status` | 프로젝트 상태, 큐, 워커 정보 조회 |
| `c4_clear` | C4 상태 초기화 (개발용) |
| `c4_start` | PLAN/HALTED → EXECUTE 전이 |

### 태스크 관리 도구

| 도구 | 설명 |
|------|------|
| `c4_add_todo` | 새 태스크 추가 (의존성 지정 가능) |
| `c4_get_task` | Worker가 다음 태스크 요청 |
| `c4_submit` | 태스크 완료 보고 (검증 결과 포함) |
| `c4_mark_blocked` | 태스크 차단 (repair queue로) |

### Supervisor 도구

| 도구 | 설명 |
|------|------|
| `c4_ensure_supervisor` | Supervisor 루프 시작/확인 |
| `c4_checkpoint` | 체크포인트 결정 기록 |

### 검증 도구

| 도구 | 설명 |
|------|------|
| `c4_run_validation` | lint, unit, integration 실행 |

### Discovery/Design 도구

| 도구 | 설명 |
|------|------|
| `c4_save_spec` | EARS 요구사항 저장 |
| `c4_list_specs` | 명세 목록 조회 |
| `c4_get_spec` | 특정 명세 조회 |
| `c4_discovery_complete` | Discovery → Design 전이 |
| `c4_save_design` | 설계 명세 저장 |
| `c4_list_designs` | 설계 목록 조회 |
| `c4_get_design` | 특정 설계 조회 |
| `c4_design_complete` | Design → Plan 전이 |

### Agent Graph 도구

| 도구 | 설명 |
|------|------|
| `c4_test_agent_routing` | 에이전트 라우팅 테스트 |
| `c4_query_agent_graph` | 에이전트 그래프 쿼리 |

---

## 8. Validation 규칙

### 검증 유형

| 이름 | 설명 | 기본 명령어 |
|------|------|-------------|
| `lint` | 코드 스타일 검사 | `ruff check .` |
| `unit` | 단위 테스트 | `pytest tests/unit/` |
| `integration` | 통합 테스트 | `pytest tests/integration/` |
| `typecheck` | 타입 검사 | `mypy .` |

### 설정 (.c4/config.yaml)

```yaml
validations:
  lint:
    command: "uv run ruff check ."
    required: true
  unit:
    command: "uv run pytest tests/unit/ -v"
    required: true
  integration:
    command: "uv run pytest tests/integration/ -v"
    required: false  # 선택적
  typecheck:
    command: "uv run mypy ."
    required: false
```

### CRITICAL - 검증 규칙

- **`required: true` 검증 실패 시 태스크 완료 불가**
- **모든 검증은 uv 환경에서 실행** (`uv run ...`)
- **검증 실패 3회 → repair queue로 이동**

---

## 9. Worker 프로토콜

### Worker 등록

```python
# c4_get_task 호출 시 자동 등록
response = c4_get_task(worker_id="worker-1")
```

### Heartbeat

- Worker는 주기적으로 `c4_get_task` 호출하여 생존 신호
- 일정 시간 무응답 시 할당된 태스크 회수

### Task 할당 흐름

```
Worker → c4_get_task(worker_id)
           ↓
    1. 의존성 충족된 pending 태스크 검색
    2. scope 잠금 확인
    3. priority 높은 순 선택
    4. status = in_progress, assigned_to = worker_id
           ↓
Worker ← TaskAssignment(task_id, branch, ...)
```

### Task 완료 흐름

```
Worker → c4_submit(task_id, commit_sha, validation_results)
           ↓
    1. 검증 결과 확인
    2. 모두 pass → status = done
    3. fail 있으면 → repair 시도 또는 blocked
           ↓
    4. 리뷰 태스크 자동 생성 (구현 태스크인 경우)
```

---

## 10. 기존 규칙 참조

다음 규칙 파일들과 함께 사용됩니다:

| 파일 | 내용 |
|------|------|
| `.claude/rules/coding-style.md` | 코딩 스타일, 파일 크기, 명명 규칙 |
| `.claude/rules/testing.md` | TDD, 커버리지 요구사항 |
| `.claude/rules/git-workflow.md` | 커밋 메시지, 브랜치 전략 |
| `.claude/rules/security.md` | 보안 체크리스트 |
| `.claude/rules/debugging.md` | 체계적 디버깅 프로세스 |
| `.claude/rules/context-management.md` | MCP 서버 관리, 컨텍스트 최적화 |

### C4 브랜치 규칙

C4 Worker가 자동 생성하는 브랜치:

```
c4/w-{task_id}
예: c4/w-T-001-0, c4/w-R-001-0
```

- **Worker가 자동 생성/전환**
- **태스크 완료 후 자동 병합 (검증 통과 시)**
- **수동 수정 금지** (Worker가 관리)

---

## 11. 슬래시 명령어

### 사용 가능한 명령어

| 명령어 | 설명 | 상태 요구 |
|--------|------|-----------|
| `/c4-init` | C4 초기화 | INIT |
| `/c4-status` | 상태 확인 | 모든 상태 |
| `/c4-plan` | 태스크 계획 | DISCOVERY, DESIGN, PLAN, HALTED |
| `/c4-run` | 실행 시작 | PLAN, HALTED |
| `/c4-stop` | 실행 중지 | EXECUTE |
| `/c4-validate` | 검증 실행 | EXECUTE |
| `/c4-submit` | 태스크 제출 | EXECUTE |
| `/c4-checkpoint` | 체크포인트 리뷰 | CHECKPOINT |
| `/c4-add-task` | 태스크 추가 | PLAN |

### CRITICAL - 명령어 규칙

- **현재 상태에서 허용되지 않은 명령어 실행 시 오류**
- **`/c4-run` 전에 최소 1개 태스크 필요**
- **`/c4-submit` 전에 검증 실행 권장**

---

## 12. 트러블슈팅

### 일반적인 문제

| 문제 | 원인 | 해결 |
|------|------|------|
| "No tasks available" | 모든 태스크 완료 또는 의존성 미충족 | `c4_status`로 확인 |
| "State transition not allowed" | 잘못된 상태에서 명령 실행 | 현재 상태 확인 |
| "Validation failed" | lint/test 실패 | 로그 확인 후 수정 |
| "Task blocked" | 3회 이상 실패 | repair queue 확인 |

### 상태 초기화

개발 중 상태 꼬임 발생 시:

```bash
c4 clear --confirm  # 모든 C4 상태 초기화
```

### 로그 확인

```bash
ls .c4/events/       # 이벤트 로그
cat .c4/state.json   # 현재 상태
```

---

## Quick Reference

### 워크플로우 요약

```
1. /c4-init          → DISCOVERY
2. /c4-plan          → DESIGN → PLAN (태스크 생성)
3. /c4-run           → EXECUTE (Worker 루프 시작)
4. Worker 자동 실행  → 태스크 할당/완료/리뷰
5. CHECKPOINT        → Supervisor 리뷰
6. APPROVE           → COMPLETE 또는 다음 phase
```

### 태스크 ID 빠른 참조

```
T-001-0  : 첫 번째 구현 태스크, 원본 버전
T-001-1  : 첫 번째 구현 태스크, 1차 수정
R-001-0  : 첫 번째 리뷰 태스크
CP-001   : 첫 번째 체크포인트
RPR-001-1: 첫 번째 수리 태스크, 깊이 1
```

### MCP 도구 빠른 참조

```
상태: c4_status, c4_start, c4_clear
태스크: c4_add_todo, c4_get_task, c4_submit
리뷰: c4_checkpoint, c4_ensure_supervisor
검증: c4_run_validation
```

---

## C4 사용 규칙

> C4가 이 프로젝트에 활성화되었습니다.

### 핵심 원칙: 메인 세션은 계획하고, Worker가 실행한다

```
✅ 올바른 흐름:
   메인: /c4-plan → 태스크 생성 → /c4-run → Worker들이 실행
   메인: 진행 상황 모니터링, 리뷰, 의사결정

❌ 잘못된 흐름:
   메인: /c4-plan → 태스크 생성 → "직접 하겠습니다" → 코드 작성
   (Worker와 충돌 가능, submit 실패 가능)
```

**메인 세션의 역할:**
- 계획 수립 (`/c4-plan`)
- 태스크 생성 (`c4_add_todo`)
- Worker 스폰 (`/c4-run`)
- 진행 상황 확인 (`/c4-status`)
- 체크포인트 리뷰 (`/c4-checkpoint`)

**구현 작업은 Worker에게 맡겨라:**
```
/c4-run           # Worker 스폰 → Worker가 알아서 태스크 처리
/c4-run 3         # 3개 Worker 병렬 스폰
```

### C4 필수 (기본값)
- 2개 이상 파일 수정
- 테스트가 필요한 변경
- 리뷰가 필요한 코드
- 롤백 가능해야 하는 작업

### Edit OK (예외)
- 단순 타이포 수정
- 로그/디버그 추가
- 1줄 수정
- 탐색/실험 중

### Quick Start
```
/c4-plan "기능 설명"    # 계획 수립 + 태스크 생성
/c4-run                 # Worker 스폰 → 자동 실행
/c4-status              # 진행 상황 확인
```

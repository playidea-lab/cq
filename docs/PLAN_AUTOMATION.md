# C4 완전 자동화 개발 계획 v2

> **목표**: "AI는 멈추지 않는다" - Worker Loop + Supervisor Loop 비동기 구현

---

## 1. 현재 구현 상태

### 1.1 완료된 부분 ✅

| 컴포넌트 | 파일 | 상태 |
|----------|------|------|
| MCP Server | `c4/mcp_server.py` | ✅ |
| State Machine | `c4/state_machine.py` | ✅ |
| Validation Runner | `c4/validation.py` | ✅ |
| Bundle Creator | `c4/bundle.py` | ✅ |
| Supervisor | `c4/supervisor.py` | ✅ |
| Lock Manager | `c4/daemon/locks.py` | ✅ |
| Worker Manager | `c4/daemon/workers.py` | ✅ |

### 1.2 누락된 부분 ❌

| 컴포넌트 | 설명 |
|----------|------|
| **Worker Ralph Loop** | task→구현→검증→제출→다음task 자동 반복 (Skill) |
| **Checkpoint Queue** | 완료된 checkpoint 대기열 |
| **Supervisor Loop** | checkpoint queue 비동기 처리 (Daemon) |
| **Repair Queue** | 에러 복구용 Supervisor 위임 |

---

## 2. 목표 아키텍처

```
┌─────────────────────────────────────────────────────────────────────┐
│                              c4d Daemon                              │
│                                                                      │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐  │
│  │   Task Queue    │    │ Checkpoint Queue│    │  Repair Queue   │  │
│  │   (pending)     │    │ (pending CPs)   │    │ (blocked tasks) │  │
│  └────────┬────────┘    └────────┬────────┘    └────────┬────────┘  │
│           │                      │                      │           │
│           ▼                      ▼                      ▼           │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                 Orchestration Engine                         │   │
│  │                                                              │   │
│  │  • Worker 할당 (scope lock)                                  │   │
│  │  • Checkpoint 조건 체크 → Queue 추가                         │   │
│  │  • Supervisor Loop (background)                              │   │
│  │  • Repair 처리 (blocked → supervisor)                        │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└──────────────────────────────┬───────────────────────────────────────┘
                               │ MCP
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
         ▼                     ▼                     ▼
    ┌─────────┐          ┌─────────┐          ┌───────────────┐
    │ Worker1 │          │ Worker2 │          │  Supervisor   │
    │ (Claude │          │ (Claude │          │  (Headless)   │
    │  Code)  │          │  Code)  │          │               │
    └─────────┘          └─────────┘          └───────────────┘
```

### 2.1 Worker Ralph Loop (Skill 기반)

```
┌─────────────────────────────────────────────────────────────┐
│                     Worker Ralph Loop                        │
│                                                              │
│   START                                                      │
│     │                                                        │
│     ▼                                                        │
│  ┌──────────────┐                                           │
│  │ c4_get_task  │◄────────────────────────────────┐        │
│  └──────┬───────┘                                 │        │
│         │                                          │        │
│    ┌────┴────┐                                    │        │
│    │ task?   │── NO ──► EXIT(complete)            │        │
│    └────┬────┘                                    │        │
│         │ YES                                      │        │
│         ▼                                          │        │
│  ┌──────────────┐                                 │        │
│  │  Implement   │  (Claude Code 코드 작성)         │        │
│  └──────┬───────┘                                 │        │
│         ▼                                          │        │
│  ┌──────────────┐                                 │        │
│  │c4_run_valid  │                                 │        │
│  └──────┬───────┘                                 │        │
│         │                                          │        │
│    ┌────┴────┐   FAIL (< 10회)                    │        │
│    │ pass?   │─────────────► Fix & Retry ─────────┤        │
│    └────┬────┘                                    │        │
│         │ PASS        │ FAIL (≥ 10회)             │        │
│         │             ▼                           │        │
│         │      ┌──────────────┐                   │        │
│         │      │ Mark BLOCKED │                   │        │
│         │      │ → Repair Q   │                   │        │
│         │      └──────┬───────┘                   │        │
│         │             │                           │        │
│         │             ▼                           │        │
│         │      EXIT(await_repair)                 │        │
│         ▼                                          │        │
│  ┌──────────────┐                                 │        │
│  │  c4_submit   │                                 │        │
│  └──────┬───────┘                                 │        │
│         │                                          │        │
│    ┌────┴─────────────┬──────────────┐            │        │
│    ▼                  ▼              ▼            │        │
│ get_next      await_checkpoint   complete         │        │
│    │                  │              │            │        │
│    │                  ▼              ▼            │        │
│    │           Poll status      EXIT(done)        │        │
│    │                  │                           │        │
│    │             ┌────┴────┐                      │        │
│    │             │EXECUTE? │── YES ───────────────┤        │
│    │             └────┬────┘                      │        │
│    │                  │ NO (still CHECKPOINT)     │        │
│    │                  ▼                           │        │
│    │             Wait & Retry                     │        │
│    │                                              │        │
│    └──────────────────────────────────────────────┘        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Supervisor Loop (Daemon 기반)

```
┌─────────────────────────────────────────────────────────────┐
│                     Supervisor Loop (Daemon)                 │
│                                                              │
│   START (daemon startup)                                     │
│     │                                                        │
│     ▼                                                        │
│  ┌──────────────────┐                                       │
│  │ checkpoint_queue │◄────────────────────────────┐        │
│  │   .get()         │                             │        │
│  └────────┬─────────┘                             │        │
│           │                                        │        │
│      ┌────┴────┐                                  │        │
│      │ empty?  │── YES ──► Sleep(1s) ─────────────┤        │
│      └────┬────┘                                  │        │
│           │ NO                                     │        │
│           ▼                                        │        │
│    ┌──────────────┐                               │        │
│    │create_bundle │                               │        │
│    └──────┬───────┘                               │        │
│           ▼                                        │        │
│    ┌──────────────┐                               │        │
│    │run_supervisor│  claude -p "..."              │        │
│    └──────┬───────┘                               │        │
│           ▼                                        │        │
│    ┌──────────────┐                               │        │
│    │parse_decision│                               │        │
│    └──────┬───────┘                               │        │
│           │                                        │        │
│    ┌──────┼──────────────┬──────────────┐        │        │
│    ▼      ▼              ▼              ▼        │        │
│ APPROVE REQUEST_CH    REPLAN        (error)      │        │
│    │      │              │              │        │        │
│    ▼      ▼              ▼              ▼        │        │
│ →EXECUTE →EXECUTE     →PLAN        Retry(3)      │        │
│ or DONE  +new tasks                              │        │
│    │      │              │              │        │        │
│    └──────┴──────────────┴──────────────┘        │        │
│                          │                        │        │
│                          ▼                        │        │
│                   Save state                      │        │
│                          │                        │        │
│                          └────────────────────────┘        │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. 구현 계획

### Phase 1: Queue Infrastructure (P0)

#### T-AUTO-01: Queue 모델 추가

**파일**: `c4/models/queue.py` (신규)

```python
from enum import Enum
from pydantic import BaseModel

class CheckpointQueueItem(BaseModel):
    checkpoint_id: str
    triggered_at: str
    tasks_completed: list[str]
    validation_results: dict

class RepairQueueItem(BaseModel):
    task_id: str
    failure_signature: str
    attempts: int
    blocked_at: str

class QueueState(BaseModel):
    checkpoint_queue: list[CheckpointQueueItem] = []
    repair_queue: list[RepairQueueItem] = []
```

#### T-AUTO-02: State에 Queue 통합

**파일**: `c4/models/state.py` 수정

```python
class C4State(BaseModel):
    # ... existing fields ...
    checkpoint_queue: list[CheckpointQueueItem] = []
    repair_queue: list[RepairQueueItem] = []
```

### Phase 2: Async Supervisor (P0)

#### T-AUTO-03: c4_submit 비동기화

**파일**: `c4/mcp_server.py` 수정

**변경 전**:
```python
def c4_submit(self, ...):
    if checkpoint_reached:
        # BLOCKING
        result = self.run_supervisor_review()
        return SubmitResponse(...)
```

**변경 후**:
```python
def c4_submit(self, ...):
    # ... validation logic ...

    cp_id = self.state_machine.check_gate_conditions(self.config)
    if cp_id:
        # NON-BLOCKING: Queue에 추가만
        self._add_to_checkpoint_queue(cp_id)
        return SubmitResponse(
            success=True,
            next_action="await_checkpoint",
            message=f"Checkpoint {cp_id} queued for review",
        )

    return SubmitResponse(success=True, next_action="get_next_task")

def _add_to_checkpoint_queue(self, cp_id: str):
    """Add checkpoint to queue for async processing"""
    item = CheckpointQueueItem(
        checkpoint_id=cp_id,
        triggered_at=datetime.now().isoformat(),
        tasks_completed=list(self.state_machine.state.queue.done),
        validation_results=self.state_machine.state.last_validation,
    )
    self.state_machine.state.checkpoint_queue.append(item)
    self.state_machine.save_state()
```

#### T-AUTO-04: Supervisor Loop 구현

**파일**: `c4/daemon/supervisor_loop.py` (신규)

```python
import asyncio
import logging
from pathlib import Path

from ..supervisor import Supervisor, SupervisorError
from ..models import SupervisorDecision

logger = logging.getLogger(__name__)

class SupervisorLoop:
    """Background loop that processes checkpoint queue"""

    def __init__(self, daemon: "C4Daemon"):
        self.daemon = daemon
        self.running = False
        self.poll_interval = 1.0  # seconds

    async def start(self):
        """Start the supervisor loop"""
        self.running = True
        logger.info("Supervisor loop started")

        while self.running:
            try:
                await self._process_checkpoint_queue()
                await self._process_repair_queue()
            except Exception as e:
                logger.error(f"Supervisor loop error: {e}")

            await asyncio.sleep(self.poll_interval)

    def stop(self):
        """Stop the supervisor loop"""
        self.running = False
        logger.info("Supervisor loop stopped")

    async def _process_checkpoint_queue(self):
        """Process pending checkpoints"""
        state = self.daemon.state_machine.state

        if not state.checkpoint_queue:
            return

        # Get next checkpoint
        item = state.checkpoint_queue[0]
        logger.info(f"Processing checkpoint: {item.checkpoint_id}")

        try:
            # Create bundle and run supervisor
            bundle_dir = self.daemon.create_checkpoint_bundle(item.checkpoint_id)

            supervisor = Supervisor(
                self.daemon.root,
                prompts_dir=self.daemon.root / "prompts"
            )

            response = supervisor.run_supervisor(bundle_dir, timeout=300, max_retries=3)

            # Apply decision
            self._apply_decision(item.checkpoint_id, response)

            # Remove from queue
            state.checkpoint_queue.pop(0)
            self.daemon.state_machine.save_state()

        except SupervisorError as e:
            logger.error(f"Supervisor failed for {item.checkpoint_id}: {e}")
            # Keep in queue for retry

    async def _process_repair_queue(self):
        """Process blocked tasks that need supervisor guidance"""
        state = self.daemon.state_machine.state

        if not state.repair_queue:
            return

        item = state.repair_queue[0]
        logger.info(f"Processing repair: {item.task_id}")

        # Create repair bundle and get supervisor guidance
        # ... implementation ...

    def _apply_decision(self, checkpoint_id: str, response):
        """Apply supervisor decision to state"""
        result = self.daemon.c4_checkpoint(
            checkpoint_id=checkpoint_id,
            decision=response.decision.value,
            notes=response.notes,
            required_changes=response.required_changes,
        )
        logger.info(f"Checkpoint {checkpoint_id}: {response.decision.value}")
```

#### T-AUTO-05: Daemon에 Supervisor Loop 통합

**파일**: `c4/mcp_server.py` 수정

```python
class C4Daemon:
    def __init__(self, ...):
        # ... existing ...
        self._supervisor_loop: SupervisorLoop | None = None
        self._loop_task: asyncio.Task | None = None

    def start_supervisor_loop(self):
        """Start background supervisor loop"""
        if self._supervisor_loop is None:
            from .daemon.supervisor_loop import SupervisorLoop
            self._supervisor_loop = SupervisorLoop(self)

        if self._loop_task is None or self._loop_task.done():
            self._loop_task = asyncio.create_task(self._supervisor_loop.start())

    def stop_supervisor_loop(self):
        """Stop background supervisor loop"""
        if self._supervisor_loop:
            self._supervisor_loop.stop()
```

### Phase 3: Repair Queue (P0)

#### T-AUTO-06: Blocked Task → Repair Queue

**파일**: `c4/mcp_server.py` 수정

```python
def c4_mark_blocked(self, task_id: str, failure_signature: str, attempts: int):
    """Mark task as blocked and add to repair queue"""
    state = self.state_machine.state

    # Move task from in_progress to blocked
    if task_id in state.queue.in_progress:
        del state.queue.in_progress[task_id]

    # Add to repair queue
    item = RepairQueueItem(
        task_id=task_id,
        failure_signature=failure_signature,
        attempts=attempts,
        blocked_at=datetime.now().isoformat(),
    )
    state.repair_queue.append(item)

    self.state_machine.save_state()

    return {"success": True, "message": f"Task {task_id} queued for repair"}
```

#### T-AUTO-07: MCP Tool 추가

**파일**: `c4/mcp_server.py` - `list_tools()` 수정

```python
Tool(
    name="c4_mark_blocked",
    description="Mark a task as blocked after max retry attempts. Adds to repair queue for supervisor guidance.",
    inputSchema={
        "type": "object",
        "properties": {
            "task_id": {"type": "string"},
            "failure_signature": {"type": "string", "description": "Error signature from validation"},
            "attempts": {"type": "integer", "description": "Number of fix attempts made"},
        },
        "required": ["task_id", "failure_signature", "attempts"],
    },
),
```

### Phase 4: Worker Skill (P0)

#### T-AUTO-08: `/c4-worker` Skill 재작성

**파일**: `.claude/commands/c4-worker.md`

```markdown
# C4 Worker - Ralph Loop

자동으로 task를 받아 구현하고, 검증하고, 제출하는 루프를 실행합니다.

## Instructions

이 스킬이 호출되면 다음 루프를 **자동으로 반복**합니다.

### Loop Start

```
LOOP:
```

### 1. Task 가져오기

```
task = mcp__c4__c4_get_task("claude-worker")
```

- `task`가 `null`이면 → `<loop_exit>COMPLETE: 모든 작업 완료</loop_exit>` 출력 후 종료
- `task`가 있으면 → 다음 단계로

### 2. 구현

Task 정보를 확인하고 구현합니다:

- **task_id**: 작업 ID
- **title**: 작업 제목
- **dod**: Definition of Done (완료 기준)
- **scope**: 수정할 파일 범위
- **branch**: 작업 브랜치

**규칙**:
- DoD를 만족하도록 구현
- scope 범위 내 파일만 수정
- TDD 원칙 (테스트 먼저)

### 3. 검증

```
result = mcp__c4__c4_run_validation()
```

**실패 시**:
- 에러 분석 후 수정
- 재검증 (최대 10회)

**10회 초과 시**:
```
mcp__c4__c4_mark_blocked(task_id, failure_signature, 10)
```
→ `<loop_exit>BLOCKED: Supervisor 대기</loop_exit>` 출력 후 종료

### 4. 제출

모든 검증 통과 후:

```bash
git add .
git commit -m "feat(task_id): description"
```

```
result = mcp__c4__c4_submit(task_id, commit_sha, validation_results)
```

### 5. 다음 행동

`result.next_action` 확인:

- **`get_next_task`**: → `LOOP`로 돌아감 (자동)
- **`await_checkpoint`**: → 상태 폴링
- **`complete`**: → `<loop_exit>DONE: 프로젝트 완료</loop_exit>`

### 6. Checkpoint 대기 (await_checkpoint인 경우)

```
while True:
    status = mcp__c4__c4_status()
    if status.status == "EXECUTE":
        break  # → LOOP로 돌아감
    if status.status == "COMPLETE":
        → <loop_exit>DONE</loop_exit>
    if status.status == "PLAN":
        → <loop_exit>REPLAN: 재계획 필요</loop_exit>
    sleep(5초)
```

## CRITICAL

- **이 루프는 자동으로 계속됩니다**
- 수동 확인 없이 다음 task로 진행합니다
- `<loop_exit>` 태그가 나올 때까지 멈추지 않습니다
```

#### T-AUTO-09: `/c4-run` Skill 수정

**파일**: `.claude/commands/c4-run.md`

```markdown
# C4 Start Execution

PLAN → EXECUTE 상태 전환 후 Worker Loop를 자동 시작합니다.

## Instructions

1. 상태 확인: `mcp__c4__c4_status()`
2. PLAN 상태가 아니면 적절한 메시지 출력
3. PLAN 상태면:
   - `uv run c4 run` 실행하여 EXECUTE 전환
   - Supervisor Loop 자동 시작됨 (daemon background)
4. Worker Loop 자동 시작:
   - `/c4-worker` 스킬 실행과 동일한 루프 진입
   - 모든 task 완료 또는 BLOCKED까지 자동 진행

## Usage

```
/c4-run
```

실행 후 사람 개입 없이 자동으로 task들을 처리합니다.
```

### Phase 5: Safety & Testing (P1)

#### T-AUTO-10: Safety Guards 구현

**파일**: `c4/daemon/safety.py` (신규)

```python
MAX_ITERATIONS_PER_TASK = 10
MAX_TOTAL_ITERATIONS = 100
TASK_TIMEOUT_MINUTES = 30

class SafetyGuard:
    def __init__(self):
        self.iteration_count = 0
        self.task_iterations = {}

    def check_task_iterations(self, task_id: str) -> bool:
        count = self.task_iterations.get(task_id, 0)
        return count < MAX_ITERATIONS_PER_TASK

    def check_total_iterations(self) -> bool:
        return self.iteration_count < MAX_TOTAL_ITERATIONS

    def increment(self, task_id: str):
        self.iteration_count += 1
        self.task_iterations[task_id] = self.task_iterations.get(task_id, 0) + 1
```

#### T-AUTO-11: Permission 설정 템플릿

**파일**: `.claude/settings.json` (프로젝트용 템플릿)

```json
{
  "permissions": {
    "allow": [
      "mcp__c4__*",
      "Bash(uv run:*)",
      "Bash(uv run pytest:*)",
      "Bash(git add:*)",
      "Bash(git commit:*)",
      "Write(src/**)",
      "Write(tests/**)",
      "Edit(src/**)",
      "Edit(tests/**)"
    ]
  }
}
```

#### T-AUTO-12: 단위 테스트

**파일**: `tests/unit/test_queues.py` (신규)

- CheckpointQueue 동작 테스트
- RepairQueue 동작 테스트
- State 통합 테스트

#### T-AUTO-13: 통합 테스트

**파일**: `tests/integration/test_supervisor_loop.py` (신규)

- Supervisor Loop 시작/정지
- Checkpoint 처리 흐름
- Repair 처리 흐름

#### T-AUTO-14: E2E 테스트

**파일**: `tests/e2e/test_full_automation.py` (신규)

- 완전 자동화 시나리오
- Multi-worker 시나리오
- Error recovery 시나리오

---

## 4. 파일 변경 요약

### 4.1 신규 파일 (6개)

| 파일 | 설명 |
|------|------|
| `c4/models/queue.py` | Queue 모델 정의 |
| `c4/daemon/supervisor_loop.py` | Supervisor Loop 구현 |
| `c4/daemon/safety.py` | Safety Guards |
| `tests/unit/test_queues.py` | Queue 단위 테스트 |
| `tests/integration/test_supervisor_loop.py` | Loop 통합 테스트 |
| `tests/e2e/test_full_automation.py` | E2E 테스트 |

### 4.2 수정 파일 (4개)

| 파일 | 변경 내용 |
|------|----------|
| `c4/models/state.py` | checkpoint_queue, repair_queue 추가 |
| `c4/mcp_server.py` | 비동기 submit, supervisor loop 통합, mark_blocked |
| `.claude/commands/c4-worker.md` | 자동 루프 + 폴링 로직 |
| `.claude/commands/c4-run.md` | Worker Loop 자동 시작 |

---

## 5. 구현 순서

```
Day 1:
  T-AUTO-01: Queue 모델
  T-AUTO-02: State에 Queue 통합

Day 2:
  T-AUTO-03: c4_submit 비동기화
  T-AUTO-04: Supervisor Loop 구현

Day 3:
  T-AUTO-05: Daemon에 Loop 통합
  T-AUTO-06: Blocked → Repair Queue
  T-AUTO-07: c4_mark_blocked MCP Tool

Day 4:
  T-AUTO-08: /c4-worker Skill
  T-AUTO-09: /c4-run Skill
  T-AUTO-10: Safety Guards

Day 5:
  T-AUTO-11: Permission 템플릿
  T-AUTO-12~14: Tests
```

---

## 6. 검증 기준

### 6.1 자동화 시나리오

```bash
# 1. 초기화
cd my-project && claude
/c4-init my-project

# 2. 계획 (수동)
"3개의 task로 로그인 기능 구현해줘"
# → PLAN.md, tasks 생성

# 3. 실행 (자동)
/c4-run
# → 사람 개입 없이:
#    - Worker가 task 가져옴
#    - 구현 → 검증 → 제출
#    - 다음 task 자동 진행
#    - Checkpoint 도달 시 Supervisor Loop가 자동 처리
#    - 완료까지 반복

# 4. 확인
/c4-status
# → status: COMPLETE
```

### 6.2 성공 기준

- [ ] 사람 개입 없이 task 3개 이상 완료
- [ ] Checkpoint Queue → Supervisor Loop 자동 처리
- [ ] 검증 실패 시 자동 수정 (최소 1회)
- [ ] Blocked task → Repair Queue → Supervisor 가이드
- [ ] 최종 상태 COMPLETE 도달

---

## 7. 의사결정 (확정)

| 질문 | 결정 | 근거 |
|------|------|------|
| Q1: Supervisor 호출 시점 | **B (비동기)** | c4d가 orchestrator |
| Q2: Worker Loop 위치 | **B (Skill)** | Claude Code가 주체 |
| Q3: 에러 복구 | **C (Supervisor)** | repair 상태 + Supervisor |
| Q4: Permission | **A (사전 설정)** | 자동화 목적 |
| Q5: Multi-Worker | **Queue 기반** | Workers가 쌓고 Supervisor Loop가 처리 |

---

## 8. 리스크 & 대응

| 리스크 | 대응 |
|--------|------|
| Permission 요청으로 루프 중단 | 사전 permission 설정 필수 |
| 무한 루프 | Safety Guards (max iterations) |
| Supervisor 실패 | 재시도 3회 + 수동 fallback |
| 품질 저하 | Supervisor gate + repair queue |
| Race condition | Queue 기반 순차 처리 |

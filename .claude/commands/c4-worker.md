# C4 Worker - Ralph Loop (자동화)

자동으로 task를 받아 구현하고, 검증하고, 제출하는 루프를 실행합니다.
**이 루프는 수동 확인 없이 자동으로 계속됩니다.**

## ⚠️ 중요: MCP 도구만 사용

**CLI(bash) 명령어를 사용하지 마세요!** 반드시 MCP 도구를 사용하세요:
- `mcp__c4__c4_get_task(worker_id)` - 태스크 할당
- `mcp__c4__c4_run_validation()` - 검증 실행
- `mcp__c4__c4_submit(...)` - 태스크 제출
- `mcp__c4__c4_mark_blocked(...)` - 블록 마킹
- `mcp__c4__c4_status()` - 상태 확인

MCP 도구가 안 되면 Claude Code를 재시작하세요.

## Instructions

이 스킬이 호출되면 다음 루프를 **자동으로 반복**합니다.

### LOOP START

```
while True:
```

### 1. Task 가져오기

```
task = mcp__c4__c4_get_task("claude-worker")
```

**분기 처리:**
- `task`가 `null`이면 → "✅ COMPLETE: 모든 작업 완료" 출력 후 **루프 종료**
- `task`가 있으면 → 다음 단계로 진행

### 2. 구현

Task 정보를 확인하고 **DoD(Definition of Done)를 만족하도록** 구현합니다:

- **task_id**: 작업 ID
- **title**: 작업 제목
- **dod**: 완료 기준 (이것을 만족시켜야 함)
- **scope**: 수정할 파일 범위
- **branch**: 작업 브랜치

**규칙:**
- DoD를 정확히 만족하도록 구현
- scope 범위 내 파일만 수정
- TDD 원칙 적용 (테스트 먼저 또는 테스트와 함께)

### 3. 검증

```
result = mcp__c4__c4_run_validation()
```

**결과에 따른 분기:**

#### 실패 시 (retry_count < 10):
1. 에러 메시지 분석
2. 코드 수정
3. 재검증
4. `retry_count += 1`

#### 10회 초과 실패 시:
```
mcp__c4__c4_mark_blocked(
    task_id=task.task_id,
    worker_id="claude-worker",
    failure_signature="validation failed after 10 attempts",
    attempts=10,
    last_error="<마지막 에러 메시지>"
)
```
→ "⏸️ BLOCKED: Task가 repair queue에 추가됨. Supervisor 대기." 출력 후 **루프 종료**

### 4. 커밋 & 제출

모든 검증 통과 후:

```bash
git add .
git commit -m "feat(task_id): <task title 요약>"
```

```
result = mcp__c4__c4_submit(
    task_id=task.task_id,
    commit_sha="<커밋 SHA>",
    validation_results=[
        {"name": "lint", "status": "pass"},
        {"name": "unit", "status": "pass"}
    ]
)
```

### 5. 다음 행동 결정

`result.next_action` 값에 따라:

#### `get_next_task`:
→ **LOOP START로 돌아감** (자동)

#### `await_checkpoint`:
→ 상태 폴링 시작:

```python
while True:
    status = mcp__c4__c4_status()

    if status["status"] == "EXECUTE":
        # Checkpoint 처리됨, 계속 진행
        break  # → LOOP START로 돌아감

    if status["status"] == "COMPLETE":
        # 프로젝트 완료
        print("✅ DONE: 프로젝트 완료")
        exit()

    if status["status"] == "PLAN":
        # REPLAN 결정됨
        print("🔄 REPLAN: 재계획 필요")
        exit()

    # 아직 CHECKPOINT 상태 - 대기
    sleep(5초)
```

#### `complete`:
→ "✅ DONE: 프로젝트 완료" 출력 후 **루프 종료**

---

## CRITICAL - 자동화 규칙

1. **이 루프는 자동으로 계속됩니다** - 다음 task로 진행할 때 사용자 확인 없음
2. **검증 실패 시 자동으로 수정 시도** - 최대 10회
3. **Checkpoint 대기 중에도 자동 폴링** - Supervisor Loop가 처리할 때까지
4. **루프 종료 조건**:
   - 모든 task 완료 (null 반환)
   - 프로젝트 COMPLETE 상태
   - Task BLOCKED (10회 실패)
   - REPLAN 결정

---

## Usage

```
/c4-worker
```

실행 후 사람 개입 없이 자동으로 task들을 처리합니다.

---

## 예상 흐름

```
/c4-worker
→ Task T-001 할당
→ 구현...
→ 검증 pass
→ 제출 (next_action: get_next_task)
→ Task T-002 할당 (자동)
→ 구현...
→ 검증 fail
→ 수정 (retry 1)
→ 검증 pass
→ 제출 (next_action: await_checkpoint)
→ 폴링... (CHECKPOINT 상태)
→ 폴링... (EXECUTE로 변경됨)
→ Task T-003 할당 (자동)
→ ...
→ 제출 (next_action: complete)
→ ✅ DONE: 프로젝트 완료
```

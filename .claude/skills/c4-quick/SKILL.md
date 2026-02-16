---
description: |
  Quick start C4 workflow: create task + assign in one command. Handles state
  transitions (PLAN/HALTED → EXECUTE), creates task with DoD and validations,
  assigns to worker, and sets up isolated worktree. Use when user wants to
  quickly start working on a task without going through full planning. Triggers
  on: "quick start", "start task quickly", "/c4-quick", "create and assign task".
---

# C4 Quick Start

**태스크 생성 + 할당을 한 번에** 처리합니다.

C4를 빠르게 시작하기 위한 도구입니다.
작업 완료 후에는 `/c4-submit`으로 제출하세요.

## Usage

```
/c4-quick "작업 설명"
/c4-quick "버그 수정: connection timeout" scope=c4/daemon/
```

## Instructions

### 1. Worker ID 생성

```python
import uuid
WORKER_ID = f"quick-{uuid.uuid4().hex[:8]}"
```

### 2. 상태 확인

```python
status = mcp__c4__c4_status()
```

상태에 따른 처리:
- **PLAN/HALTED**: Step 3으로 (EXECUTE 전환)
- **EXECUTE**: Step 4로 (바로 태스크 생성)
- **INIT**: "먼저 /c4-init으로 초기화하세요." 출력 후 종료
- **CHECKPOINT**: "Checkpoint 대기 중입니다." 출력 후 종료
- **COMPLETE**: "프로젝트가 완료되었습니다." 출력 후 종료

### 3. PLAN/HALTED → EXECUTE 전환

```python
result = mcp__c4__c4_start()
```

### 4. 태스크 생성

`$ARGUMENTS`를 title로 사용:

```python
add_result = mcp__c4__c4_add_todo(
    title="$ARGUMENTS",
    scope=scope or None,  # scope 인자가 있으면 사용
    dod=f"- [ ] $ARGUMENTS 완료\n- [ ] 테스트 통과",
    validations=["lint", "unit"],
)
# add_result.task_id = "T-XXX-0"
```

### 5. 태스크 할당

```python
task = mcp__c4__c4_get_task(worker_id=WORKER_ID)
```

### 6. 결과 출력

```
✅ C4 Quick Start 완료

📋 태스크: {task.title}
🔢 ID: {task.task_id}
🌿 Branch: {task.branch}
📁 Worktree: {task.worktree_path}

⚠️ 이제 worktree에서 작업하세요:
   {task.worktree_path}

작업 완료 후:
   /c4-submit
```

## Example

```
/c4-quick "버그 수정: health check timeout"

→ T-001-0 생성 및 할당
→ Branch: c4/w-T-001-0
→ Worktree: .c4/worktrees/quick-abc123/

# 작업...
# Edit, Read 등 사용

/c4-submit
→ 검증 실행
→ 리뷰 태스크 생성
→ 완료
```

## Notes

- C4의 모든 가치(트래킹, 브랜칭, 검증, 리뷰) 유지
- 작업 후 반드시 `/c4-submit`으로 제출
- 멀티 세션에서도 안전 (worktree 격리)

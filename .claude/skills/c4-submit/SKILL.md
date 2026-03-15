---
name: c4-submit
description: |
  Submit completed C4 tasks with automated validation (lint, unit tests, DDD-CLEANCODE
  checks). Auto-detects in-progress tasks, runs required validators, verifies commit SHA,
  and triggers checkpoint reviews when needed. Use when task implementation is done.
  Triggers: "제출", "태스크 제출", "작업 완료", "submit task", "complete task",
  "submit T-XXX", "/c4-submit".
---

# C4 Submit Task

Submit the current in-progress task with validation and commit verification.

## Usage

```
/c4-submit [task-id]
```

If task-id is omitted, auto-detects the current in-progress task.

## Workflow

### 1. Check Current State

```python
status = mcp__c4__c4_status()
```

- Identify current worker's in_progress tasks
- If multiple, present list for user selection
- If `$ARGUMENTS` is empty, auto-detect and confirm

### 2. Run Validation

Auto-run before submission:
- **Basic**: lint + unit tests (always)
- **DDD-CLEANCODE**: boundary + work_breakdown + contract_spec (only if Worker Packet specs present)

See `references/ddd-validation.md` for DDD-CLEANCODE details.

On failure: report errors, block submission, offer to help fix.

### 3. Verify Commit SHA

```bash
commit_sha = git rev-parse HEAD
```

Confirm recent commit message and SHA with user.

### 4. Execute Submission

```python
mcp__c4__c4_submit(
    task_id=task_id,
    commit_sha=commit_sha,
    validation_results=[{"name": "lint", "status": "pass"}, ...],
    handoff=json.dumps({
        "summary": "구현 요약 (1-2줄)",
        "files_changed": ["path/to/file.go"],
        "discoveries": ["발견사항"],
        "concerns": ["우려사항"],
        "rationale": "설계 결정 이유"
    })
)
```

### 5. Handoff 구조 (CRITICAL)

| 필드 | 필수 | 설명 |
|------|------|------|
| `summary` | Y | 구현 요약 (1-2줄) |
| `files_changed` | Y | 변경된 파일 경로 목록 |
| `discoveries` | N | 구현 중 발견사항 (의존성, 사이드이펙트) |
| `concerns` | N | 잠재적 이슈 (버그, 성능) |
| `rationale` | N | 설계 결정 이유 |

handoff 포함 시 Go 핸들러(`autoRecordKnowledge`)가 자동으로 knowledge 기록.

### 6. Post-Submit: Continue to Next Task (CRITICAL)

제출 성공 직후 `c4_status()` 호출:
- `EXECUTE` + `ready_now > 0` → **자동으로 `/c4-run 1` 실행** (묻지 않음)
- `CHECKPOINT` → 체크포인트 안내만 출력
- `COMPLETE` → 완료 안내만 출력

## After Submission

System automatically:
- Marks task as completed
- Releases scope lock
- Checks checkpoint conditions
- Triggers Supervisor review if needed

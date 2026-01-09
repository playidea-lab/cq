# C4 Start Execution (자동화)

PLAN → EXECUTE 상태 전환 후 **Worker Loop를 자동으로 시작**합니다.

## 0. Worker ID 생성 (필수!)

**루프 시작 전** 고유한 worker_id를 생성하세요:

```python
import uuid
WORKER_ID = f"worker-{uuid.uuid4().hex[:8]}"  # 예: "worker-a1b2c3d4"
```

이 ID를 이 세션 전체에서 사용합니다. **절대로 "claude-worker" 같은 고정값 사용 금지!**

## ⚠️ 중요: MCP 도구만 사용

**CLI(bash) 명령어를 사용하지 마세요!** 반드시 MCP 도구를 사용하세요:
- `mcp__c4__c4_status()` - 상태 확인
- `mcp__c4__c4_start()` - PLAN/HALTED → EXECUTE 전환
- `mcp__c4__c4_get_task(worker_id)` - 태스크 할당
- `mcp__c4__c4_submit(task_id, commit_sha, validation_results)` - 태스크 제출
- `mcp__c4__c4_run_validation(names)` - 검증 실행

MCP 도구가 안 되면 Claude Code를 재시작하세요.

## ⚡ Accept Edits 모드 확인

자동화 작업 전에 **Accept Edits** 모드가 켜져 있는지 확인하세요.
파일 편집마다 권한 요청이 뜨면 자동화가 중단됩니다.

**확인 방법:**
- 화면 하단 상태바에서 "Accept Edits" 표시 확인
- 또는 `Shift+Tab` 눌러서 활성화

**설정으로 기본값 지정** (`.claude/settings.json`):
```json
{
  "permissions": {
    "defaultMode": "acceptEdits"
  }
}
```

⚠️ Accept Edits가 꺼져있으면 매 파일 수정마다 승인 필요 → 자동화 불가!

## Instructions

### 1. 상태 확인

```
status = mcp__c4__c4_status()
```

상태에 따른 처리:

- **EXECUTE**: "이미 실행 중입니다. /c4-worker로 작업을 시작하세요."
- **CHECKPOINT**: "Checkpoint 리뷰 대기 중입니다."
- **COMPLETE**: "프로젝트가 완료되었습니다."
- **INIT**: "먼저 /c4-plan으로 계획을 수립하세요."

### 2. PLAN 또는 HALTED 상태인 경우

MCP 도구로 상태 전환:

```
result = mcp__c4__c4_start()
```

성공 시 `result.success == true`, `result.status == "EXECUTE"`

### 3. 상태 확인

```
status = mcp__c4__c4_status()
```

전환 성공 시:
- 새로운 상태 표시
- 대기 중인 task 수 표시

### 4. Worker Loop 자동 시작

상태 전환 성공 후 **즉시** Worker Ralph Loop를 시작합니다.

이는 `/c4-worker` 스킬과 동일한 루프입니다:

```
LOOP:
  task = c4_get_task(WORKER_ID)
  if task is null:
      exit("✅ COMPLETE")

  implement(task)
  validate()
  if fail_count >= 10:
      mark_blocked(task)
      exit("⏸️ BLOCKED")

  commit()
  result = submit(task)

  if result.next_action == "get_next_task":
      continue LOOP
  elif result.next_action == "await_checkpoint":
      poll until EXECUTE or exit
  elif result.next_action == "complete":
      exit("✅ DONE")
```

## Usage

```
/c4-run
```

실행 후 **사람 개입 없이** 자동으로 모든 task를 처리합니다.

## 예상 흐름

```
/c4-run
→ 상태 확인: PLAN
→ mcp__c4__c4_start()로 EXECUTE 전환
→ Worker Loop 시작 (자동)
→ Task T-001 할당...
→ 구현... 검증... 제출...
→ Task T-002 할당 (자동)...
→ ...
→ Checkpoint 대기... Supervisor 처리...
→ ...
→ ✅ DONE: 프로젝트 완료
```

## 중요

- `/c4-run` 실행 후에는 **루프가 종료될 때까지** 자동으로 진행됩니다
- Supervisor Loop는 c4d daemon에서 백그라운드로 실행됩니다
- Worker는 checkpoint 대기 중에도 폴링하며 대기합니다

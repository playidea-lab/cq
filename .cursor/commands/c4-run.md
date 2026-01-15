<!--
  Platform: Cursor
  Based on: Claude Code version

  TODO: Cursor 특화 수정 필요시 여기에 기록
-->

# C4 Run (자동화)

**Worker Loop를 실행**합니다. 상태에 따라 자동 처리:
- PLAN/HALTED → EXECUTE 전환 후 작업 시작
- EXECUTE → 바로 작업 참여 (멀티 워커 지원)

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

- **PLAN/HALTED**: → Step 2로 (EXECUTE 전환)
- **EXECUTE**: → Step 3으로 (바로 Worker Loop 시작)
- **CHECKPOINT**: "Checkpoint 리뷰 대기 중입니다." 출력 후 종료
- **COMPLETE**: "프로젝트가 완료되었습니다." 출력 후 종료
- **INIT**: "먼저 /c4-plan으로 계획을 수립하세요." 출력 후 종료

### 2. PLAN 또는 HALTED 상태인 경우

MCP 도구로 상태 전환:

```
result = mcp__c4__c4_start()
```

성공 시 `result.success == true`, `result.status == "EXECUTE"`

### 3. Worker Loop 시작

EXECUTE 상태에서 Worker Loop를 시작합니다:

```
LOOP:
  task = c4_get_task(WORKER_ID)
  if task is null:
      exit("✅ COMPLETE")

  implement_with_agent_routing(task)  # ← Phase 4: Agent Routing
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

## 🤖 Agent Routing (Phase 4)

`c4_get_task()` 응답에 에이전트 라우팅 정보가 포함됩니다:

```python
task = c4_get_task(WORKER_ID)
# task.recommended_agent: "frontend-developer"   ← 사용할 에이전트
# task.agent_chain: ["frontend-developer", "test-automator", "code-reviewer"]
# task.domain: "web-frontend"
# task.handoff_instructions: "Pass component specs and test requirements..."
```

### 사용 방법 (자동)

**Worker(Claude)가 자동으로 판단합니다.** 사용자 개입 불필요.

```python
# Worker가 태스크를 받으면:
task = c4_get_task(WORKER_ID)

# 추천 에이전트로 구현
Task(subagent_type=task.recommended_agent, prompt=f"""
    Task: {task.title}
    DoD: {task.dod}
    Scope: {task.scope}
    Branch: {task.branch}

    Implement this task completely, following the DoD.
""")
```

**기본 동작**:
1. MCP가 도메인 기반으로 `recommended_agent` 제공
2. Worker가 해당 에이전트로 Task 실행
3. 필요시 `agent_chain`의 추가 에이전트 활용

### 언제 agent_chain을 사용하는가?

Worker가 다음 조건에서 **자동으로** 체인을 활용:

| 조건 | 동작 |
|------|------|
| 구현 후 테스트 실패 | `test-automator` 호출 |
| 코드 품질 이슈 발견 | `code-reviewer` 호출 |
| 보안 관련 코드 | `security-auditor` 추가 |
| 디버깅 필요 | `debugger` 사용 |

```python
# 예시: 구현 후 테스트 실패 시
result = Task(subagent_type=task.recommended_agent, ...)

if validation_failed:
    # 체인에서 test-automator 찾아서 호출
    Task(subagent_type="test-automator", prompt=f"""
        Fix failing tests for: {task.title}
        Error: {validation_error}
    """)
```

### 도메인별 추천 에이전트

| Domain | Primary Agent | 추가 Chain |
|--------|--------------|-----------|
| web-frontend | `frontend-developer` | test → reviewer |
| web-backend | `backend-architect` | python → test → reviewer |
| fullstack | `backend-architect` | frontend → test → reviewer |
| ml-dl | `ml-engineer` | python → test |
| mobile-app | `mobile-developer` | test → reviewer |
| infra | `cloud-architect` | deployment |
| library | `python-pro` | docs → test → reviewer |
| unknown | `general-purpose` | reviewer |

### Override 케이스 (특수 상황)

Worker가 추천 대신 다른 에이전트 선택하는 경우:

```python
# 디버깅 태스크
if "debug" in task.title.lower() or "fix bug" in task.title.lower():
    agent = "debugger"  # 추천 무시, debugger 사용

# 성능 최적화
elif "performance" in task.dod.lower() or "optimize" in task.dod.lower():
    agent = "performance-engineer"

# 보안 민감
elif "auth" in task.title.lower() or "security" in task.dod.lower():
    agent = task.recommended_agent
    # + 리뷰 단계에서 security-auditor 추가

else:
    agent = task.recommended_agent  # 기본: 추천 사용
```

**핵심**: 모든 판단은 Worker(Claude)가 자동으로 수행. 사용자는 `/c4-run` 실행만 하면 됩니다.

## Usage

```
/c4-run
```

실행 후 **사람 개입 없이** 자동으로 모든 task를 처리합니다.

## 예상 흐름

### 첫 번째 워커 (PLAN 상태에서)
```
/c4-run
→ 상태 확인: PLAN
→ mcp__c4__c4_start()로 EXECUTE 전환
→ Worker Loop 시작
→ Task T-001 할당...
→ 구현... 검증... 제출...
→ ✅ DONE: 프로젝트 완료
```

### 추가 워커 (이미 EXECUTE 상태)
```
/c4-run
→ 상태 확인: EXECUTE
→ Worker Loop 바로 시작 (전환 없음)
→ Task T-002 할당...
→ 구현... 검증... 제출...
→ ✅ DONE: 작업 완료
```

## 멀티 워커

여러 Claude Code 창에서 동시에 `/c4-run` 실행 가능:
- 첫 번째 창: PLAN → EXECUTE 전환 + 작업
- 추가 창들: 바로 작업 참여

SQLite WAL 모드로 race condition 없이 안전하게 동작합니다.

## 중요

- `/c4-run` 실행 후에는 **루프가 종료될 때까지** 자동으로 진행됩니다
- Supervisor Loop는 백그라운드로 실행됩니다
- Worker는 checkpoint 대기 중에도 폴링하며 대기합니다

# MCP 도구 레퍼런스

이 문서는 C4 MCP 서버가 제공하는 저수준 도구(tools)의 상세 스펙을 설명합니다.

## 개요

C4 MCP 서버는 다음 9개의 도구를 제공합니다:

| 도구 | 설명 | 주요 용도 |
|------|------|-----------|
| `c4_status` | 프로젝트 상태 조회 | 현재 상태 확인 |
| `c4_start` | PLAN/HALTED → EXECUTE 전환 | 실행 시작 |
| `c4_get_task` | 태스크 할당 요청 (+ 에이전트 라우팅) | Worker가 작업 받기 |
| `c4_submit` | 태스크 완료 보고 | 작업 결과 제출 |
| `c4_add_todo` | 태스크 추가 | REQUEST_CHANGES 시 |
| `c4_checkpoint` | 체크포인트 결정 | Supervisor 리뷰 |
| `c4_run_validation` | 검증 실행 | lint, test 등 |
| `c4_mark_blocked` | 블로킹 보고 | 재시도 실패 시 |
| `c4_clear` | 상태 초기화 | 개발/디버깅용 |

---

## c4_status

프로젝트 상태를 조회합니다.

### 파라미터

없음

### 응답

```json
{
  "project_id": "my-project",
  "status": "EXECUTE",
  "queue": {
    "pending": 3,
    "in_progress": 1,
    "completed": 5
  },
  "workers": {
    "active": ["worker-001"],
    "idle": []
  },
  "checkpoint_queue": [],
  "repair_queue": [],
  "passed_checkpoints": ["CP-001"],
  "last_updated": "2025-01-08T10:30:00"
}
```

### 필드 설명

| 필드 | 타입 | 설명 |
|------|------|------|
| `project_id` | string | 프로젝트 식별자 |
| `status` | string | 현재 상태 (INIT, PLAN, EXECUTE, CHECKPOINT, HALTED, COMPLETE) |
| `queue` | object | 태스크 상태별 개수 |
| `workers` | object | 워커 상태 목록 |
| `checkpoint_queue` | array | 대기 중인 체크포인트 |
| `repair_queue` | array | 수리 대기 태스크 |
| `passed_checkpoints` | array | 통과한 체크포인트 ID 목록 |
| `last_updated` | string | 마지막 업데이트 시간 (ISO 8601) |

### 예시

```python
# Claude Code 내부에서
result = mcp.call_tool("c4_status", {})
print(f"Status: {result['status']}")
print(f"Pending tasks: {result['queue']['pending']}")
```

---

## c4_start

PLAN 또는 HALTED 상태에서 EXECUTE 상태로 전환합니다.

### 파라미터

없음

### 응답

```json
{
  "success": true,
  "status": "EXECUTE",
  "message": "Execution started"
}
```

### 응답 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `success` | boolean | 전환 성공 여부 |
| `status` | string | 전환 후 상태 |
| `message` | string | 결과 메시지 |

### 동작

1. 현재 상태가 PLAN 또는 HALTED인지 확인
2. EXECUTE 상태로 전환
3. Supervisor Loop 자동 시작
4. 결과 반환

### 예시

```python
# Worker Loop 시작 전
result = mcp.call_tool("c4_start", {})
if result["success"]:
    print("Execution started!")
    # Worker Loop 진입
```

---

## c4_get_task

Worker가 다음 태스크 할당을 요청합니다.

### 파라미터

| 이름 | 타입 | 필수 | 설명 |
|------|------|------|------|
| `worker_id` | string | O | Worker의 고유 식별자 |

### 응답

**태스크가 있는 경우:**

```json
{
  "task_id": "T-003",
  "title": "API 엔드포인트 구현",
  "scope": "src/api",
  "dod": "GET /users 엔드포인트 구현 및 테스트",
  "validations": ["lint", "unit"],
  "branch": "c4/w-abc123/T-003",
  "recommended_agent": "backend-architect",
  "agent_chain": ["backend-architect", "python-pro", "test-automator", "code-reviewer"],
  "domain": "web-backend",
  "handoff_instructions": "Pass API specs and validation requirements to next agent..."
}
```

**태스크가 없는 경우:**

```json
null
```

### 응답 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `task_id` | string | 태스크 ID |
| `title` | string | 태스크 제목 |
| `scope` | string | 작업 범위 (Scope Lock 대상) |
| `dod` | string | Definition of Done (완료 조건) |
| `validations` | array | 실행할 검증 명령어 목록 |
| `branch` | string | 작업용 Git 브랜치 |
| `recommended_agent` | string | 추천 에이전트 (Phase 4) |
| `agent_chain` | array | 에이전트 체인 (Phase 4) |
| `domain` | string | 태스크 도메인 (Phase 4) |
| `handoff_instructions` | string | 에이전트 간 핸드오프 지침 (Phase 4) |

### 동작

1. `worker_id`로 Worker 등록/갱신
2. 할당 가능한 태스크 검색:
   - `pending` 상태
   - 모든 `dependencies` 완료됨
   - `scope`가 다른 Worker에 의해 잠기지 않음
3. 태스크의 `scope`에 대해 Lock 획득
4. 태스크 상태를 `in_progress`로 변경
5. **에이전트 라우팅 정보 계산 (Phase 4)**:
   - 태스크 도메인 또는 프로젝트 도메인 확인
   - 도메인에 맞는 추천 에이전트 결정
   - 에이전트 체인 및 핸드오프 지침 생성
6. `TaskAssignment` 반환

### 예시

```python
# Worker Loop에서
assignment = mcp.call_tool("c4_get_task", {"worker_id": "worker-001"})
if assignment:
    task_id = assignment["task_id"]
    dod = assignment["dod"]

    # Phase 4: 에이전트 라우팅 활용
    agent = assignment["recommended_agent"]
    chain = assignment["agent_chain"]

    # 추천 에이전트로 작업 수행
    Task(subagent_type=agent, prompt=f"""
        Task: {assignment['title']}
        DoD: {dod}
        Branch: {assignment['branch']}
    """)
```

---

## c4_submit

태스크 완료를 보고합니다.

### 파라미터

| 이름 | 타입 | 필수 | 설명 |
|------|------|------|------|
| `task_id` | string | O | 완료한 태스크 ID |
| `commit_sha` | string | O | 작업 커밋의 Git SHA |
| `validation_results` | array | O | 검증 결과 목록 |

**validation_results 구조:**

```json
[
  {
    "name": "lint",
    "status": "pass",
    "message": "No issues found"
  },
  {
    "name": "unit",
    "status": "pass",
    "message": "10 tests passed"
  }
]
```

### 응답

```json
{
  "status": "completed",
  "task_id": "T-003",
  "message": "Task completed successfully",
  "checkpoint_triggered": null
}
```

또는 체크포인트 트리거 시:

```json
{
  "status": "completed",
  "task_id": "T-003",
  "message": "Task completed, checkpoint triggered",
  "checkpoint_triggered": "CP-001"
}
```

### 응답 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `status` | string | 제출 결과 (completed, validation_failed) |
| `task_id` | string | 제출한 태스크 ID |
| `message` | string | 결과 메시지 |
| `checkpoint_triggered` | string | 트리거된 체크포인트 ID (없으면 null) |

### 동작

1. 검증 결과 확인 (모두 `pass`인지)
2. 태스크 상태를 `completed`로 변경
3. Scope Lock 해제
4. Gate 조건 확인 (체크포인트 트리거 여부)
5. 이벤트 기록

### 예시

```python
# 작업 완료 후
result = mcp.call_tool("c4_submit", {
    "task_id": "T-003",
    "commit_sha": "abc123",
    "validation_results": [
        {"name": "lint", "status": "pass"},
        {"name": "unit", "status": "pass"}
    ]
})

if result["checkpoint_triggered"]:
    print(f"Checkpoint {result['checkpoint_triggered']} triggered!")
```

---

## c4_add_todo

새 태스크를 큐에 추가합니다.

### 파라미터

| 이름 | 타입 | 필수 | 설명 |
|------|------|------|------|
| `task_id` | string | O | 태스크 ID |
| `title` | string | O | 태스크 제목 |
| `scope` | string | X | 작업 범위 |
| `dod` | string | O | Definition of Done |

### 응답

```json
{
  "status": "success",
  "task_id": "T-NEW-001",
  "message": "Task added to queue"
}
```

### 동작

1. 새 태스크 생성 (`pending` 상태)
2. 태스크 큐에 추가
3. 상태 저장

### 사용 시나리오

**REQUEST_CHANGES 처리:**

```python
# Supervisor가 변경 요청 시
for change in required_changes:
    mcp.call_tool("c4_add_todo", {
        "task_id": f"T-FIX-{idx}",
        "title": f"Fix: {change[:50]}",
        "dod": change
    })
```

**런타임 태스크 추가:**

```python
# 작업 중 추가 태스크 발견
mcp.call_tool("c4_add_todo", {
    "task_id": "T-EXTRA-001",
    "title": "추가 발견된 버그 수정",
    "scope": "src/utils",
    "dod": "utils.py의 edge case 처리"
})
```

---

## c4_checkpoint

Supervisor의 체크포인트 결정을 기록합니다.

### 파라미터

| 이름 | 타입 | 필수 | 설명 |
|------|------|------|------|
| `checkpoint_id` | string | O | 체크포인트 ID |
| `decision` | string | O | 결정 (APPROVE, REQUEST_CHANGES, REPLAN) |
| `notes` | string | O | 리뷰 코멘트 |
| `required_changes` | array | X | 요청 변경사항 (REQUEST_CHANGES 시) |

### 응답

```json
{
  "checkpoint_id": "CP-001",
  "decision": "APPROVE",
  "notes": "모든 기준을 충족합니다.",
  "next_status": "EXECUTE"
}
```

### 응답 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `checkpoint_id` | string | 처리한 체크포인트 ID |
| `decision` | string | 결정 |
| `notes` | string | 리뷰 노트 |
| `next_status` | string | 다음 프로젝트 상태 |

### 결정별 동작

**APPROVE:**
- 체크포인트를 `passed_checkpoints`에 추가
- EXECUTE 상태로 전환
- 다음 태스크 작업 계속

**REQUEST_CHANGES:**
- `required_changes`를 새 태스크로 추가
- EXECUTE 상태로 전환
- 수정 태스크 작업

**REPLAN:**
- PLAN 상태로 전환
- 전체 계획 재수립 필요

### 예시

```python
# APPROVE
mcp.call_tool("c4_checkpoint", {
    "checkpoint_id": "CP-001",
    "decision": "APPROVE",
    "notes": "코드 품질이 좋습니다. 테스트 커버리지 충분합니다."
})

# REQUEST_CHANGES
mcp.call_tool("c4_checkpoint", {
    "checkpoint_id": "CP-001",
    "decision": "REQUEST_CHANGES",
    "notes": "몇 가지 수정이 필요합니다.",
    "required_changes": [
        "에러 핸들링 추가 필요",
        "테스트 커버리지 80% 이상으로"
    ]
})

# REPLAN
mcp.call_tool("c4_checkpoint", {
    "checkpoint_id": "CP-001",
    "decision": "REPLAN",
    "notes": "설계 변경이 필요합니다. 재계획 필요."
})
```

---

## c4_run_validation

검증 명령을 실행하고 결과를 반환합니다.

### 파라미터

| 이름 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `names` | array | X | 전체 | 실행할 검증 이름 목록 |
| `fail_fast` | boolean | X | true | 첫 실패 시 중단 |
| `timeout` | integer | X | 300 | 검증당 타임아웃 (초) |

### 응답

```json
{
  "results": [
    {
      "name": "lint",
      "status": "pass",
      "message": "No issues found",
      "duration": 2.5
    },
    {
      "name": "unit",
      "status": "pass",
      "message": "25 tests passed",
      "duration": 15.3
    }
  ],
  "all_passed": true,
  "total_duration": 17.8
}
```

### 응답 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `results` | array | 각 검증 결과 목록 |
| `all_passed` | boolean | 모든 검증 통과 여부 |
| `total_duration` | number | 총 소요 시간 (초) |

### 기본 검증 명령

`.c4/config.yaml`에서 정의:

```yaml
validations:
  lint:
    command: "uv run ruff check"
    description: "코드 스타일 검사"
  unit:
    command: "uv run pytest tests/unit"
    description: "단위 테스트"
  integration:
    command: "uv run pytest tests/integration"
    description: "통합 테스트"
```

### 예시

```python
# 모든 검증 실행
result = mcp.call_tool("c4_run_validation", {})

# 특정 검증만 실행
result = mcp.call_tool("c4_run_validation", {
    "names": ["lint", "unit"]
})

# 타임아웃 조정
result = mcp.call_tool("c4_run_validation", {
    "names": ["e2e"],
    "timeout": 600,  # 10분
    "fail_fast": False
})
```

---

## c4_mark_blocked

태스크를 블로킹 상태로 표시하고 수리 큐에 추가합니다.

### 파라미터

| 이름 | 타입 | 필수 | 설명 |
|------|------|------|------|
| `task_id` | string | O | 블로킹된 태스크 ID |
| `worker_id` | string | O | 작업 중이던 Worker ID |
| `failure_signature` | string | O | 에러 시그니처 |
| `attempts` | integer | O | 시도 횟수 |
| `last_error` | string | X | 마지막 에러 메시지 |

### 응답

```json
{
  "status": "blocked",
  "task_id": "T-003",
  "message": "Task added to repair queue",
  "repair_queue_size": 1
}
```

### 동작

1. 태스크 상태를 `blocked`로 변경
2. Scope Lock 해제
3. 수리 큐(repair_queue)에 추가
4. Supervisor 리뷰 대기

### 사용 시나리오

Worker가 최대 재시도 후에도 검증 실패 시:

```python
MAX_RETRIES = 3

for attempt in range(MAX_RETRIES):
    # 작업 시도...
    result = run_validations()
    if result["all_passed"]:
        break
else:
    # 모든 재시도 실패
    mcp.call_tool("c4_mark_blocked", {
        "task_id": task_id,
        "worker_id": worker_id,
        "failure_signature": "test_api_endpoint: AssertionError",
        "attempts": MAX_RETRIES,
        "last_error": "Expected 200, got 500"
    })
```

---

## c4_clear

C4 상태를 완전히 초기화합니다. 개발 및 디버깅용.

### 파라미터

| 이름 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `confirm` | boolean | O | - | 삭제 확인 (true 필수) |
| `keep_config` | boolean | X | false | config.yaml 유지 여부 |

### 응답

```json
{
  "success": true,
  "message": "C4 state cleared"
}
```

### 동작

1. `confirm`이 true인지 확인
2. `.c4/` 디렉토리 삭제
3. 데몬 캐시 초기화
4. `keep_config`가 true면 config.yaml 복원

### 예시

```python
# 완전 초기화
mcp.call_tool("c4_clear", {"confirm": True})

# 설정 유지하고 초기화
mcp.call_tool("c4_clear", {"confirm": True, "keep_config": True})
```

---

## 에러 처리

모든 도구는 에러 발생 시 다음 형식으로 응답합니다:

```json
{
  "error": "에러 메시지"
}
```

### 일반적인 에러

| 에러 | 원인 | 해결 |
|------|------|------|
| `C4 not initialized` | 프로젝트 미초기화 | `/c4-init` 실행 |
| `Invalid transition` | 잘못된 상태 전이 | 현재 상태 확인 |
| `Task not found` | 존재하지 않는 태스크 | 태스크 ID 확인 |
| `Scope locked` | 다른 Worker가 작업 중 | 대기 또는 다른 태스크 |
| `Validation failed` | 검증 명령 실패 | 코드 수정 후 재시도 |

---

## 슬래시 명령어와의 관계

MCP 도구는 저수준 API이며, 슬래시 명령어는 이를 감싸는 고수준 인터페이스입니다:

| 슬래시 명령어 | MCP 도구 호출 |
|--------------|---------------|
| `/c4-status` | `c4_status` |
| `/c4-run` | 상태 전이 + `c4_status` |
| `/c4-worker join` | `c4_get_task` 반복 |
| `/c4-worker submit` | `c4_run_validation` + `c4_submit` |
| `/c4-validate` | `c4_run_validation` |
| `/c4-checkpoint` | `c4_checkpoint` |

### 직접 사용 vs 슬래시 명령어

**슬래시 명령어 사용 (권장):**
- 일반적인 워크플로우
- 자동화된 루프
- 사용자 친화적 출력

**MCP 도구 직접 사용:**
- 커스텀 자동화 스크립트
- 세밀한 제어 필요 시
- 다른 시스템과 통합

---

## 다음 단계

- [명령어 레퍼런스](../user-guide/명령어-레퍼런스.md) - 슬래시 명령어 상세
- [아키텍처](../developer-guide/아키텍처.md) - 시스템 구조
- [워크플로우 개요](../user-guide/워크플로우-개요.md) - 전체 흐름

# C4 Swarm - 병렬 Worker 스폰

**N개의 Worker를 Task tool subagent로 병렬 실행**합니다.

Claude Code의 Task tool을 활용하여 최대 7개의 Worker를 동시에 스폰하고, 태스크 큐를 병렬로 소비합니다.

## Usage

```
/c4-swarm [N]
```

- **N**: Worker 수 (기본: pending 태스크 수, 최대: 7)

## Instructions

### 1. 상태 확인

```python
status = mcp__c4__c4_status()

if status.state not in ["EXECUTE", "PLAN", "HALTED"]:
    print(f"❌ 현재 상태 {status.state}에서는 실행할 수 없습니다.")
    exit()

# PLAN/HALTED면 EXECUTE로 전환
if status.state in ["PLAN", "HALTED"]:
    mcp__c4__c4_start()

pending_count = status.queue.pending
N = min(pending_count, 7)  # 최대 7개

if N == 0:
    print("⚠️ 실행 가능한 태스크가 없습니다.")
    exit()
```

### 2. Worker 스폰

```python
import uuid

WORKER_PROMPT = """
You are C4 Worker {worker_id}.

## Mission
Execute C4 tasks autonomously using MCP tools until no tasks remain.

## MCP Tools (MUST USE)
- `mcp__c4__c4_get_task(worker_id="{worker_id}")` - 태스크 할당
- `mcp__c4__c4_run_validation(names=["lint", "unit"])` - 검증
- `mcp__c4__c4_submit(task_id, worker_id, commit_sha, validation_results)` - 제출

## Worker Loop
1. task = c4_get_task(worker_id="{worker_id}")
2. if no task: exit
3. Implement following DoD
4. Run validations, fix issues (max 3 retries)
5. git commit
6. c4_submit()
7. Go to step 1

## Your Worker ID: {worker_id}

START: Call `mcp__c4__c4_get_task(worker_id="{worker_id}")`
"""

workers = []
for i in range(N):
    worker_id = f"swarm-{uuid.uuid4().hex[:8]}"

    result = Task(
        subagent_type="general-purpose",
        description=f"C4 Worker {i+1}/{N}",
        prompt=WORKER_PROMPT.format(worker_id=worker_id),
        run_in_background=True
    )

    workers.append({"id": worker_id, "output": result.output_file})
    print(f"🚀 Worker {i+1}/{N} spawned: {worker_id}")
```

### 3. 결과 출력

```
🐝 C4 Swarm: {N} workers spawned

Workers:
  • {worker_id_1}: {output_file_1}
  • {worker_id_2}: {output_file_2}
  ...

Monitor:
  /c4-status - 전체 진행 상황
  tail -f {output_file} - 개별 Worker 로그
```

## 제약사항

| 제약 | 설명 |
|------|------|
| 최대 Worker | 7개 |
| Subagent 중첩 | 불가 |

## 관련 명령어

- `/c4-status` - 상태 확인
- `/c4-run` - 단일 Worker 실행
- `/c4-stop` - 실행 중지

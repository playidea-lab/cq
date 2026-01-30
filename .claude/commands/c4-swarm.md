# C4 Swarm - 병렬 Worker 스폰

**N개의 Worker를 Task tool subagent로 병렬 실행**합니다.

Claude Code의 Task tool을 활용하여 최대 7개의 Worker를 동시에 스폰하고, 태스크 큐를 병렬로 소비합니다.

## Economic Mode

태스크별 `model` 필드를 지원합니다. Worker는 자신의 모델에 맞는 태스크만 처리합니다.

- **sonnet**: 빠르고 저렴한 모델 (단순 구현 태스크)
- **opus**: 고성능 모델 (복잡한 구현, 리뷰, 체크포인트)
- **haiku**: 가장 빠른 모델 (간단한 작업)

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
if pending_count == 0:
    print("⚠️ 실행 가능한 태스크가 없습니다.")
    exit()
```

### 2. 모델별 태스크 그룹화 (Economic Mode)

```python
from collections import defaultdict

# pending 태스크를 모델별로 그룹화
model_groups = defaultdict(list)

for task_id in status.queue.pending_ids:
    # 각 태스크의 model 정보 확인 (status에 포함됨)
    task_info = next(
        (t for t in status.queue.tasks if t["id"] == task_id),
        None
    )
    if task_info:
        model = task_info.get("model", "opus")  # 기본값 opus
        model_groups[model].append(task_id)

print(f"📊 Pending tasks by model:")
for model, task_ids in model_groups.items():
    print(f"   {model}: {len(task_ids)} ({', '.join(task_ids[:3])}{'...' if len(task_ids) > 3 else ''})")
```

### 3. 모델별 Worker 스폰

```python
import uuid

WORKER_PROMPT = """
You are C4 Worker {worker_id}.

## Mission
Execute C4 tasks with model={model_filter} until no tasks remain.

## MCP Tools (MUST USE)
- `mcp__c4__c4_get_task(worker_id="{worker_id}", model_filter="{model_filter}")` - 내 모델 태스크만 요청
- `mcp__c4__c4_run_validation(names=["lint", "unit"])` - 검증
- `mcp__c4__c4_submit(task_id, worker_id, commit_sha, validation_results)` - 제출

## Worker Loop
1. task = c4_get_task(worker_id="{worker_id}", model_filter="{model_filter}")
2. if no task: exit (다른 모델 태스크만 남음)
3. Implement following DoD
4. Run validations, fix issues (max 3 retries)
5. git commit
6. c4_submit()
7. Go to step 1

## Your Worker ID: {worker_id}
## Your Model Filter: {model_filter}

START: Call `mcp__c4__c4_get_task(worker_id="{worker_id}", model_filter="{model_filter}")`
"""

workers = []
total_spawned = 0

for model, task_ids in model_groups.items():
    # 모델당 Worker 수 결정 (태스크 수 또는 최대 3개)
    worker_count = min(len(task_ids), 3)

    for i in range(worker_count):
        if total_spawned >= 7:  # 전체 최대 7개
            break

        worker_id = f"{model[:3]}-{uuid.uuid4().hex[:8]}"

        result = Task(
            subagent_type="general-purpose",
            description=f"C4 {model.title()} Worker {i+1}/{worker_count}",
            prompt=WORKER_PROMPT.format(worker_id=worker_id, model_filter=model),
            model=model,  # Worker 모델 = 태스크 모델
            run_in_background=True
        )

        workers.append({"id": worker_id, "model": model, "output": result.output_file})
        total_spawned += 1
        print(f"🚀 {model.title()} Worker {i+1}/{worker_count} spawned: {worker_id}")

    if total_spawned >= 7:
        break
```

### 4. 결과 출력

```
🐝 C4 Swarm: {N} workers spawned (Economic Mode)

📊 Workers by model:
  Sonnet: {count} workers
  Opus: {count} workers

Workers:
  • {worker_id_1} (sonnet): {output_file_1}
  • {worker_id_2} (opus): {output_file_2}
  ...

Monitor:
  /c4-status - 전체 진행 상황
  tail -f {output_file} - 개별 Worker 로그
```

## 비용 최적화

| 모델 | 상대 비용 | 권장 용도 |
|------|----------|----------|
| haiku | 0.2x | 간단한 수정, 문서화 |
| sonnet | 1x | 일반 구현 태스크 |
| opus | 5x | 복잡한 구현, 리뷰, 체크포인트 |

## 제약사항

| 제약 | 설명 |
|------|------|
| 최대 Worker | 7개 (모델 합산) |
| 모델당 최대 | 3개 |
| Subagent 중첩 | 불가 |

## 관련 명령어

- `/c4-status` - 상태 확인
- `/c4-run` - 단일 Worker 실행
- `/c4-stop` - 실행 중지

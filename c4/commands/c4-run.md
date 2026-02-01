# C4 Run (Smart Auto Mode)

**의존성 그래프를 분석하여 최적의 Worker 수를 자동 계산하고 실행**합니다.

## Usage

```
/c4-run           # 자동: 분석 후 최적 Worker 수로 실행 (기본)
/c4-run 1         # 1개 Worker 스폰 (백그라운드)
/c4-run 3         # 3개 Worker 스폰 (백그라운드)
/c4-run --max 4   # 자동이지만 최대 4개로 제한
```

**모든 Worker는 백그라운드에서 실행됩니다.** 메인 세션은 사용자와 대화를 계속할 수 있습니다.

## Instructions

### 0. Worker ID 생성 (필수!)

**루프 시작 전** 고유한 worker_id를 생성하세요:

```python
import uuid
WORKER_ID = f"worker-{uuid.uuid4().hex[:8]}"  # 예: "worker-a1b2c3d4"
```

이 ID를 이 세션 전체에서 사용합니다. **절대로 "claude-worker" 같은 고정값 사용 금지!**

## ⚠️ 중요: MCP 도구만 사용

**CLI(bash) 명령어를 사용하지 마세요!** 반드시 MCP 도구를 사용하세요:
- `mcp__c4__c4_status()` - 상태 확인 + 병렬도 분석
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

⚠️ Accept Edits가 꺼져있으면 매 파일 수정마다 승인 필요 → 자동화 불가!

---

### 1. 상태 확인 및 병렬도 분석

```python
status = mcp__c4__c4_status()

# 병렬도 정보 확인
parallelism = status["parallelism"]
# parallelism = {
#   "recommended": 4,        # 추천 Worker 수
#   "ready_now": 6,          # 현재 실행 가능한 태스크
#   "max_parallelism": 5,    # DAG 최대 너비
#   "by_model": {"opus": 3, "sonnet": 3},  # 모델별 분포
#   "pending_total": 10,     # 전체 pending
#   "blocked_count": 4,      # 의존성 미충족
#   "reason": "6 tasks ready, capped at 4 workers"
# }
```

상태에 따른 처리:

- **PLAN/HALTED**: → Step 2로 (EXECUTE 전환)
- **EXECUTE**: → Step 3으로 (바로 Worker 스폰)
- **CHECKPOINT**: "Checkpoint 리뷰 대기 중입니다." 출력 후 종료
- **COMPLETE**: "프로젝트가 완료되었습니다." 출력 후 종료
- **INIT**: "먼저 /c4-plan으로 계획을 수립하세요." 출력 후 종료

### 2. PLAN 또는 HALTED 상태인 경우

MCP 도구로 상태 전환:

```python
result = mcp__c4__c4_start()
```

성공 시 `result.success == true`, `result.status == "EXECUTE"`

### 3. Worker 수 결정 (Smart Auto)

```python
# ARGUMENTS 파싱
args = "$ARGUMENTS".strip()

if args == "" or args == "--auto":
    # 자동 모드: 추천 값 사용
    worker_count = parallelism["recommended"]
elif args.startswith("--max"):
    # 최대값 제한
    max_workers = int(args.split()[-1])
    worker_count = min(parallelism["recommended"], max_workers)
else:
    # 숫자 직접 지정
    worker_count = int(args)

# 최대 7개 제한 (Claude Code subagent 한계)
worker_count = min(worker_count, 7)

# 분석 결과 출력
print(f"""
📊 병렬도 분석:
   총 {parallelism['pending_total']}개 태스크
   현재 실행 가능: {parallelism['ready_now']}개
   의존성 대기: {parallelism['blocked_count']}개
   DAG 최대 너비: {parallelism['max_parallelism']}

💡 추천: {parallelism['recommended']}개 Worker
   이유: {parallelism['reason']}

🚀 실행: {worker_count}개 Worker
""")
```

### 4. Worker 스폰

**모든 Worker는 subagent로 spawn됩니다** (메인 세션은 사용자 대화용).

```python
import uuid

WORKER_PROMPT = """
You are C4 Worker {worker_id}.

## Mission
Execute C4 tasks in a **CONTINUOUS LOOP** until no tasks remain.

## MCP Tools (MUST USE)
- `mcp__c4__c4_get_task(worker_id="{worker_id}")` - 태스크 요청
- `mcp__c4__c4_run_validation(names=["lint", "unit"])` - 검증
- `mcp__c4__c4_submit(task_id, worker_id, commit_sha, validation_results)` - 제출

## ⚠️ CRITICAL: Worker Loop (반드시 루프!)

```
WHILE TRUE:
    1. task = c4_get_task(worker_id="{worker_id}")
    2. IF task is None or no task_id:
           PRINT "✅ No more tasks"
           EXIT
    3. Implement the task (follow DoD)
    4. Run validations, fix issues (max 3 retries)
    5. git commit
    6. result = c4_submit(task_id, ...)
    7. CHECK result.next_action:
           - "get_next_task" → CONTINUE TO STEP 1 (⚠️ 반드시!)
           - "await_checkpoint" → POLL or EXIT
           - "complete" → EXIT
```

**중요**: `next_action`이 `"get_next_task"`이면 **반드시** 다시 `c4_get_task()`를 호출하세요!
**절대로** 태스크 하나 완료 후 종료하지 마세요!

## Your Worker ID: {worker_id}

START NOW: Call `mcp__c4__c4_get_task(worker_id="{worker_id}")` and keep looping!
"""

workers = []
for i in range(worker_count):
    worker_id = f"worker-{uuid.uuid4().hex[:8]}"

    # 모델 결정 (by_model 분포 기반 또는 기본 opus)
    model = "opus"  # 기본값

    result = Task(
        subagent_type="general-purpose",
        description=f"C4 Worker {i+1}/{worker_count}",
        prompt=WORKER_PROMPT.format(worker_id=worker_id),
        model=model,
        run_in_background=True
    )

    workers.append({"id": worker_id, "output": result.output_file})
    print(f"🚀 Worker {i+1}/{worker_count} spawned: {worker_id}")

print(f"""
🐝 C4 Run: {worker_count} workers spawned (백그라운드)

Workers:
""")
for w in workers:
    print(f"  • {w['id']}: {w['output']}")

print("""
Monitor:
  /c4-status - 전체 진행 상황
  tail -f {output_file} - 개별 Worker 로그

⚠️ Worker가 백그라운드에서 실행 중입니다.
   이 세션에서 다른 작업을 하거나 대화를 계속할 수 있습니다.
""")

---

## 🌲 Worktree 격리 (멀티 Worker 필수!)

**여러 Worker가 같은 프로젝트에서 작업할 때 브랜치 충돌을 방지합니다.**

`c4_get_task()` 응답에 `worktree_path`가 포함됩니다:

```python
task = c4_get_task(WORKER_ID)
# task.worktree_path: ".c4/worktrees/worker-abc123"  ← 이 경로 사용!
# task.branch: "c4/w-T-001-0"
```

**모든 파일 작업은 worktree_path 내에서 수행**:

```python
if task.worktree_path:
    work_dir = Path(task.worktree_path)
    file_to_edit = work_dir / "src" / "module.py"
    Read(file_to_edit)
    Edit(file_to_edit, ...)
```

---

## 🤖 Agent Routing

`c4_get_task()` 응답에 에이전트 라우팅 정보가 포함됩니다:

```python
task = c4_get_task(WORKER_ID)
# task.recommended_agent: "frontend-developer"
# task.agent_chain: ["frontend-developer", "test-automator", "code-reviewer"]
```

Worker는 자동으로 판단하여 적절한 에이전트를 선택합니다.

---

## 예상 흐름

### 자동 모드 (기본)
```
/c4-run
→ 상태 확인: EXECUTE
→ 병렬도 분석: 5개 태스크 실행 가능, DAG 너비 4
→ 추천: 4개 Worker
→ 🚀 4개 Worker 스폰
→ 각 Worker가 병렬로 태스크 처리
→ ✅ 모든 태스크 완료
```

### 단일 모드
```
/c4-run 1
→ 상태 확인: EXECUTE
→ 병렬도 분석: (표시만)
→ 🚀 1개 Worker 스폰 (백그라운드)
→ Worker가 백그라운드에서 태스크 처리
→ 메인 세션에서 다른 작업 가능
→ ✅ 모든 태스크 완료
```

---

## 제약사항

| 제약 | 설명 |
|------|------|
| 최대 Worker | 7개 (Claude Code subagent 한계) |
| Worktree | 멀티 Worker 시 필수 |
| Accept Edits | 자동화에 필수 |

## 관련 명령어

- `/c4-status` - 상태 확인 (병렬도 분석 포함)
- `/c4-stop` - 실행 중지
- `/c4-submit` - 수동 제출

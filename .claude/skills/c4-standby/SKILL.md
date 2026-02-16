---
description: "Convert this Claude Code session into a persistent worker that waits for jobs from C5 Hub. Auto-triggers on: standby, worker mode, 대기 모드"
---

# /c4-standby — Persistent Worker Mode

이 세션을 C5 Hub에 연결된 상주 워커로 전환합니다.
워커는 잡이 올 때까지 대기하고, 잡을 실행한 후 다시 대기합니다.

## Usage

```
/c4-standby              # 자동 ID 생성 (hostname-pid)
/c4-standby my-worker    # 지정 ID 사용
```

## Worker Loop

다음 루프를 반복 실행하세요:

### Step 1: 대기

```
c4_worker_standby(worker_id, capabilities: {"tags": ["claude", "mcp"]})
```

이 도구는 **블로킹** 호출입니다. 잡이 올 때까지 또는 shutdown 신호가 올 때까지 대기합니다.

### Step 2: 반환값 확인

- `shutdown: true` → Step 5 (종료)
- `job_id, command, lease_id` → Step 3 (실행)

### Step 3: 잡 실행

`command` 필드를 파싱하여 실행합니다:

| command 패턴 | 실행 방법 |
|-------------|----------|
| `task_id=T-001-0` | `c4_get_task(T-001-0)` → 구현 → `c4_submit(T-001-0)` |
| 셸 명령 (e.g. `go test ./...`) | Bash로 직접 실행 |
| 자연어 (e.g. "fix the login bug") | Claude가 해석하여 자율 실행 |

### Step 4: 완료 보고

```
c4_worker_complete(
  job_id: "<job_id>",
  lease_id: "<lease_id>",
  worker_id: "<worker_id>",
  status: "SUCCEEDED" | "FAILED",
  commit_sha: "<git commit SHA if applicable>",
  summary: "<작업 요약>"
)
```

→ Step 1로 돌아갑니다.

### Step 5: 종료

shutdown 신호를 받으면:

```
c4_worker_shutdown(worker_id: "<worker_id>", reason: "<reason>")
```

## Notes

- 워커 채널 `#worker-{id}`가 Messenger에 자동 생성됩니다
- 프레즌스가 자동 관리됩니다 (idle → working → idle)
- Hub에서 잡을 보내려면: `c4_hub_submit(command: "task_id=T-001-0")`
- 다른 세션에서 종료하려면: `c4_worker_shutdown(worker_id: "my-worker")`

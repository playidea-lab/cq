---
name: c4-standby
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

### Step 3: 잡 실행 (+ 자동 Lease 갱신)

`command` 필드를 파싱하여 실행합니다:

| command/capability 패턴 | 실행 방법 |
|------------------------|----------|
| `task_id=T-001-0` | `c4_get_task(T-001-0)` → 구현 → `c4_submit(T-001-0)` |
| `capability=reasoning` | params.task별 분기 (아래 참조) |
| 셸 명령 (e.g. `go test ./...`) | Bash로 직접 실행 |
| 자연어 (e.g. "fix the login bug") | Claude가 해석하여 자율 실행 |

**Reasoning 잡 처리** (capability=reasoning):

| params.task | 실행 방법 |
|------------|----------|
| `conference` | params에서 hypothesis, metric_history, context 추출 → 토론 수행 → 새 hypothesis + experiment spec 생성 |
| `implement` | params에서 hypothesis, spec 추출 → 코드 작성/수정 |
| `check` | params에서 결과 데이터 추출 → 분석 보고 |

Reasoning 잡 완료 시 result 포맷:
```
c4_worker_complete(
  status: "SUCCEEDED",
  result: {
    new_hypothesis_id: "hyp-yyy",
    experiment_specs: ["r4_exp1.yaml"],
    next_action: "submit_experiment" | "submit_implement" | "finish",
    files_changed: [...],
    summary: "토론 결과 요약"
  }
)
```

**Lease 자동 갱신 (내장):**

`c4_worker_standby`가 잡을 반환하면, **60초 주기 자동 갱신 루프**가 백그라운드에서 시작됩니다.

- `c4_worker_complete` 호출 시 자동 중단됩니다.
- 갱신 3회 연속 실패 시: 워커에 shutdown 신호가 저장되어 다음 루프에서 종료됩니다.
- 이 갱신은 기본 Lease TTL 5분 내에 만료를 방지합니다.

**수동 갱신 (선택):**

5분 이상 소요되는 잡에서 추가로 직접 갱신이 필요하면:

```
c4_hub_lease_renew(lease_id: "<lease_id>")
```

- 갱신 성공 시: `new_expires_at` 타임스탬프가 반환됩니다.
- 갱신 3회 모두 실패하면 잡을 중단하고 `c4_worker_complete(status: "FAILED")`로 보고하세요.

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

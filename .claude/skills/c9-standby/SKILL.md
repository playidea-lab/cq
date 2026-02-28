# C9 Standby

RUN phase 중 훈련이 진행되는 동안 대기. cq mail로 완료 알림을 받으면 자동으로 CHECK를 실행.

**트리거**: "c9-standby", "standby", RUN phase 진입 시 자동

## 역할

훈련은 pi 서버(C5 Hub 워커)에서 독립적으로 실행됨.
Claude는 이 스킬로 대기하다가 완료 알림을 받으면 즉시 다음 phase를 자동 실행.

## 실행 순서

### Step 1: 현재 active_jobs 확인
```bash
cat .c9/state.yaml  # active_jobs의 job_id 목록 확인
```

### Step 2: 알림 방법 결정
`state.yaml`의 `notify.session`이 비어있으면:
```bash
cq ls  # 현재 세션 이름 확인
# 세션 이름이 있으면 state.yaml에 저장
```

### Step 3: Standby 진입

`/c4-standby` 스킬을 호출하여 C5 Hub 워커 모드로 전환.

단, c9-standby는 일반 C5 job이 아니라 **cq mail**을 감지하는 방식:
```
cq mail ls --unread
```
를 주기적으로 확인하여 `[C9-CHECK]` 메시지가 오면 깨어남.

### Step 4: 메일 수신 → 자동 CHECK

수신된 메시지 형식:
```
[C9-CHECK] Round=N exp=exp_name mpjpe=X.X pa=Y.Y util=Z.Z
```

또는 `[C9-BLOCKED]` 수신 시:
```
[C9-BLOCKED] Round=N exp=exp_name reason=...
```

### Step 5: 자동 phase 전환

| 수신 메시지 | 자동 실행 |
|------------|---------|
| `[C9-CHECK]` | `/c9-check` 스킬 실행 |
| `[C9-BLOCKED]` | state.yaml phase → CONFERENCE |
| `[C9-DONE]` + 수렴 | `/c9-finish` 스킬 실행 |
| `[C9-ERROR]` | Dooray 알림 + 사용자 개입 요청 |

## 대기 중 보고

standby 진입 시 두레이로 알림:
```bash
./scripts/c9-notify.sh RUN "훈련 대기 중 — Round N exp_name 실행 중" N
```

## 주의

- standby 중 컨텍스트 소진 방지: state.yaml만 주기적으로 읽음
- cq mail이 없는 환경: 수동으로 `/c9-loop` 재실행
- 여러 실험이 병렬 실행 중이면 모두 완료 후 CHECK 실행

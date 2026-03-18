---
name: c4-reboot
description: |
  Reboot the current named Claude Code session. Writes a reboot flag so cq
  automatically resumes with the same session UUID after exit.
  Only works when launched via `cq claude -t <name>`.
  Triggers: "reboot", "재시작", "세션 재시작", "/reboot", "restart session".
---

# C4 Reboot

현재 named session을 종료하고 동일 UUID로 즉시 재시작합니다.

## 동작 원리

```
cq claude -t mywork    ← cq가 부모 프로세스로 대기
  └── claude (현재 세션)
        └── /reboot 실행
              ├── ~/.c4/.reboot 파일 작성
              └── /exit 지시
                    ↓
              cq: .reboot 감지 → claude --resume <uuid> 재실행
```

## Steps

### 1. 세션 확인

```bash
echo "SESSION: ${CQ_SESSION_NAME:-<unnamed>} / UUID: ${CQ_SESSION_UUID:-<unknown>}"
```

`CQ_SESSION_NAME`이 비어 있으면 `cq claude -t <name>` 없이 실행된 세션입니다.
이 경우 reboot은 동작하지 않습니다 — 사용자에게 알리고 중단합니다.

### 2. Reboot 플래그 작성 + 자동 종료

```bash
echo "${CQ_SESSION_UUID:-}" > ~/.c4/.reboot && echo "rebooting session '${CQ_SESSION_NAME}' (${CQ_SESSION_UUID})..."
```

`.reboot` 파일에 UUID를 기록합니다. `cq` 부모 프로세스가 Claude Code 종료 후
파일에서 UUID를 읽어 `claude --resume <uuid>`를 실행합니다.

위 명령 실행 후 **반드시 `/exit`을 호출**하여 Claude Code를 종료합니다.
`kill`로 프로세스를 종료하면 PID 불일치로 잘못된 프로세스가 종료될 수 있습니다.

```
/exit
```

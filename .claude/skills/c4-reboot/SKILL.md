---
name: c4-reboot
description: |
  Reboot the current named Claude Code session. Writes a reboot flag so cq
  automatically resumes with the same session UUID after exit.
  Only works when launched via `cq -t <name>`.
  Triggers: "reboot", "재시작", "세션 재시작", "/reboot", "restart session".
allowed-tools: Bash
---

# C4 Reboot

현재 named session을 종료하고 동일 UUID로 즉시 재시작합니다.

## 동작 원리

```
cq -t mywork           ← cq가 부모 프로세스로 대기
  └── claude (현재 세션)
        └── /reboot 실행
              ├── ~/.c4/.reboot-mywork 파일 작성 (세션별 격리)
              └── cq가 감지 → interrupt → 재시작
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
touch ~/.c4/.reboot-${CQ_SESSION_NAME} && echo "rebooting session '${CQ_SESSION_NAME}' (${CQ_SESSION_UUID})..."
```

`.reboot-{name}` 파일을 생성합니다. **세션 이름별로 격리**되므로 다른 세션에 영향 없음.
`cq` 부모 프로세스가 2초 간격으로 자기 이름의 파일만 감시합니다.

위 명령 실행 후 **아무것도 하지 않아도 됩니다** — cq가 자동으로 처리합니다.

---
name: done
essential: true
description: |
  [internal] Mark the current session as done with full capture (summary + knowledge + persona).
  Creates a .done marker so cq runs captureSessionFull on exit.
  Only works when launched via `cq -t <name>`.
  Triggers: "done", "세션 완료", "세션 끝", "/done", "mark done".
allowed-tools: Bash
---

# C4 Done

현재 named session을 "완료" 상태로 마킹하고 종료합니다.
종료 시 풀 캡처 실행: 전체 요약 + 지식 기록 + 페르소나 학습 + status→done.

## /exit vs /done

| 명령 | 캡처 레벨 | 상태 변경 |
|------|----------|----------|
| `/exit` | Light (Updated만 갱신) | 없음 |
| `/done` | Full (요약+지식+페르소나) | → done |

## 동작 원리

```
cq -t mywork           <- cq가 부모 프로세스로 대기
  └── claude (현재 세션)
        └── /done 실행
              ├── ~/.c4/running/mywork.done 마커 파일 작성
              └── cq가 감지 → interrupt → captureSessionFull → 종료
```

## Steps

### 1. 세션 확인

```bash
echo "SESSION: ${CQ_SESSION_NAME:-<unnamed>} / UUID: ${CQ_SESSION_UUID:-<unknown>}"
```

`CQ_SESSION_NAME`이 비어 있으면 `cq claude -t <name>` 없이 실행된 세션입니다.
이 경우 done은 동작하지 않습니다 — 사용자에게 알리고 중단합니다.

### 2. Done 마커 작성 + 자동 종료

```bash
mkdir -p ~/.c4/running && touch ~/.c4/running/${CQ_SESSION_NAME}.done && echo "session '${CQ_SESSION_NAME}' marked as done — closing with full capture..."
```

`.done` 마커 파일을 생성합니다. cq 부모 프로세스가 2초 ticker로 감지하여
SIGINT → captureSessionFull → 세션 종료.

마커 작성 후 claude 프로세스에 SIGINT를 보내 즉시 종료합니다:

```bash
CLAUDE_PID=$(cat ~/.c4/running/${CQ_SESSION_NAME}.pid 2>/dev/null)
if [ -n "$CLAUDE_PID" ]; then
  kill -INT "$CLAUDE_PID" 2>/dev/null && echo "SIGINT sent to claude (PID $CLAUDE_PID) — cq will run captureSessionFull"
fi
```

cq 부모 프로세스가 claude 종료를 감지하면 `.done` 마커 유무를 확인하고
captureSessionFull을 실행합니다. ticker 대기(최대 2초) 없이 즉시 종료.

---
name: c4-attach
description: |
  Attach a name to the current Claude Code session so it can be resumed later
  with `cq claude -t <name>`. Works even if the session was started without -t.
  Optionally adds a short memo describing the session's purpose.
  Triggers: "세션 이름 붙여", "이 세션에 이름", "attach", "name this session",
  "session name", "/c4-attach".
---

# C4 Attach

현재 세션에 이름을 붙여 나중에 `cq claude -t <name>`으로 재개할 수 있게 합니다.

## 인자 파싱

```
/c4-attach <name>               → 이름만
/c4-attach <name> <memo...>     → 이름 + 메모
```

## Steps

### 1. 이름 확인

**인자로 이름을 전달받은 경우**: 그대로 사용.

**이름 없이 호출된 경우**: `basename $(pwd)` 기본값 제안 → 사용자 확인 후 진행.

### 2. 세션 이름 저장

메모가 있는 경우:
```bash
cq session name <이름> --force -m "<메모>"
```

메모가 없는 경우:
```bash
cq session name <이름> --force
```

> **CRITICAL**: `--uuid`를 직접 지정하지 마라. JSONL 경로에서 UUID를 추론하지 마라.
> CLI가 `CQ_SESSION_UUID` env var 또는 최신 JSONL 파일에서 자동 감지한다.

`--force`: 이미 같은 이름이 있어도 즉시 덮어씀.

### 3. 목록 확인

```bash
cq ls
```

`(current)` 표시로 현재 세션이 정확히 저장되었는지 확인합니다.

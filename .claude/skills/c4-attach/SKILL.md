---
description: |
  Attach a name to the current Claude Code session so it can be resumed later
  with `cq claude -t <name>`. Works even if the session was started without -t.
  Triggers: "세션 이름 붙여", "이 세션에 이름", "attach", "name this session",
  "session name", "/c4-attach".
---

# C4 Attach

현재 세션에 이름을 붙여 나중에 `cq claude -t <name>`으로 재개할 수 있게 합니다.

## Steps

### 1. 이름 확인

**인자로 이름을 전달받은 경우** (예: `/c4-attach maildev`): 그대로 사용, 확인 없이 즉시 Step 2로.

**이름 없이 호출된 경우**: 기본값을 제안하고 반드시 사용자 확인을 받은 후 Step 2로.

```bash
basename $(pwd)
```

예: "기본값은 `cq`입니다. 이 이름으로 설정할까요?" → 사용자가 확인하면 진행.

### 2. 세션 이름 저장

```bash
cq session name <이름>
```

성공하면 다음과 같이 출력됩니다:
```
session '<name>' → <uuid8>...
Next time: cq claude -t <name>
```

### 3. 목록 확인

```bash
cq ls
```

`(current)` 표시로 현재 세션이 정확히 저장되었는지 확인합니다.

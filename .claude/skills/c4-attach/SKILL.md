---
description: |
  Attach a name to the current Claude Code session so it can be resumed later
  with `cq claude -t <name>`. Works even if the session was started without -t.
  Optionally adds a short memo describing the session's purpose.
  Triggers: "세션 이름 붙여", "이 세션에 이름", "attach", "name this session",
  "session name", "/c4-attach".
---

# C4 Attach

현재 세션에 이름을 붙여 나중에 `cq claude -t <name>`으로 재개할 수 있게 합니다.
메모를 함께 지정하면 `cq ls`에서 이 세션이 무엇을 위한 것인지 볼 수 있습니다.

## 인자 파싱

```
/c4-attach <name>               → 이름만
/c4-attach <name> <memo...>     → 이름 + 메모 (공백 포함 가능, 나머지 전체가 메모)
```

예:
- `/c4-attach maildev` → 이름: `maildev`, 메모 없음
- `/c4-attach payment 결제 플로우 리팩토링` → 이름: `payment`, 메모: `결제 플로우 리팩토링`

## Steps

### 1. 이름 확인

**인자로 이름을 전달받은 경우**: 그대로 사용, 확인 없이 즉시 Step 2로.

**이름 없이 호출된 경우**: 기본값을 제안하고 반드시 사용자 확인을 받은 후 Step 2로.

```bash
basename $(pwd)
```

예: "기본값은 `cq`입니다. 이 이름으로 설정할까요?" → 사용자가 확인하면 진행.

### 2. 현재 세션 UUID 확인 (CRITICAL)

Claude는 현재 대화의 세션 UUID를 시스템 컨텍스트에서 직접 알고 있다.
JSONL 파일 경로 형식: `~/.claude/projects/<project>/<uuid>.jsonl`

자동 감지는 불안정하므로, 반드시 `--uuid` 플래그로 명시적으로 전달한다.

### 3. 세션 이름 저장

메모가 있는 경우:
```bash
cq session name <이름> --uuid <uuid> -m "<메모>"
```

메모가 없는 경우:
```bash
cq session name <이름> --uuid <uuid>
```

성공하면 다음과 같이 출력됩니다:
```
session '<name>' → <uuid8>...
Next time: cq claude -t <name>
```

### 4. 목록 확인

```bash
cq ls
```

`(current)` 표시로 현재 세션이 정확히 저장되었는지 확인합니다.
메모가 있으면 세션 이름 아래에 `memo: ...` 로 표시됩니다.

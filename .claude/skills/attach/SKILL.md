---
name: attach
essential: true
description: |
  [internal] Attach a name to the current Claude Code session so it can be resumed later
  with `cq -t <name>`. Works even if the session was started without -t.
  Optionally adds a short memo describing the session's purpose.
  Triggers: "세션 이름 붙여", "이 세션에 이름", "attach", "name this session",
  "session name", "/attach".
allowed-tools: Bash(cq *), Bash(ls *), Bash(basename *)
---

# C4 Attach

현재 세션에 이름을 붙여 나중에 `cq -t <name>`으로 재개할 수 있게 합니다.

## 인자 파싱

```
/attach <name>               → 이름만
/attach <name> <memo...>     → 이름 + 메모
```

## Steps

### 1. 이름 확인

**인자로 이름을 전달받은 경우**: 그대로 사용.

**이름 없이 호출된 경우**: `basename $(pwd)` 기본값 제안 → 사용자 확인 후 진행.

### 2. UUID 감지

`CQ_SESSION_UUID` 환경변수가 비어있을 수 있다 (예: `claude --resume`으로 직접 시작한 세션).
이 경우 JSONL 파일에서 현재 세션 UUID를 직접 감지해야 한다.

```bash
# 환경변수 확인
UUID="$CQ_SESSION_UUID"
if [ -z "$UUID" ]; then
  # JSONL에서 가장 최근 수정된 파일의 이름(=UUID) 추출
  UUID=$(ls -t ~/.claude/projects/$(echo "$PWD" | sed 's|/|-|g')/*.jsonl 2>/dev/null | head -1 | xargs basename 2>/dev/null | sed 's/.jsonl$//')
fi
```

### 3. 세션 이름 저장

메모가 있는 경우:
```bash
cq session name <이름> --uuid "$UUID" --force -m "<메모>"
```

메모가 없는 경우:
```bash
cq session name <이름> --uuid "$UUID" --force
```

> **CRITICAL**: `--uuid`를 반드시 명시 전달한다. CLI 자동 감지에 의존하지 않는다.
> 여러 세션이 동시에 실행 중이면 CLI가 잘못된 UUID를 잡을 수 있기 때문이다.

`--force`: 이미 같은 이름이 있어도 즉시 덮어씀.

### 4. 목록 확인

```bash
cq ls
```

`(current)` 표시로 현재 세션이 정확히 저장되었는지 확인합니다.

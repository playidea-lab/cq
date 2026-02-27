# Antipattern Hooks Install Guide

이 가이드는 CQ 프로젝트에서 사용하는 두 가지 PreToolUse 훅의 설치 방법을 설명합니다.

## 훅 구성

| 훅 | 경로 | 범위 | 차단 대상 |
|----|------|------|----------|
| `global-antipattern.sh` | `~/.claude/hooks/` | 전역 (모든 프로젝트) | pip install, python *.py, pytest |
| `c4-gate.sh` | `.claude/hooks/` | C4 프로젝트 전용 | TodoWrite, EnterPlanMode |

---

## 1. global-antipattern.sh (전역 훅)

Python 개발 안티패턴을 차단합니다. 모든 프로젝트에 적용됩니다.

### 차단 패턴

| 명령 | 차단 이유 | 대안 |
|------|---------|------|
| `pip install <pkg>` | uv 사용 원칙 | `uv add <pkg>` |
| `pip3 install <pkg>` | uv 사용 원칙 | `uv add <pkg>` |
| `python script.py` | uv 없는 직접 실행 | `uv run python script.py` |
| `python3 script.py` | uv 없는 직접 실행 | `uv run python script.py` |
| `pytest [args]` | uv 없는 직접 실행 | `uv run pytest [args]` |

**허용되는 예외:**
- `uv pip install` (uv의 pip 래퍼)
- `uv run python ...`
- `uv run pytest`
- `python --version`, `python -V`
- `python -c "..."` (인라인 코드)
- `python -m module`

### 설치

```bash
# 1. 훅 파일 복사
cp c4-core/cmd/c4/templates/hooks/global-antipattern.sh ~/.claude/hooks/
chmod +x ~/.claude/hooks/global-antipattern.sh

# 또는 심볼릭 링크 (업데이트 자동 반영)
ln -sf "$(pwd)/c4-core/cmd/c4/templates/hooks/global-antipattern.sh" \
    ~/.claude/hooks/global-antipattern.sh
```

> **참고**: `~/.claude/hooks/global-antipattern.sh`가 없다면 프로젝트 훅에서 직접 사용:
> `bash .claude/hooks/c4-gate.sh` 또는 `cq init` 실행 후 안내에 따릅니다.

### ~/.claude/settings.json 등록

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "~/.claude/hooks/global-antipattern.sh"
          }
        ]
      }
    ]
  }
}
```

### 동작 확인

```bash
# 차단 테스트 (exit 2 반환해야 함)
echo '{"tool_name":"Bash","tool_input":{"command":"pip install requests"}}' \
    | bash ~/.claude/hooks/global-antipattern.sh
echo "exit: $?"

# 허용 테스트 (exit 0 반환해야 함)
echo '{"tool_name":"Bash","tool_input":{"command":"uv run pytest"}}' \
    | bash ~/.claude/hooks/global-antipattern.sh
echo "exit: $?"
```

---

## 2. c4-gate.sh (C4 프로젝트 전용 훅)

C4 워크플로우 도구 사용을 강제합니다. `.c4/` 디렉토리가 있는 프로젝트에서만 활성화됩니다.

### 차단 패턴

| 도구 | 차단 이유 | 대안 |
|------|---------|------|
| `TodoWrite` | C4 SSOT 이중 트래킹 | `c4_add_todo` 또는 `/c4-add-task` |
| `EnterPlanMode` | C4 플래닝 워크플로우 우회 | `/c4-plan` 스킬 |

### 설치 확인

`cq init`을 실행하면 `.claude/hooks/c4-gate.sh`가 자동으로 프로젝트에 등록됩니다.

```bash
cq init
```

수동 확인:

```bash
ls -la .claude/hooks/c4-gate.sh
```

### ~/.claude/projects/.../settings.json 등록

`cq init`이 자동으로 등록하므로, 직접 편집은 불필요합니다. 수동 등록이 필요한 경우:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "TodoWrite|EnterPlanMode|Bash|Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/project/.claude/hooks/c4-gate.sh"
          }
        ]
      }
    ]
  }
}
```

### 동작 확인

```bash
# 차단 테스트 (exit 2 반환해야 함)
echo '{"tool_name":"TodoWrite","tool_input":{"todos":[]}}' \
    | CLAUDE_PROJECT_DIR="$(pwd)" bash .claude/hooks/c4-gate.sh
echo "exit: $?"

echo '{"tool_name":"EnterPlanMode","tool_input":{}}' \
    | CLAUDE_PROJECT_DIR="$(pwd)" bash .claude/hooks/c4-gate.sh
echo "exit: $?"
```

---

## 테스트 실행

```bash
# bats 설치 (macOS)
brew install bats-core

# 전체 hooktest 스위트 실행
bats c4-core/test/hooktest/

# 개별 테스트
bats c4-core/test/hooktest/test_global_antipattern.bats   # 8개 테스트
bats c4-core/test/hooktest/test_c4gate_mcp.bats           # 4개 테스트
```

---

## 문제 해결

### global-antipattern.sh가 동작하지 않는 경우

1. 파일 존재 확인: `ls -la ~/.claude/hooks/global-antipattern.sh`
2. 실행 권한 확인: `chmod +x ~/.claude/hooks/global-antipattern.sh`
3. jq 설치 확인: `which jq || brew install jq`
4. settings.json에 훅 등록 확인

### c4-gate.sh가 동작하지 않는 경우

1. `CLAUDE_PROJECT_DIR` 환경변수 또는 `.c4/` 디렉토리 확인
2. `cq doctor` 실행으로 훅 설정 진단
3. `cq init --yes`로 훅 재등록

---
name: hookify
description: |
  커스텀 자동화 훅/규칙 생성 가이드. git hook, CI hook, 패턴 감지 규칙, 자동 경고 등
  자동화 트리거를 만들 때 이 스킬을 사용하세요. "훅 만들기", "hookify", "자동화 규칙",
  "커스텀 훅", "패턴 감지", "자동 경고", "pre-commit hook" 등의 요청에 트리거됩니다.
---
# Hookify

커스텀 자동화 훅/규칙 생성 가이드.

## 트리거

"훅 만들기", "hookify", "자동화 규칙", "커스텀 훅", "패턴 감지", "자동 경고"

## Steps

### 1. 규칙 정의

어떤 상황에서 어떤 행동을 할지 결정:

- **감지 대상**: 어떤 도구/명령/패턴을 감시?
- **조건**: 언제 트리거?
- **행동**: 경고 / 차단 / 수정 제안?

### 2. 훅 유형

| 유형 | 시점 | 용도 |
|------|------|------|
| **PreToolUse** | 도구 실행 전 | 위험한 명령 차단 |
| **PostToolUse** | 도구 실행 후 | 결과 검증, 로깅 |
| **SessionStart** | 세션 시작 | 컨텍스트 로딩, 안내 |
| **Stop** | 세션 종료 전 | 미완료 작업 경고 |

### 3. 설정 파일 구조

`.claude/settings.json`에 훅 등록:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": ".claude/hooks/my-guard.sh"
          }
        ]
      }
    ]
  }
}
```

### 4. 훅 스크립트 작성

훅은 stdin으로 JSON을 받고 stdout으로 JSON을 반환:

```bash
#!/bin/bash
# .claude/hooks/no-force-push.sh
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

if echo "$COMMAND" | grep -q "push.*--force"; then
  echo '{"decision":"block","reason":"force push는 금지입니다. --force-with-lease를 사용하세요."}'
  exit 0
fi

echo '{"decision":"allow"}'
```

### 5. 흔한 규칙 패턴

| 규칙 | 감지 | 행동 |
|------|------|------|
| force push 차단 | `push.*--force` | block |
| rm -rf / 차단 | `rm.*-rf.*/` | block |
| pip install 차단 | `^pip install` | block + "uv add 사용" 안내 |
| 시크릿 감지 | API key 패턴 | block + 경고 |
| 큰 파일 경고 | 10MB+ 파일 추가 | warn |
| TODO 리마인더 | TODO 포함 코드 작성 | warn |

### 6. 테스트

```bash
# 훅 스크립트 직접 테스트
echo '{"tool_input":{"command":"git push --force origin main"}}' | .claude/hooks/no-force-push.sh
# Expected: {"decision":"block","reason":"..."}
```

### 7. 디버깅

- 훅 실패 시 stderr에 로그 출력 (stdout은 프로토콜용)
- `exit 0` = allow (fail-open), `exit 1` = 훅 에러 (기본 allow)
- `.claude/settings.json` 문법 에러 시 모든 훅 무시됨

## 안티패턴

- 너무 많은 훅 (매 명령마다 지연)
- 모든 것을 block (사용성 파괴)
- stdout에 디버깅 출력 (프로토콜 오염)
- fail-close 설계 (훅 에러 시 전체 차단)

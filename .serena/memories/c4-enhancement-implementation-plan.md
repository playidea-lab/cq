# C4 Enhancement Implementation Plan

> **목표**: everything-claude-code 저장소에서 배운 베스트 프랙티스를 C4 프로젝트에 적용
> **상태**: 계획 수립 완료, 구현 대기

---

## 📁 변경 요약

### 새로 생성할 파일/디렉토리

```
.claude/
├── rules/                      # NEW: 필수 가이드라인
│   ├── security.md             # 보안 체크리스트
│   ├── coding-style.md         # 코딩 스타일 가이드
│   ├── testing.md              # 테스트 요구사항
│   └── git-workflow.md         # Git 워크플로우
├── hooks.json                  # NEW: 자동화 훅 설정
└── agents/                     # NEW: 에이전트 정의 (선택적)
    ├── code-reviewer.md
    ├── security-reviewer.md
    └── planner.md
```

### 수정할 파일

```
.claude/
├── settings.json               # MODIFY: 훅 활성화
├── commands/
│   ├── c4-plan.md             # MODIFY: 보안 체크 추가
│   ├── c4-run.md              # MODIFY: 자동 리뷰 트리거 추가
│   └── c4-validate.md         # MODIFY: 보안 검증 추가
```

---

## 1. Rules 디렉토리 생성

### 1.1 `.claude/rules/security.md`

**목적**: 모든 코드 변경 시 적용되는 보안 체크리스트

```markdown
# Security Rules (필수)

> 모든 코드 변경 시 자동 적용됩니다.

## 체크리스트

### CRITICAL (차단 - 커밋 불가)
- [ ] 하드코딩된 시크릿 없음 (API 키, 비밀번호, 토큰)
- [ ] SQL 인젝션 방지 (파라미터화 쿼리 또는 ORM 사용)
- [ ] 명령어 인젝션 방지 (사용자 입력 직접 실행 금지)

### HIGH (경고 - 검토 필요)
- [ ] XSS 방지 (입력 이스케이프, CSP 헤더)
- [ ] CSRF 토큰 사용 (상태 변경 요청)
- [ ] 인증/인가 검증 (모든 보호 엔드포인트)

### MEDIUM (권장)
- [ ] 에러 메시지에 내부 정보 노출 금지
- [ ] 민감 데이터 로깅 금지
- [ ] HTTPS 사용 (프로덕션)

## 검사 명령

```bash
# 시크릿 검사
grep -rn "password\s*=" --include="*.py" --include="*.js" --include="*.ts"
grep -rn "api_key\s*=" --include="*.py" --include="*.js" --include="*.ts"
grep -rn "secret\s*=" --include="*.py" --include="*.js" --include="*.ts"

# SQL 인젝션 패턴
grep -rn "execute.*%s" --include="*.py"
grep -rn "f\".*SELECT.*{" --include="*.py"
```

## 위반 시 처리

| 심각도 | 처리 |
|--------|------|
| CRITICAL | 커밋 차단, 즉시 수정 필수 |
| HIGH | PR 리뷰에서 반드시 확인 |
| MEDIUM | 개선 권장, 병합 가능 |
```

---

### 1.2 `.claude/rules/coding-style.md`

**목적**: 코드 품질 및 스타일 가이드

```markdown
# Coding Style Rules

> 일관된 코드 품질을 위한 가이드입니다.

## 파일 크기

- **권장**: 200-400줄
- **최대**: 500줄 (초과 시 분할 고려)

## 함수 크기

- **권장**: 30줄 이하
- **최대**: 50줄

## 명명 규칙

### Python
- 클래스: `PascalCase`
- 함수/변수: `snake_case`
- 상수: `UPPER_SNAKE_CASE`
- private: `_prefix`

### TypeScript/JavaScript
- 클래스: `PascalCase`
- 함수/변수: `camelCase`
- 상수: `UPPER_SNAKE_CASE`
- 인터페이스: `IPascalCase` 또는 `PascalCase`

## 불변성

- `const` 우선 (JS/TS)
- `Final` 사용 권장 (Python)
- 가변 상태 최소화

## 타입 안전성

### Python
```python
# ✅ Good
def get_user(user_id: int) -> User | None:
    ...

# ❌ Bad
def get_user(user_id):
    ...
```

### TypeScript
```typescript
// ✅ Good
function getUser(userId: number): User | undefined {
    ...
}

// ❌ Bad: any 사용
function getUser(userId: any): any {
    ...
}
```

## 주석

- 복잡한 로직에만 주석 추가
- "왜"를 설명, "무엇"은 코드가 설명
- TODO/FIXME는 이슈 번호와 함께
```

---

### 1.3 `.claude/rules/testing.md`

**목적**: 테스트 요구사항

```markdown
# Testing Rules

> 모든 코드 변경에는 테스트가 필요합니다.

## 커버리지 요구사항

| 단계 | 최소 커버리지 |
|------|--------------|
| 탐색/프로토타입 | 0% (테스트 선택적) |
| 검증 | 50%+ (핵심 로직) |
| 프로덕션 | 80%+ (전체) |

## TDD 사이클

```
🔴 RED → 🟢 GREEN → 🔵 REFACTOR → 반복
```

1. **RED**: 실패하는 테스트 먼저 작성
2. **GREEN**: 테스트 통과하는 최소 구현
3. **REFACTOR**: 코드 개선 (테스트 유지)

## 테스트 유형

### Unit Tests (필수)
- 단일 함수/메서드 테스트
- 외부 의존성 모킹
- 빠른 실행 (< 1초/테스트)

### Integration Tests (권장)
- 컴포넌트 간 상호작용
- 실제 DB 사용 가능 (테스트 DB)
- 주요 플로우 테스트

### E2E Tests (선택적)
- 전체 사용자 시나리오
- 브라우저/API 실제 호출
- 주요 비즈니스 플로우

## 네이밍 컨벤션

```python
# Python
def test_login_with_valid_credentials_returns_token():
    ...

def test_login_with_invalid_password_raises_auth_error():
    ...
```

```typescript
// TypeScript
describe("AuthService", () => {
    it("should return token when credentials are valid", () => {
        ...
    });

    it("should throw AuthError when password is invalid", () => {
        ...
    });
});
```

## 검증 명령

```bash
# Python
uv run pytest --cov=. --cov-report=term-missing

# TypeScript
npm test -- --coverage
```
```

---

### 1.4 `.claude/rules/git-workflow.md`

**목적**: Git 워크플로우 가이드

```markdown
# Git Workflow Rules

## 커밋 메시지 형식

```
<type>(<scope>): <subject>

[body]

[footer]
```

### Type
- `feat`: 새 기능
- `fix`: 버그 수정
- `refactor`: 리팩토링 (기능 변경 없음)
- `test`: 테스트 추가/수정
- `docs`: 문서 변경
- `chore`: 빌드/설정 변경

### 예시
```
feat(auth): add JWT token refresh endpoint

- Add /api/auth/refresh endpoint
- Implement token rotation on refresh
- Add tests for token expiration

Closes #123
```

## 브랜치 전략

```
main (또는 master)
  └── feature/<task-id>-<description>
      예: feature/T-001-add-login-api
```

## PR 규칙

1. **제목**: 커밋 메시지 형식과 동일
2. **설명**: 
   - 변경 사항 요약
   - 테스트 방법
   - 스크린샷 (UI 변경 시)
3. **리뷰어**: 최소 1명 필수

## 금지 사항

- ❌ `main`/`master`에 직접 push
- ❌ `--force` push (협업 브랜치)
- ❌ 시크릿 커밋
- ❌ 대용량 바이너리 커밋
```

---

## 2. Hooks 설정

### 2.1 `.claude/hooks.json`

**목적**: 자동화 훅 정의

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "id": "security-check-before-commit",
        "matcher": "Bash(git commit:*)",
        "command": "echo '🔒 Security check before commit...' && grep -rn 'password\\s*=' --include='*.py' --include='*.ts' . || true",
        "description": "커밋 전 하드코딩된 시크릿 검사"
      },
      {
        "id": "prevent-force-push-main",
        "matcher": "Bash(git push --force:*main*)",
        "command": "echo '⚠️ Force push to main is blocked!' && exit 1",
        "description": "main 브랜치 force push 방지"
      }
    ],
    "PostToolUse": [
      {
        "id": "auto-lint-python",
        "matcher": "Edit(*.py)",
        "command": "uv run ruff check --fix $FILE && uv run ruff format $FILE",
        "description": "Python 파일 수정 후 자동 린트/포맷"
      },
      {
        "id": "auto-lint-typescript",
        "matcher": "Edit(*.ts)",
        "command": "npx eslint --fix $FILE && npx prettier --write $FILE",
        "description": "TypeScript 파일 수정 후 자동 린트/포맷"
      },
      {
        "id": "type-check-typescript",
        "matcher": "Edit(*.ts)",
        "command": "npx tsc --noEmit",
        "description": "TypeScript 수정 후 타입 검사"
      },
      {
        "id": "warn-console-log",
        "matcher": "Edit(*.ts)|Edit(*.js)",
        "command": "grep -n 'console.log' $FILE && echo '⚠️ console.log found - remove before commit' || true",
        "description": "console.log 사용 경고"
      }
    ],
    "Stop": [
      {
        "id": "final-security-check",
        "command": "echo '🔒 Final security check...' && grep -rn 'TODO: remove' --include='*.py' --include='*.ts' . || true",
        "description": "세션 종료 시 최종 검사"
      }
    ]
  }
}
```

---

## 3. Settings 수정

### 3.1 `.claude/settings.json` 수정

**변경 내용**: 훅 활성화 및 규칙 참조

```json
{
  "permissions": {
    "allow": [
      "mcp__c4__*",
      "mcp__serena__*",
      "mcp__plugin_serena_serena__*",
      "Bash",
      "Write(/Users/changmin/git/c4/**)",
      "Edit(/Users/changmin/git/c4/**)",
      "MultiEdit(/Users/changmin/git/c4/**)",
      "Read(/Users/changmin/git/c4/**)"
    ],
    "defaultMode": "acceptEdits"
  },
  "enableAllProjectMcpServers": true,
  "hooks": {
    "enabled": true,
    "configPath": ".claude/hooks.json"
  },
  "rules": {
    "always": [
      ".claude/rules/security.md",
      ".claude/rules/coding-style.md"
    ],
    "onCommit": [
      ".claude/rules/git-workflow.md"
    ],
    "onTest": [
      ".claude/rules/testing.md"
    ]
  }
}
```

---

## 4. Commands 수정

### 4.1 `c4-plan.md` 수정

**변경**: Phase 0에 보안 규칙 로드 추가

```diff
### Phase 0: 현황 파악 (Context Display)

**먼저 전체 현황을 수집하고 출력합니다.**

#### 0.1 데이터 수집

```python
# 1. 상태 확인
status = mcp__c4__c4_status()

+ # 1.5. 보안 규칙 로드 (항상 적용)
+ security_rules = read(".claude/rules/security.md")
+ coding_rules = read(".claude/rules/coding-style.md")
+ 
+ print("📋 활성화된 규칙:")
+ print("  - security.md (CRITICAL/HIGH/MEDIUM 체크)")
+ print("  - coding-style.md (파일/함수 크기, 명명 규칙)")

# 2. 기존 Specs 확인
...
```

---

### 4.2 `c4-run.md` 수정

**변경**: 구현 후 자동 리뷰 트리거 추가

```diff
### 3. Worker Loop 시작

```
LOOP:
  task = c4_get_task(WORKER_ID)
  if task is null:
      exit("✅ COMPLETE")

  implement_with_agent_routing(task)
+ 
+ # 자동 코드 리뷰 (보안 + 품질)
+ review_result = run_auto_review(task.scope)
+ if review_result.has_critical:
+     print("❌ CRITICAL 이슈 발견 - 수정 필요")
+     continue  # 수정 후 재시도
+ if review_result.has_high:
+     print("⚠️ HIGH 이슈 발견 - 검토 권장")

  validate()
  ...
```

**자동 리뷰 함수 추가:**

```python
def run_auto_review(scope: str) -> ReviewResult:
    """
    보안 및 품질 체크 자동 실행
    
    1. security.md 규칙 검사
    2. coding-style.md 규칙 검사
    3. 결과 집계 (CRITICAL/HIGH/MEDIUM)
    """
    issues = []
    
    # 보안 검사
    secrets = check_hardcoded_secrets(scope)
    if secrets:
        issues.append(Issue("CRITICAL", "하드코딩된 시크릿 발견", secrets))
    
    sql_injection = check_sql_injection(scope)
    if sql_injection:
        issues.append(Issue("CRITICAL", "SQL 인젝션 위험", sql_injection))
    
    # 품질 검사
    large_files = check_file_size(scope, max_lines=500)
    if large_files:
        issues.append(Issue("MEDIUM", "파일 크기 초과", large_files))
    
    large_functions = check_function_size(scope, max_lines=50)
    if large_functions:
        issues.append(Issue("MEDIUM", "함수 크기 초과", large_functions))
    
    return ReviewResult(
        has_critical=any(i.severity == "CRITICAL" for i in issues),
        has_high=any(i.severity == "HIGH" for i in issues),
        issues=issues
    )
```
```

---

### 4.3 `c4-validate.md` 수정

**변경**: 보안 검증 단계 추가

```diff
## 검증 항목

1. **lint**: 코드 스타일 검사
2. **unit**: 유닛 테스트
3. **e2e**: E2E 테스트 (설정 시)
+ 4. **security**: 보안 검사 (신규)

+ ### security 검증
+ 
+ ```yaml
+ validation:
+   commands:
+     security: |
+       echo "🔒 Security validation..."
+       # 시크릿 검사
+       ! grep -rn "password\s*=" --include="*.py" . | grep -v "test"
+       ! grep -rn "api_key\s*=" --include="*.py" . | grep -v "test"
+       # SQL 인젝션 패턴
+       ! grep -rn "execute.*%s" --include="*.py" .
+       echo "✅ Security check passed"
+ ```
```

---

## 5. Agents 디렉토리 (선택적)

### 5.1 `.claude/agents/code-reviewer.md`

**목적**: 코드 리뷰 전문 에이전트 정의

```markdown
# Code Reviewer Agent

> 모든 코드 변경 시 자동 활성화되는 리뷰 에이전트

## 자동 활성화 조건

- `Edit()` 또는 `Write()` 도구 사용 후
- PR 생성 전
- `/c4-submit` 실행 전

## 검사 항목

### 1. 보안 (CRITICAL)
- `.claude/rules/security.md` 기준 적용

### 2. 품질 (HIGH/MEDIUM)
- `.claude/rules/coding-style.md` 기준 적용

### 3. 테스트 (HIGH)
- `.claude/rules/testing.md` 기준 적용

## 출력 형식

```
📝 Code Review Report

🔴 CRITICAL (차단):
  - [security] 하드코딩된 API 키 발견 (line 42)

🟡 HIGH (검토 필요):
  - [test] 테스트 커버리지 45% (최소 80% 필요)

🟢 MEDIUM (권장):
  - [style] 함수 크기 62줄 (권장 50줄 이하)

결과: ❌ FAIL (CRITICAL 이슈 해결 필요)
```

## 우선순위 체계

| 심각도 | 처리 | 커밋 |
|--------|------|------|
| CRITICAL | 즉시 수정 | 차단 |
| HIGH | 검토 후 결정 | 경고 |
| MEDIUM | 권장사항 | 허용 |
```

---

### 5.2 `.claude/agents/security-reviewer.md`

**목적**: 보안 전문 리뷰 에이전트

```markdown
# Security Reviewer Agent

> 보안 관련 코드 변경 시 자동 활성화

## 자동 활성화 조건

- `auth`, `login`, `password`, `token`, `secret` 키워드가 포함된 파일 수정
- `.env`, `config` 파일 수정
- API 엔드포인트 추가/수정

## 검사 항목

### 인증/인가
- 모든 보호 엔드포인트에 인증 미들웨어 적용
- 권한 검사 로직 존재
- 토큰 만료 처리

### 입력 검증
- 사용자 입력 검증
- SQL 파라미터화
- 명령어 인젝션 방지

### 데이터 보호
- 비밀번호 해싱 (bcrypt/argon2)
- 민감 데이터 암호화
- 로깅에서 민감 정보 제외

## 출력 형식

```
🔒 Security Review Report

CRITICAL Issues:
  1. [AUTH] /api/users 엔드포인트에 인증 누락
  2. [INJECTION] execute(f"SELECT * FROM users WHERE id = {user_id}")

Recommendations:
  1. authMiddleware 추가
  2. 파라미터화 쿼리 사용: execute("SELECT * FROM users WHERE id = ?", [user_id])

Status: ❌ BLOCKED - CRITICAL 이슈 해결 필수
```
```

---

## 6. 구현 순서

### Phase 1: 규칙 추가 (즉시)

1. `.claude/rules/` 디렉토리 생성
2. `security.md` 작성
3. `coding-style.md` 작성
4. `testing.md` 작성
5. `git-workflow.md` 작성

### Phase 2: 훅 설정 (단기)

1. `.claude/hooks.json` 생성
2. PreToolUse 훅 추가 (시크릿 검사, force push 방지)
3. PostToolUse 훅 추가 (자동 린트, 타입 검사)
4. `settings.json` 수정하여 훅 활성화

### Phase 3: 명령어 수정 (중기)

1. `c4-plan.md`에 규칙 로드 추가
2. `c4-run.md`에 자동 리뷰 추가
3. `c4-validate.md`에 보안 검증 추가

### Phase 4: 에이전트 정의 (선택적)

1. `.claude/agents/` 디렉토리 생성
2. `code-reviewer.md` 작성
3. `security-reviewer.md` 작성
4. 기존 에이전트 라우팅과 통합

---

## 7. 검증 방법

### Phase 1 검증
```bash
# 규칙 파일 존재 확인
ls -la .claude/rules/
# security.md, coding-style.md, testing.md, git-workflow.md 존재

# 내용 확인
cat .claude/rules/security.md | head -20
```

### Phase 2 검증
```bash
# 훅 설정 확인
cat .claude/hooks.json | jq '.hooks.PreToolUse'

# 훅 동작 테스트 (시크릿 검사)
echo "password = 'secret123'" > test_secret.py
git add test_secret.py
git commit -m "test"
# → 경고 메시지 출력 확인
rm test_secret.py
```

### Phase 3 검증
```bash
# /c4-plan 실행 시 규칙 로드 확인
# → "📋 활성화된 규칙:" 출력 확인

# /c4-run 실행 시 자동 리뷰 확인
# → "📝 Code Review Report" 출력 확인
```

---

## 8. 롤백 계획

문제 발생 시:

1. **훅 비활성화**:
   ```json
   // .claude/settings.json
   "hooks": { "enabled": false }
   ```

2. **규칙 비활성화**:
   ```json
   // .claude/settings.json
   "rules": { "always": [] }
   ```

3. **이전 버전 복원**:
   ```bash
   git checkout HEAD~1 -- .claude/
   ```

---

## 참고

- **원본 분석**: `.serena/memories/claude-code-best-practices.md`
- **everything-claude-code**: https://github.com/affaan-m/everything-claude-code

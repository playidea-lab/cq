# Git Workflow Rules

> Git 사용 시 준수해야 하는 규칙입니다.

## 커밋 메시지 형식

```
<type>(<scope>): <subject>

[optional body]

[optional footer]
```

### Type (필수)

| Type | 설명 | 예시 |
|------|------|------|
| **feat** | 새로운 기능 추가 | `feat(auth): add JWT token validation` |
| **fix** | 버그 수정 | `fix(api): handle null response correctly` |
| **refactor** | 기능 변경 없이 코드 개선 | `refactor(db): simplify query builder` |
| **test** | 테스트 추가/수정 | `test(user): add unit tests for UserService` |
| **docs** | 문서 변경 | `docs(readme): update installation guide` |
| **chore** | 빌드, 설정 등 기타 | `chore(deps): upgrade pytest to 8.0` |
| **perf** | 성능 개선 | `perf(cache): reduce memory usage by 30%` |
| **style** | 포맷팅, 세미콜론 등 | `style(lint): apply ruff formatting` |
| **ci** | CI/CD 설정 변경 | `ci(github): add Python 3.12 to matrix` |

### Scope (선택)

- 변경 범위를 나타내는 명사
- 예: `auth`, `api`, `db`, `ui`, `config`

### Subject (필수)

- 50자 이내
- 소문자로 시작
- 마침표 없음
- 명령형 사용 ("add" not "added", "fix" not "fixed")

```
# ✅ GOOD
feat(auth): add password reset flow
fix(api): prevent SQL injection in user query
refactor(service): extract common validation logic

# ❌ BAD
feat(auth): Added password reset flow.    # 과거형, 마침표
Fix: bug in API                           # type만, scope 없음, 대문자
updated the user service                  # type 없음
```

### Body (선택)

- 72자에서 줄바꿈
- **무엇**을 **왜** 변경했는지 설명
- "어떻게"는 코드가 설명함

```
fix(auth): prevent session fixation attack

The previous implementation reused session IDs after login,
making it vulnerable to session fixation attacks.

Now generates a new session ID upon successful authentication
as recommended by OWASP guidelines.
```

### Footer (선택)

- Breaking changes: `BREAKING CHANGE: description`
- Issue 참조: `Closes #123`, `Fixes #456`

```
feat(api): change response format to JSON:API

BREAKING CHANGE: API responses now follow JSON:API spec.
Clients must update their parsers.

Closes #789
```

---

## 브랜치 전략

### 브랜치 네이밍

```
<type>/T-<task-id>-<short-description>
```

| Type | 용도 | 예시 |
|------|------|------|
| **feature** | 새 기능 | `feature/T-123-user-authentication` |
| **fix** | 버그 수정 | `fix/T-456-login-redirect-loop` |
| **refactor** | 리팩토링 | `refactor/T-789-simplify-api-client` |
| **docs** | 문서 작업 | `docs/T-012-api-documentation` |
| **c4/w-** | C4 Worker 자동 브랜치 | `c4/w-T-901-0` |

### 브랜치 플로우

```
main (protected)
  │
  ├── feature/T-123-new-feature
  │     ├── commit 1
  │     ├── commit 2
  │     └── PR → main
  │
  └── fix/T-456-bug-fix
        ├── commit 1
        └── PR → main
```

### 규칙

1. **main 브랜치는 항상 배포 가능 상태**
2. **모든 변경은 브랜치에서 시작**
3. **브랜치는 PR로만 main에 병합**
4. **병합 후 브랜치 삭제**

---

## PR (Pull Request) 규칙

### 1. PR 제목은 커밋 메시지 형식 준수

```
feat(auth): add OAuth2 integration
```

### 2. PR 본문 필수 포함 사항

```markdown
## Summary
- 변경 사항 요약 (bullet points)

## Test Plan
- [ ] 테스트 방법 체크리스트

## Related Issues
- Closes #123
```

### 3. 리뷰어 최소 1명 필요

- 코드 변경: 팀원 또는 AI 리뷰어 (code-reviewer agent)
- 보안 관련: 추가로 security-reviewer 필요

### 4. CI 통과 필수

```
✅ Lint passed
✅ Tests passed
✅ Coverage >= 80%
```

### 5. 충돌 해결 후 병합

- Squash merge 권장 (깔끔한 히스토리)
- 복잡한 기능은 일반 merge 허용

### 6. PR 크기 제한

| 크기 | 라인 수 | 권장 |
|------|---------|------|
| **Small** | < 200 | ✅ 권장 |
| **Medium** | 200-500 | ⚠️ 주의 |
| **Large** | > 500 | ❌ 분할 필요 |

---

## 금지 사항

### 1. Force Push to Protected Branches

```bash
# ❌ NEVER
git push --force origin main
git push -f origin main

# ✅ OK (개인 브랜치에서만)
git push --force origin feature/T-123-my-feature
```

### 2. 시크릿/자격 증명 커밋

```bash
# ❌ NEVER commit these
.env
*.pem
*.key
credentials.json
*_secret*
```

`.gitignore` 필수 포함:
```gitignore
# Secrets
.env
.env.*
*.pem
*.key
credentials*.json
*secret*
```

### 3. 대용량 바이너리 파일 커밋

```bash
# ❌ NEVER
*.zip
*.tar.gz
*.exe
*.dll
*.so
model_weights.bin   # ML 모델

# ✅ Use Git LFS for large files
git lfs track "*.bin"
```

### 4. Main 브랜치에 직접 커밋

```bash
# ❌ NEVER
git checkout main
git commit -m "quick fix"
git push origin main

# ✅ ALWAYS use branches
git checkout -b fix/T-999-quick-fix
git commit -m "fix(api): handle edge case"
git push origin fix/T-999-quick-fix
# Then create PR
```

### 5. 리뷰 없이 병합

```bash
# ❌ NEVER
gh pr merge --admin  # 리뷰 우회

# ✅ ALWAYS wait for review
gh pr merge --squash  # 리뷰 승인 후
```

### 6. 불완전한 커밋

```bash
# ❌ NEVER
git commit -m "WIP"
git commit -m "fix"
git commit -m "update"
git commit -m "asdf"

# ✅ ALWAYS meaningful commits
git commit -m "feat(auth): add email verification step"
```

---

## 유용한 명령어

```bash
# 커밋 수정 (push 전)
git commit --amend -m "new message"

# 마지막 n개 커밋 정리 (push 전)
git rebase -i HEAD~3

# 브랜치 최신화
git fetch origin
git rebase origin/main

# 작업 임시 저장
git stash
git stash pop

# 커밋 기록 확인
git log --oneline -10

# 변경 사항 확인
git diff --staged
```

---

## C4 Worker 브랜치

C4 시스템 사용 시 자동 생성되는 브랜치:

```
c4/w-T-{task-id}
```

예: `c4/w-T-901-0`, `c4/w-T-902-0`

### Worker 브랜치 규칙

1. **Worker가 자동 생성/전환**
2. **태스크 완료 후 자동 병합 (검증 통과 시)**
3. **수동 수정 금지** (Worker가 관리)

---

## 참고 자료

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Git Flow](https://nvie.com/posts/a-successful-git-branching-model/)
- [GitHub Flow](https://docs.github.com/en/get-started/quickstart/github-flow)

---
name: c4-release
description: |
  Generate release notes and changelogs from Git commit history. Analyzes commits
  since last tag using Conventional Commits format, categorizes changes (features,
  fixes, breaking changes, etc.), generates CHANGELOG.md, suggests semantic version
  bump, and optionally creates Git tags. Use when the user wants to create a release,
  generate changelog, or prepare version tags. Triggers: "릴리스", "변경 로그",
  "버전 태그", "create release", "generate changelog", "release notes",
  "prepare version".
---

# C4 Release - Changelog Generator

**Git 커밋 히스토리 기반으로 릴리즈 노트를 자동 생성**합니다.

## Instructions

### 1. 버전 정보 수집

```bash
# 최근 태그 확인
git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"

# 태그 이후 커밋 수
git rev-list $(git describe --tags --abbrev=0 2>/dev/null || echo "HEAD~50")..HEAD --count
```

### 2. 커밋 분석

```bash
# 태그 이후 모든 커밋 (또는 최근 50개)
git log $(git describe --tags --abbrev=0 2>/dev/null || echo "HEAD~50")..HEAD \
  --pretty=format:"%h|%s|%an|%ad" --date=short
```

### 3. Conventional Commits 분류

커밋 메시지를 다음 카테고리로 분류:

| Prefix | 카테고리 | 설명 |
|--------|----------|------|
| `feat:` | ✨ Features | 새 기능 |
| `fix:` | 🐛 Bug Fixes | 버그 수정 |
| `perf:` | ⚡ Performance | 성능 개선 |
| `refactor:` | ♻️ Refactoring | 리팩토링 |
| `docs:` | 📚 Documentation | 문서 |
| `test:` | 🧪 Tests | 테스트 |
| `chore:` | 🔧 Chores | 기타 작업 |
| `BREAKING CHANGE` | 💥 Breaking Changes | 호환성 변경 |

### 4. CHANGELOG 생성

다음 형식으로 `CHANGELOG.md` 업데이트:

```markdown
# Changelog

## [v0.2.0] - 2025-01-21

### 💥 Breaking Changes
- **api**: 인증 엔드포인트 변경 (#123)

### ✨ Features
- **rules**: Code Simplification 규칙 추가 (#456)
- **rules**: Frontend Design 규칙 추가 (#457)

### 🐛 Bug Fixes
- **checkpoint**: checkpoint_as_task 상태 전환 버그 수정 (#458)

### ⚡ Performance
- **worker**: 병렬 처리 성능 개선

### 📚 Documentation
- README 업데이트

---

## [v0.1.0] - 2025-01-15
...
```

### 5. 버전 결정

| 변경 유형 | 버전 증가 |
|----------|----------|
| Breaking Changes | Major (1.0.0 → 2.0.0) |
| Features | Minor (1.0.0 → 1.1.0) |
| Bug Fixes only | Patch (1.0.0 → 1.0.1) |

### 6. 태그 생성 + Push

CHANGELOG.md 커밋 후 자동으로 태그를 생성하고 origin에 push합니다.

```bash
# 태그 생성
git tag -a vX.Y.Z -m "Release vX.Y.Z"

# main + 태그 push
git push origin main
git push origin vX.Y.Z
```

`--no-push` 플래그 명시 시 로컬 태그 생성까지만 수행.

## Usage

```
/c4-release            # CHANGELOG → 태그 → push (기본)
/c4-release v0.2.0    # 특정 버전 지정
/c4-release --dry-run  # 미리보기만 (커밋/태그/push 없음)
/c4-release --no-push  # 로컬 태그 생성까지만 (push 생략)
```

## Output 예시

```
📦 Release Summary
==================
Version: v0.1.0 → v0.2.0 (Minor)
Commits: 23
Period: 2025-01-15 ~ 2025-01-21

📝 Changes:
- 💥 Breaking: 1
- ✨ Features: 5
- 🐛 Fixes: 8
- ⚡ Performance: 2
- 📚 Docs: 4
- 🔧 Chores: 3

📄 CHANGELOG.md updated + committed
🏷️  Tag v0.2.0 created
🚀 Pushed: origin/main, origin/v0.2.0
```

## 자동화 옵션

### GitHub Release 연동

```bash
# GitHub CLI로 릴리즈 생성
gh release create v0.2.0 \
  --title "v0.2.0" \
  --notes-file RELEASE_NOTES.md
```

### CI/CD 연동

```yaml
# .github/workflows/release.yml
on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          body_path: RELEASE_NOTES.md
```

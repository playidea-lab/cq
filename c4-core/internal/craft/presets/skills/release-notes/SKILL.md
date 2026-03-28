---
name: release-notes
description: |
  Git 커밋 로그를 분석해 릴리즈 노트(changelog)를 자동 생성.
  트리거: "릴리즈 노트", "release notes", "changelog", "변경 이력"
allowed-tools: Read, Write, Bash
---
# Release Notes

Git 커밋 이력을 분석해 릴리즈 노트를 작성합니다.

## 실행 순서

### Step 1: 커밋 범위 확인

```bash
# 마지막 태그 확인
git describe --tags --abbrev=0

# 이전 릴리즈 이후 커밋 목록
git log v<last-tag>..HEAD --oneline --no-merges
```

또는 날짜 기준:
```bash
git log --since="2024-01-01" --until="2024-02-01" --oneline --no-merges
```

### Step 2: 커밋 분류

Conventional Commits 기준으로 분류:

```bash
# 타입별 커밋 추출
git log v<last-tag>..HEAD --oneline --no-merges --pretty="%s" | sort
```

분류 기준:
- `feat:` → 새 기능
- `fix:` → 버그 수정
- `perf:` → 성능 개선
- `refactor:` → 리팩토링 (사용자 영향 없음)
- `docs:` → 문서 변경
- `chore:`, `ci:`, `build:` → 내부 변경 (노트 제외 가능)
- `BREAKING CHANGE` 포함 → 주요 변경

### Step 3: 릴리즈 노트 생성

```markdown
# Changelog

## [v<버전>] — YYYY-MM-DD

### 주요 변경 (Breaking Changes)
- ...

### 새 기능
- feat(scope): 기능 설명 (#PR번호)

### 버그 수정
- fix(scope): 수정 내용 (#PR번호)

### 성능 개선
- perf(scope): 개선 내용

### 기타
- chore: 의존성 업데이트
```

### Step 4: 버전 결정 (Semantic Versioning)

| 변경 유형 | 버전 증가 |
|----------|----------|
| BREAKING CHANGE | Major (1.x.x → 2.0.0) |
| 새 기능 (feat) | Minor (1.0.x → 1.1.0) |
| 버그 수정 (fix) | Patch (1.0.0 → 1.0.1) |

```bash
# 현재 버전
git describe --tags --abbrev=0

# 새 태그 생성
git tag -a v<새버전> -m "Release v<새버전>"
git push origin v<새버전>
```

### Step 5: 노트 게시

```bash
# GitHub Releases
gh release create v<버전> \
  --title "v<버전> — <요약>" \
  --notes-file CHANGELOG.md

# CHANGELOG.md 업데이트
# 파일 상단에 새 섹션 추가
```

## 출력 형식

```
## 릴리즈 노트 — v<버전>

**릴리즈 날짜**: YYYY-MM-DD
**이전 버전**: v<이전버전>
**커밋 수**: N개

### 하이라이트
<가장 중요한 변경 1-3줄>

### 전체 변경사항
<자동 생성된 분류 목록>
```

# CUSTOMIZE: 버전 관리 전략, 게시 채널 (Slack, GitHub, GitLab), CHANGELOG.md 위치
# 예: CHANGELOG.md가 docs/ 하위인 경우, GitHub Releases 자동 생성 설정

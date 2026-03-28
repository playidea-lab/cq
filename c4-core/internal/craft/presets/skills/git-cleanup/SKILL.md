---
name: git-cleanup
description: |
  브랜치 정리 워크플로우. Stale 브랜치 감지, 병합 완료 브랜치 삭제, remote 동기화.
  트리거: "브랜치 정리", "git cleanup", "stale 브랜치", "오래된 브랜치 삭제"
allowed-tools: Read, Bash
---
# Git Cleanup

로컬/원격 브랜치를 점검하고 안전하게 정리합니다.

## 실행 순서

### Step 1: 현재 상태 파악

```bash
git fetch --prune
git branch -vv
git branch -r
```

- 로컬 브랜치 목록과 최근 커밋 날짜 확인
- remote tracking이 `gone`인 브랜치 식별

### Step 2: 병합 완료 브랜치 탐지

```bash
# main에 병합된 브랜치 목록
git branch --merged main | grep -v '^\*\|main\|master\|develop'

# 원격도 함께 확인
git branch -r --merged main | grep -v 'main\|master\|develop\|HEAD'
```

### Step 3: Stale 브랜치 기준 적용

다음 조건 중 하나라도 해당되면 stale:

- [ ] 마지막 커밋 90일 초과
- [ ] `git branch --merged`에 포함
- [ ] remote tracking이 `gone`

```bash
# 날짜 기준 확인 (90일)
git for-each-ref --sort=committerdate refs/heads/ \
  --format='%(committerdate:short) %(refname:short)'
```

### Step 4: 삭제 전 확인

삭제 대상 목록을 출력하고 확인:

```
## 삭제 예정 브랜치

### 로컬 (병합 완료)
- feature/old-login (2024-01-15, merged)
- fix/typo-readme (2024-02-01, merged)

### 로컬 (remote gone)
- feature/abandoned (2024-03-01, remote gone)
```

### Step 5: 안전 삭제

```bash
# 로컬 — 병합된 브랜치만 (안전)
git branch -d <branch-name>

# 로컬 — 강제 삭제 (미병합 stale)
git branch -D <branch-name>

# 원격 — 주의해서 실행
git push origin --delete <branch-name>
```

### Step 6: 결과 확인

```bash
git branch -vv
git remote prune origin --dry-run
```

## 안전 규칙

- `main`, `master`, `develop`, `release/*` 는 절대 삭제하지 않는다
- `git branch -D`(강제)는 명시적 확인 후에만 실행
- 원격 삭제 전 팀원에게 공지

# CUSTOMIZE: 보호 브랜치 목록 추가
# 예: protected_branches=("main" "develop" "staging" "release/*")
# 예: stale 기준 일수 변경 (기본 90일)

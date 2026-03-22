---
name: git-worktree
description: |
  Git worktree 기반 병렬 작업 가이드. 여러 브랜치를 동시에 체크아웃하여 병렬로 작업하거나,
  리뷰하면서 구현을 계속하는 워크플로우를 안내합니다. "worktree", "병렬 작업", "브랜치 동시",
  "git worktree", "격리된 작업", "parallel branch" 등의 요청에 트리거됩니다.
---

# Git Worktree

Git worktree 기반 병렬 작업 가이드.

## 트리거

"worktree", "병렬 작업", "git worktree", "격리된 작업", "parallel branch"

## 개념

worktree = 같은 repo에서 **여러 브랜치를 동시에 체크아웃**하는 기능.

```
~/project/              ← main (기본 worktree)
~/.worktrees/feature-a/ ← feature-a 브랜치
~/.worktrees/hotfix-1/  ← hotfix 브랜치
```

stash나 commit 없이 브랜치 전환. 각 worktree는 독립된 작업 디렉토리.

## Steps

### 1. Worktree 생성

```bash
# 새 브랜치로 worktree 생성
git worktree add ../feature-login -b feature/login

# 기존 브랜치를 worktree로
git worktree add ../hotfix hotfix/urgent-fix

# 특정 경로에 생성
git worktree add ~/.worktrees/review-pr42 origin/pr-42
```

### 2. 병렬 작업 패턴

**패턴 A: 구현 + 리뷰 동시**
```
~/project/           ← 구현 계속
~/.worktrees/review/ ← PR 리뷰 (읽기 전용)
```

**패턴 B: 여러 기능 병렬 개발**
```
~/project/             ← main (안정)
~/.worktrees/feature-a/ ← 기능 A
~/.worktrees/feature-b/ ← 기능 B
```

**패턴 C: 핫픽스 긴급 대응**
```
~/project/           ← 현재 작업 중단 없이
~/.worktrees/hotfix/ ← 핫픽스 작성 → 머지 → 삭제
```

### 3. Worktree 관리

```bash
# 목록 확인
git worktree list

# worktree 삭제 (작업 완료 후)
git worktree remove ../feature-login

# 정리 (삭제된 worktree 참조 제거)
git worktree prune
```

### 4. 주의사항

- **같은 브랜치를 두 worktree에서 동시 체크아웃 불가**
- worktree 간 `.git/` 공유 — commit, stash, config 공유됨
- `node_modules/`, `.venv/` 등 의존성은 worktree별로 설치 필요
- IDE가 여러 worktree를 동시에 열 수 있음 (별도 창)

### 5. CQ 연동

CQ의 Worker는 내부적으로 worktree를 사용합니다:
```bash
# CQ Worker가 자동으로:
git worktree add .c4/worktrees/worker-xxx -b c4/w-T-001-0
# → 독립 worktree에서 태스크 구현
# → 완료 후 main으로 merge → worktree 삭제
```

수동으로 worktree를 만들어 작업하고 싶으면:
```bash
git worktree add ../my-task -b feature/my-task
cd ../my-task
# 작업 후
git add . && git commit -m "feat: ..."
cd ../project
git merge feature/my-task
git worktree remove ../my-task
```

## 안티패턴

- worktree를 만들고 안 지움 (디스크 낭비, `git worktree prune`으로 정리)
- 같은 파일을 두 worktree에서 동시 수정 (merge conflict 확률 높음)
- worktree에서 `git checkout` (다른 브랜치로 전환하면 혼란)

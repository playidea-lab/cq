---
name: hotfix-flow
description: |
  긴급 수정 워크플로우. hotfix 브랜치 생성 → 수정 → 테스트 → main + develop 동시 merge.
  트리거: "핫픽스", "hotfix", "긴급 수정", "프로덕션 버그", "hotfix-flow"
allowed-tools: Bash, Read, Edit, Grep
---
# Hotfix Flow

프로덕션 버그를 빠르고 안전하게 수정하는 워크플로우입니다.

## 실행 순서

### Step 1: 현황 파악 (5분 이내)

```bash
# 현재 상태 확인
git status
git log --oneline -3

# 버그 재현 확인
# 에러 로그, 스택 트레이스 수집
```

버그의 범위와 영향도를 먼저 파악한다.
- 어떤 기능이 깨졌나?
- 영향받는 사용자 범위는?
- 롤백이 더 빠른가, 수정이 더 빠른가?

### Step 2: hotfix 브랜치 생성

```bash
# main에서 직접 분기
git checkout main
git pull origin main
git checkout -b hotfix/<짧은-설명>
# 예: hotfix/fix-auth-token-expiry
```

### Step 3: 최소 수정

- 버그만 수정한다. 리팩토링, 개선 금지.
- 수정 범위를 최소화할수록 좋다.

```bash
# 수정 후 빌드 확인
# CUSTOMIZE: 프로젝트 빌드 명령으로 교체
go build ./... && go vet ./...
```

### Step 4: 테스트

```bash
# 회귀 테스트 전체 실행
# CUSTOMIZE: 프로젝트 테스트 명령으로 교체
go test ./... -count=1

# 버그 재현 케이스 테스트 추가 (가능하면)
```

### Step 5: 커밋 & Merge

```bash
git add <수정된 파일>
git commit -m "fix(<scope>): <버그 한 줄 설명>

Fixes: <이슈 번호>
Impact: <영향 범위>
Root cause: <원인>"

# main에 merge
git checkout main
git merge --no-ff hotfix/<짧은-설명>
git tag -a v<버전> -m "hotfix: <설명>"

# develop에도 반영 (있는 경우)
git checkout develop
git merge --no-ff hotfix/<짧은-설명>

# 브랜치 정리
git branch -d hotfix/<짧은-설명>
```

### Step 6: 배포 & 검증

```bash
git push origin main
git push origin --tags
```

배포 후:
- [ ] 프로덕션에서 버그 재현 안 됨 확인
- [ ] 관련 메트릭 정상화 확인
- [ ] 사후 분석(Post-mortem) 일정 잡기

# CUSTOMIZE: 팀 알림 채널 설정
# 핫픽스 배포 시 슬랙/팀즈에 알림:
# - 수정 내용
# - 배포 시각
# - 영향받은 기능
# - 담당자

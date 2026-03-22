---
name: deploy
description: |
  PI Lab 표준 배포 워크플로우. 코드 검증, 빌드, 배포, 사후 확인까지 체계적으로 안내합니다.
  프로덕션 배포, 스테이징 배포, 핫픽스 배포 시 반드시 이 스킬을 사용하세요.
  "배포", "deploy", "릴리스", "release", "프로덕션 반영", "canary 배포",
  "배포해줘", "rollback", "hotfix 배포" 등의 요청에 트리거됩니다.
---

# Standard Deploy Workflow

PI Lab 표준 배포 프로세스를 단계별로 가이드합니다.

## 배포 전 체크리스트

### 1. 코드 검증
```bash
# 언어별 검증
# Go
go build ./... && go vet ./... && go test ./...

# Python
uv run ruff check . && uv run pytest

# TypeScript
pnpm lint && pnpm build && pnpm test
```

### 2. 브랜치 상태 확인
- [ ] main 브랜치 최신 상태
- [ ] CI 파이프라인 통과
- [ ] 미병합 MR 없음 (이 배포에 포함될 것)

### 3. 변경 사항 요약
- `git log --oneline <last-tag>..HEAD` 로 변경 목록 확인
- Breaking change 여부 확인
- 마이그레이션 필요 여부 확인

## 배포 프로세스

### Step 1: 버전 태그
```bash
# Semantic Versioning
# Breaking change → major
# 새 기능 → minor
# 버그 수정 → patch
git tag -a v<X.Y.Z> -m "Release v<X.Y.Z>: <요약>"
```

### Step 2: 배포 실행
- CI/CD 파이프라인을 통한 배포 (수동 배포 금지)
- canary 또는 blue-green 전략 사용
- 프로덕션 직접 배포 금지 → staging 먼저

### Step 3: 배포 후 확인
- [ ] health check 통과 (`/healthz`, `/readyz`)
- [ ] 핵심 API 응답 정상
- [ ] 에러율 급증 없음 (모니터링 대시보드)
- [ ] 로그에 예상치 못한 에러 없음

### Step 4: 롤백 준비
- 이전 버전으로 롤백 명령 준비
- 롤백 기준: 에러율 > 1% 또는 latency > 2x

## 배포 후

- CHANGELOG 업데이트 (또는 `/c4-release` 사용)
- 팀 알림 (Slack/Discord)
- 마이그레이션 실행 확인

## 긴급 배포 (Hotfix)

1. `hotfix/<설명>` 브랜치 생성 (main에서)
2. 수정 + 테스트
3. MR → 리뷰 (최소 1명) → 병합
4. 즉시 배포
5. 사후: 원인 분석 문서 작성

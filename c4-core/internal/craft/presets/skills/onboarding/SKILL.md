---
name: onboarding
description: |
  새 팀원 온보딩 가이드. 개발환경 셋업, 코드베이스 투어, 첫 PR까지 단계별 안내.
  트리거: "온보딩", "onboarding", "새 팀원", "개발환경 설정"
allowed-tools: Read, Write, Glob, Grep, Bash
---
# Onboarding

새 팀원이 첫날부터 기여할 수 있도록 안내합니다.

## Phase 1: 개발환경 셋업 (Day 1)

### 필수 도구 설치

```bash
# 버전 확인
go version      # 또는 python --version / node --version
git --version
docker --version
```

```bash
# 저장소 클론
git clone <repo-url>
cd <project>

# 의존성 설치
<install-command>   # go mod tidy / uv sync / pnpm install

# 환경변수 설정
cp .env.example .env
# .env 파일을 팀 시크릿 관리 도구에서 채울 것
```

### 로컬 실행 확인

```bash
<run-command>       # go run . / uv run python main.py / pnpm dev

# 헬스체크
curl http://localhost:<port>/health
```

- [ ] 빌드 성공
- [ ] 로컬 서버 실행
- [ ] 테스트 통과: `<test-command>`

## Phase 2: 코드베이스 투어 (Day 1-2)

### 디렉토리 구조 파악

```bash
tree -L 2 -I 'node_modules|.git|vendor'
```

주요 디렉토리 설명:
- `cmd/` — 진입점
- `internal/` — 핵심 비즈니스 로직
- `pkg/` — 공개 패키지
- `docs/` — 문서

### 핵심 코드 읽기 순서

1. 진입점 (main.go / app.py / index.ts)
2. 라우터/핸들러
3. 서비스 레이어
4. 데이터 레이어 (DB 모델)
5. 설정 파일

### 아키텍처 이해

```bash
# 의존 관계 시각화 (Go)
go doc ./...

# API 엔드포인트 목록
grep -rn "router\|@app.route\|app.get\|app.post" .
```

## Phase 3: 개발 워크플로우 (Day 2-3)

### 브랜치 & PR 규칙

```bash
# 브랜치 생성
git checkout -b feature/<이름>/<기능-설명>

# 커밋 메시지 형식
git commit -m "feat(scope): 기능 설명"
```

### 코드 리뷰 프로세스

1. PR 생성 → 자동 CI 통과 확인
2. 최소 1명 리뷰어 지정
3. 코멘트 반영 후 Approve → Merge

### 테스트 실행

```bash
<test-command>
```

## Phase 4: 첫 기여 (Week 1)

- [ ] `good-first-issue` 라벨 이슈 선택
- [ ] 브랜치 생성 후 구현
- [ ] PR 생성 (체크리스트 작성)
- [ ] 리뷰 반영 후 Merge

## 체크리스트 요약

```
## 온보딩 체크리스트

Day 1:
- [ ] 저장소 클론 및 빌드 성공
- [ ] .env 설정 완료
- [ ] 로컬 실행 확인
- [ ] 테스트 통과

Day 2:
- [ ] 코드베이스 구조 파악
- [ ] 아키텍처 문서 읽기
- [ ] 팀 커뮤니케이션 채널 합류

Week 1:
- [ ] 첫 PR 생성
- [ ] 코드 리뷰 경험
```

# CUSTOMIZE: 프로젝트 특화 도구, 내부 시스템 접근 방법, 팀 채널 정보 추가
# 예: VPN 설정, 내부 NPM 레지스트리, Slack 채널 목록

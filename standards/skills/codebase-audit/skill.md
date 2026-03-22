---
name: codebase-audit
description: |
  코드베이스 전체 감사. 아키텍처, 코드 품질, 보안, 테스트, 성능을 한번에 점검합니다.
  프로젝트 인수인계, 기술 부채 파악, 리팩토링 전 현황 파악 시 이 스킬을 사용하세요.
  "코드베이스 감사", "codebase audit", "전체 점검", "기술 부채 파악",
  "프로젝트 건강도", "코드 품질 전체 리뷰", "인수인계" 등의 요청에 트리거됩니다.
---

# Codebase Audit

코드베이스 전체 감사: 아키텍처 + 품질 + 보안 + 테스트 + 성능.

## 트리거

"코드베이스 감사", "codebase audit", "전체 점검", "기술 부채", "프로젝트 건강도"

## 사용 시점

- 새 프로젝트 인수인계 받을 때
- 기술 부채를 정량적으로 파악하고 싶을 때
- 대규모 리팩토링 전 현황 파악
- 정기 점검 (분기 1회 권장)

## Steps

### 1. 프로젝트 개요 파악

```bash
# 규모
find . -name "*.go" -o -name "*.py" -o -name "*.ts" | xargs wc -l | tail -1
# 의존성
cat go.mod | wc -l       # Go
cat pyproject.toml        # Python
cat package.json | jq '.dependencies | length'  # Node

# 커밋 활동
git log --oneline --since="3 months ago" | wc -l
git shortlog -sn --since="3 months ago" | head -5
```

### 2. 아키텍처 감사

| 확인 항목 | 방법 |
|-----------|------|
| 디렉토리 구조 | `tree -L 2 -d` — 역할별 분리가 명확한가? |
| 의존성 방향 | 순환 참조 없는가? 레이어 위반 없는가? |
| 진입점 | main/cmd 개수, 역할 명확한가? |
| 설정 관리 | 환경별 분리, 시크릿 처리 |
| 외부 의존성 | 버전 고정, 취약점, 라이선스 |

**등급:**
- A: 명확한 레이어, 순환 없음, 문서화됨
- B: 구조 있지만 일부 위반
- C: 혼재, 순환 참조, 문서 없음

### 3. 코드 품질 감사

```bash
# Go
go vet ./...
golangci-lint run

# Python
uv run ruff check .
uv run mypy .

# TypeScript
pnpm lint
tsc --noEmit
```

| 확인 항목 | 기준 |
|-----------|------|
| 린트 경고 | 0이 이상적, <10 허용 |
| 함수 길이 | 50줄 이하 |
| 파일 길이 | 500줄 이하 |
| 중복 코드 | 유사 블록 3회 이상 → 추출 대상 |
| 네이밍 | 의도가 드러나는가? |
| 에러 처리 | 에러 삼키기(swallow) 없는가? |

### 4. 보안 감사

```bash
# 시크릿 스캔
gitleaks detect --source .

# 의존성 취약점
govulncheck ./...           # Go
uv run pip-audit            # Python
pnpm audit                  # Node

# OWASP Top 10 체크리스트
```

| 확인 항목 | 위험도 |
|-----------|--------|
| 하드코딩된 시크릿 | Critical |
| SQL injection | Critical |
| 입력 검증 누락 | High |
| 인증/인가 우회 | High |
| 의존성 CVE | 심각도별 |

### 5. 테스트 감사

```bash
# 커버리지
go test -coverprofile=cover.out ./... && go tool cover -func=cover.out | tail -1
uv run pytest --cov=src --cov-report=term-missing
```

| 확인 항목 | 기준 |
|-----------|------|
| 테스트 존재 | 핵심 로직에 테스트가 있는가? |
| 커버리지 | 수치보다 핵심 경로 커버 여부 |
| 에러 경로 | 실패 케이스 테스트 있는가? |
| 테스트 속도 | 전체 실행 1분 이내 |
| Flaky 테스트 | 간헐적 실패 없는가? |

### 6. 성능 감사

| 확인 항목 | 방법 |
|-----------|------|
| 느린 쿼리 | `EXPLAIN ANALYZE`, slow query log |
| N+1 문제 | ORM 로그에서 반복 쿼리 |
| 메모리 누수 | 프로파일러 (pprof, py-spy) |
| 시작 시간 | cold start 측정 |
| API 지연 | P95 < 500ms? |

### 7. 보고서

```markdown
# 코드베이스 감사 보고서

**프로젝트**: [이름]
**일시**: YYYY-MM-DD
**규모**: N LOC, N 파일, N 의존성

## 종합 점수

| 영역 | 등급 | 주요 이슈 |
|------|------|----------|
| 아키텍처 | A/B/C | |
| 코드 품질 | A/B/C | |
| 보안 | A/B/C | |
| 테스트 | A/B/C | |
| 성능 | A/B/C | |

## Critical 이슈 (즉시 수정)
1. ...

## High 이슈 (다음 스프린트)
1. ...

## Medium 이슈 (백로그)
1. ...

## 권장 개선 로드맵
1. [1주] Critical 보안 이슈 수정
2. [2주] 테스트 커버리지 핵심 경로 보강
3. [1개월] 아키텍처 레이어 위반 정리
```

## 안티패턴

- 감사 없이 "잘 돌아가니까 괜찮다"
- 수치만 보고 맥락 무시 (커버리지 80%여도 핵심이 빠지면 무의미)
- 감사 후 보고서만 작성하고 개선 안 함
- 모든 이슈를 한번에 고치려 함 (우선순위 필수)

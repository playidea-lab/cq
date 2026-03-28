---
name: code-review
description: |
  6축 코드 리뷰 체크리스트. PR diff 또는 변경 파일을 읽고 Correctness, Security, Reliability, Observability, Test Coverage, Readability 순서로 검토.
  트리거: "코드 리뷰", "리뷰해줘", "code review", "변경사항 검토"
allowed-tools: Read, Glob, Grep, Bash
---
# Code Review

변경된 파일을 읽고 6축 체크리스트로 리뷰합니다.

## 실행 순서

### Step 1: 변경 파일 파악

```bash
git diff --name-only HEAD~1 HEAD
# 또는 PR이면:
git diff --name-only origin/main...HEAD
```

파일 목록을 확인한 뒤 각 파일을 Read로 읽는다.

### Step 2: 6축 체크리스트 적용

각 축을 순서대로 검토하고 발견사항을 기록한다.

#### 1. Correctness (정확성)
- [ ] 요구사항/DoD를 충족하는가?
- [ ] 엣지 케이스(nil, empty, overflow)를 처리하는가?
- [ ] off-by-one, race condition 없는가?
- [ ] 함수 반환값이 모두 처리되는가?

#### 2. Security (보안)
- [ ] 사용자 입력을 검증하는가?
- [ ] 시크릿이 코드에 하드코딩되지 않았는가?
- [ ] 인증/권한 체크가 적절한가?
- [ ] SQL/Command injection 방어가 있는가?

#### 3. Reliability (신뢰성)
- [ ] 외부 호출에 timeout이 있는가?
- [ ] 실패 시 graceful degradation이 가능한가?
- [ ] 리소스 누수 없는가? (파일, DB 커넥션, goroutine)
- [ ] retry/circuit breaker가 필요한 곳에 있는가?

#### 4. Observability (관측성)
- [ ] 에러가 적절히 로깅되는가?
- [ ] 중요 상태 변경이 로그에 남는가?
- [ ] 장애 시 원인 파악이 가능한가?

#### 5. Test Coverage (테스트)
- [ ] 핵심 로직에 단위 테스트가 있는가?
- [ ] 에러 경로도 테스트하는가?
- [ ] 테스트가 독립적이고 반복 가능한가?

#### 6. Readability (가독성)
- [ ] 변수/함수명이 의도를 드러내는가?
- [ ] 불필요한 복잡도가 없는가?
- [ ] 주석이 "왜"를 설명하는가?

### Step 3: 리뷰 결과 출력

```
## 코드 리뷰 결과

**대상**: <파일 목록>

### Approve / Request Changes / Comment

#### 필수 수정 (Blocking)
- ...

#### 개선 제안 (Non-blocking)
- ...

#### 잘된 점
- ...
```

## 리뷰 결정 기준

| 결정 | 조건 |
|------|------|
| **Approve** | 1-4축 모두 통과, 5-6축 경미한 코멘트만 |
| **Request Changes** | 1-4축(정확성/보안/신뢰성/관측성)에 문제 |
| **Comment** | 5-6축(테스트/가독성) 개선 제안. 병합 차단 안 함 |

# CUSTOMIZE: 팀 리뷰 기준 추가
# 아래에 팀 특화 체크 항목을 추가하세요:
# - 특정 패턴 금지 (예: sync.Mutex 대신 channel 사용)
# - 성능 임계값 (예: DB 쿼리 100ms 이하)
# - 아키텍처 규칙 (예: service → repository 레이어만 허용)

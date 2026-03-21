# Code Review Criteria

> 코드 리뷰 시 확인하는 6개 축. 우선순위는 soul.md 참조.

## 1. Correctness (정확성)

- 요구사항/DoD를 충족하는가?
- 엣지 케이스를 처리하는가?
- off-by-one, null pointer, race condition 없는가?

## 2. Security (보안)

- 입력 검증이 있는가?
- 시크릿이 코드에 없는가?
- 인증/권한 체크가 적절한가?
- SQL injection, XSS, CSRF 방어가 있는가?

## 3. Reliability (신뢰성)

- 실패 시 graceful degradation이 가능한가?
- timeout, retry, circuit breaker가 적절한가?
- 리소스 누수 없는가? (파일 핸들, DB 커넥션, goroutine)

## 4. Observability (관측성)

- 적절한 로깅이 있는가? (에러, 중요 상태 변경)
- 메트릭 수집 포인트가 있는가?
- 장애 시 원인 파악이 가능한가?

## 5. Test Coverage (테스트)

- 핵심 로직에 단위 테스트가 있는가?
- 에러 경로도 테스트하는가?
- 테스트가 독립적이고 반복 가능한가?

## 6. Readability (가독성)

- 의도가 명확한 변수/함수명인가?
- 불필요한 복잡도가 없는가?
- 주석이 "왜"를 설명하는가? ("무엇"이 아니라)

## 리뷰 결정 기준

- **Approve**: 6축 모두 통과 또는 경미한 코멘트만.
- **Request Changes**: 1-4축(정확성/보안/신뢰성/관측성)에 문제.
- **Comment**: 5-6축(테스트/가독성)에 개선 제안. 병합 차단하지 않음.

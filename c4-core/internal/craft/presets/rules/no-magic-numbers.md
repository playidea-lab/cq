# Rule: no-magic-numbers
> 코드에 의미 없는 숫자 리터럴 금지. 모든 매직 넘버는 이름 있는 상수로 추출한다.

## 규칙

- 숫자 리터럴을 직접 비교/연산에 사용 금지
- 예외: `0`, `1`, `-1` (인덱스, 길이 체크 등 명확한 경우)
- 예외: 단위 테스트 내부의 픽스처 데이터
- 상수명은 의도를 드러내야 함 (`SEVEN` 금지, `MAX_RETRY_COUNT` 허용)

## 나쁜 예 / 좋은 예

### Go
```go
// 금지
if attempts > 3 {
    return ErrTooManyRetries
}
time.Sleep(30 * time.Second)

// 허용
const (
    MaxRetryCount    = 3
    RetryBackoffSecs = 30 * time.Second
)

if attempts > MaxRetryCount {
    return ErrTooManyRetries
}
time.Sleep(RetryBackoffSecs)
```

### TypeScript
```typescript
// 금지
if (password.length < 8) throw new Error('too short');
const expiry = Date.now() + 86400000;

// 허용
const MIN_PASSWORD_LENGTH = 8;
const ONE_DAY_MS = 24 * 60 * 60 * 1000;

if (password.length < MIN_PASSWORD_LENGTH) throw new Error('too short');
const expiry = Date.now() + ONE_DAY_MS;
```

### Python
```python
# 금지
if score >= 90:
    grade = 'A'
elif score >= 80:
    grade = 'B'

# 허용
GRADE_A_THRESHOLD = 90
GRADE_B_THRESHOLD = 80

if score >= GRADE_A_THRESHOLD:
    grade = 'A'
elif score >= GRADE_B_THRESHOLD:
    grade = 'B'
```

## 상수 배치 규칙

- 파일 상단 또는 전용 `constants.go` / `constants.ts` 파일
- 관련 상수는 그룹화하고 주석으로 목적 설명
- 패키지 레벨 상수 vs 함수 레벨 상수 적절히 분리

# CUSTOMIZE: 허용 예외 숫자 추가
# 예: HTTP 상태코드 200, 404, 500은 허용 (표준이 명확)
# 예: 비트 연산의 2의 거듭제곱 허용

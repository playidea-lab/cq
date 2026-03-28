# Rule: no-console-log
> 디버그용 console.log/print 출력 금지. 프로덕션 코드에는 structured logger를 사용한다.

## 규칙

- `console.log`, `console.debug`, `console.warn` (TypeScript/JavaScript) 금지
- `print()`, `pprint()` (Python) 프로덕션 코드에서 금지
- `fmt.Println`, `fmt.Printf` (Go) 로깅 목적 사용 금지
- 모든 로그는 레벨, 컨텍스트, 구조화된 필드를 갖춰야 한다

## 허용 예외

- 테스트 코드: `console.log` 허용 (단, CI에서 `-v` 없으면 출력 안 됨)
- CLI 도구의 사용자 출력: `fmt.Fprintln(os.Stdout, ...)` 허용 (로그가 아닌 출력)
- 스크립트/일회성 코드: 허용

## 대체 방법

### TypeScript/JavaScript
```typescript
// 금지
console.log('user created', userId);

// 허용
logger.info('user created', { userId });
logger.error('failed to create user', { userId, error: err.message });
```

### Python
```python
# 금지
print(f"user created: {user_id}")

# 허용
import logging
logger = logging.getLogger(__name__)
logger.info("user created", extra={"user_id": user_id})
```

### Go
```go
// 금지
fmt.Println("user created:", userID)

// 허용
slog.Info("user created", "user_id", userID)
logger.Error("failed to create user", "user_id", userID, "err", err)
```

# CUSTOMIZE: 팀 로거 라이브러리 지정
# 예: Winston, Pino (Node.js)
# 예: loguru, structlog (Python)
# 예: zap, slog, zerolog (Go)
# 로거 초기화 코드 예시를 여기에 추가하세요.

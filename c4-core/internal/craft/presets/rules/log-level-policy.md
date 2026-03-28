# Rule: log-level-policy
> 로그 레벨 기준을 명확히 하고 일관되게 사용한다.

## 레벨별 기준

| 레벨 | 언제 사용 | 알림 여부 |
|------|----------|----------|
| ERROR | 즉각 조치 필요한 장애. 요청 실패, 데이터 손실 위험 | 온콜 알림 |
| WARN | 이상하지만 서비스는 계속. 재시도 성공, 설정 누락 | 이메일/슬랙 |
| INFO | 정상 운영 이벤트. 서버 시작, 주요 트랜잭션 완료 | 없음 |
| DEBUG | 개발 중 상세 정보. 쿼리 파라미터, 중간 계산 | 없음 |

## 규칙

- ERROR: 반드시 에러 객체 + 컨텍스트 포함
- WARN: 조치가 필요하지 않으면 INFO 사용
- INFO: 운영자가 알아야 할 것만. 과도한 INFO는 노이즈
- DEBUG: 프로덕션에서 기본 비활성화
- 개인정보(이메일, 주민번호 등)는 어떤 레벨에도 포함 금지

## 예시

### Go (slog)
```go
// ERROR — 에러 + 컨텍스트
slog.Error("payment failed", "order_id", orderID, "err", err)

// WARN — 비정상이지만 처리됨
slog.Warn("rate limit hit, retrying", "attempt", attempt, "endpoint", url)

// INFO — 주요 이벤트
slog.Info("order created", "order_id", orderID, "user_id", userID)

// DEBUG — 상세 (개발용)
slog.Debug("cache miss", "key", cacheKey)
```

### Python
```python
# ERROR
logger.error("DB connection failed", extra={"host": host}, exc_info=True)

# WARN
logger.warning("retry attempt %d for %s", attempt, url)

# INFO
logger.info("user signed up", extra={"user_id": user_id})

# DEBUG
logger.debug("query params: %s", params)
```

## 구조화 로그 필수 필드

```json
{
  "timestamp": "2024-01-01T00:00:00Z",
  "level": "ERROR",
  "message": "payment failed",
  "service": "order-service",
  "trace_id": "abc123",
  "order_id": "ord_456"
}
```

# CUSTOMIZE: 팀 로거 라이브러리, 알림 임계값, 필수 로그 필드, 샘플링 설정 (DEBUG)

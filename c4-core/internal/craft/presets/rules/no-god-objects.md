# Rule: no-god-objects
> 단일 책임 원칙. 하나의 클래스/구조체는 하나의 책임만 가진다.

## 규칙

- 메서드/함수가 10개 초과인 struct/class는 분리 검토
- 파일이 300줄 초과 시 분리 검토
- "Manager", "Util", "Helper", "Service" 단독 이름 주의 (범위가 너무 넓음)

## 탐지 신호

- 이름에 "And"가 들어감: `UserAndOrderManager`
- 생성자 인자가 5개 초과
- 다른 레이어가 모두 이 클래스를 알고 있음
- 변경 이유가 2가지 이상: "사용자 규칙이 바뀌면", "주문 규칙이 바뀌면"

## 예시

```go
// 금지 — God Struct
type AppManager struct {
    db           *sql.DB
    redis        *redis.Client
    emailClient  EmailClient
    smsClient    SMSClient
    s3Client     S3Client
    // 20개 메서드...
}

// 허용 — 책임 분리
type UserService struct {
    repo UserRepository
    mailer Mailer
}

type OrderService struct {
    repo  OrderRepository
    stock StockChecker
}

type NotificationService struct {
    email EmailClient
    sms   SMSClient
}
```

## 분리 방법

1. **책임 목록 나열**: 이 클래스가 하는 일을 모두 나열
2. **응집도 기준 그룹화**: 함께 변경되는 것끼리 묶기
3. **인터페이스 추출**: 외부 의존성을 인터페이스로 분리
4. **단계적 분리**: 한 번에 전부 분리하지 말고 가장 명확한 것부터

## 예외

- 진입점 (main, app.go): 의존성 조립 목적으로 여러 컴포넌트 보유 가능
- 설정 구조체: 여러 설정 항목을 하나에 모으는 것은 정상

# CUSTOMIZE: 허용 메서드 수, 파일 크기 기준, 팀 아키텍처 레이어 규칙

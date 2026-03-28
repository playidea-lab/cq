# Rule: max-function-length
> 함수/메서드는 50줄 이하로 유지한다. 초과 시 책임을 분리한다.

## 규칙

- 함수 본문 50줄 초과 시 분리 고려
- 100줄 초과 시 반드시 분리
- 빈 줄, 주석 포함 계산
- 테스트 함수는 예외 (최대 100줄)

## 이유

- 긴 함수는 여러 책임을 지는 신호
- 테스트하기 어렵고, 변경 시 영향 범위가 크다
- 읽기 어렵고 코드 리뷰 효율 저하

## 분리 방법

```go
// 금지 — 120줄짜리 함수
func ProcessOrder(order Order) error {
    // 유효성 검사 (20줄)
    // 재고 확인 (20줄)
    // 결제 처리 (30줄)
    // 알림 발송 (30줄)
    // 로그 기록 (20줄)
}

// 허용 — 책임 분리
func ProcessOrder(order Order) error {
    if err := validateOrder(order); err != nil {
        return fmt.Errorf("validation: %w", err)
    }
    if err := checkInventory(order); err != nil {
        return fmt.Errorf("inventory: %w", err)
    }
    if err := chargePayment(order); err != nil {
        return fmt.Errorf("payment: %w", err)
    }
    go sendNotification(order)  // 비동기
    return logOrderComplete(order)
}
```

## 탐지 방법

```bash
# Go: 긴 함수 탐지 (간이)
awk '/^func /{start=NR} start && NR-start>50{print FILENAME ":" start " (길이: " NR-start "줄)"; start=0}' \
  $(find . -name "*.go" | grep -v _test)

# 또는 gocognit 도구
go install github.com/uudashr/gocognit/cmd/gocognit@latest
gocognit -over 15 .
```

## 예외

- 인터페이스 구현체의 단순 위임 함수
- 생성되는 코드 (protobuf, mock 등)
- 테스트 셋업 함수 (TestMain 등)

# CUSTOMIZE: 허용 최대 줄 수 변경, 언어별 예외 규칙, 린트 도구 통합 방법

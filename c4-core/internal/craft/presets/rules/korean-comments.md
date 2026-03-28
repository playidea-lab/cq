# Rule: korean-comments
> 코드 주석은 한국어로 작성한다. 변수명/함수명은 영어 유지.

## 규칙

- 인라인 주석 (`//`, `#`): 한국어 작성
- 블록 주석 (`/* */`, `"""..."""`): 한국어 작성
- JSDoc/GoDoc/docstring: 한국어 작성
- 변수명, 함수명, 클래스명: 영어 유지 (국제 표준 코드베이스와 호환성)
- 커밋 메시지: 영어 (Conventional Commits 형식)
- PR 설명: 한국어 허용

## 예시

### Go
```go
// 사용자 인증 토큰을 검증하고 클레임을 반환합니다.
// 토큰 만료 시 ErrTokenExpired를 반환합니다.
func validateToken(token string) (*Claims, error) {
    // 서명 키로 토큰 파싱
    parsed, err := jwt.Parse(token, keyFunc)
    if err != nil {
        // 만료된 토큰은 별도 에러로 구분
        if errors.Is(err, jwt.ErrTokenExpired) {
            return nil, ErrTokenExpired
        }
        return nil, fmt.Errorf("토큰 파싱 실패: %w", err)
    }
    // ...
}
```

### Python
```python
def calculate_discount(price: float, user_tier: str) -> float:
    """
    사용자 등급에 따른 할인 금액을 계산합니다.

    Args:
        price: 원래 가격 (0 이상)
        user_tier: 사용자 등급 ('vip', 'regular', 'new')

    Returns:
        할인 적용 후 최종 가격
    """
    # VIP는 20% 할인, 일반 회원은 5% 할인
    discount_rate = {"vip": 0.20, "regular": 0.05, "new": 0.0}
    rate = discount_rate.get(user_tier, 0.0)
    return price * (1 - rate)
```

## 예외

- 오픈소스 기여 또는 외부 공개 코드: 영어 주석
- 서드파티 라이브러리 인터페이스 구현 시: 원본 주석 언어 유지

# CUSTOMIZE: 주석 언어 예외 설정
# 예: 특정 패키지는 영어 주석 (오픈소스 공개용)
# 예: 외부 협업자가 있는 경우 이중 언어 주석 허용

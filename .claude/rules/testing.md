# Testing Rules

> 모든 코드에 적용되는 테스트 가이드라인입니다.

## 단계별 커버리지 요구사항

| 단계 | 커버리지 | 설명 | 병합 가능 |
|------|----------|------|-----------|
| **Exploration** | 0% | 가설 검증, 프로토타입 | ❌ (브랜치만) |
| **Validation** | 50-70% | 핵심 로직 검증 | ⚠️ (리뷰 필요) |
| **Production** | 80%+ | 안정성 확보 | ✅ |

```
Exploration (탐색)
    ↓ 가설 검증됨
Validation (검증)
    ↓ 50%+ 커버리지 달성
Production (프로덕션)
    ↓ 80%+ 커버리지 달성
✅ 배포 가능
```

---

## TDD 사이클 (RED-GREEN-REFACTOR)

### 1. RED: 실패하는 테스트 먼저 작성

```python
# ❌ 아직 구현 안 됨 → 테스트 실패
def test_user_can_login_with_valid_credentials():
    """유효한 자격 증명으로 로그인 가능."""
    auth = AuthService()

    result = auth.login("user@example.com", "password123")

    assert result.success is True
    assert result.user.email == "user@example.com"
```

### 2. GREEN: 테스트 통과하는 최소 구현

```python
# ✅ 테스트 통과하는 최소한의 코드
class AuthService:
    def login(self, email: str, password: str) -> LoginResult:
        # 최소 구현 - 하드코딩도 OK
        return LoginResult(
            success=True,
            user=User(email=email),
        )
```

### 3. REFACTOR: 코드 개선 (테스트 유지)

```python
# ✅ 리팩토링 - 실제 로직 구현
class AuthService:
    def __init__(self, user_repo: UserRepository) -> None:
        self._user_repo = user_repo

    def login(self, email: str, password: str) -> LoginResult:
        user = self._user_repo.find_by_email(email)
        if user is None:
            return LoginResult(success=False, error="User not found")

        if not user.verify_password(password):
            return LoginResult(success=False, error="Invalid password")

        return LoginResult(success=True, user=user)
```

### TDD 체크리스트

```
[ ] RED: 실패하는 테스트 작성
[ ] GREEN: 테스트 통과 (최소 구현)
[ ] REFACTOR: 코드 정리 (테스트 계속 통과)
[ ] 반복
```

---

## 테스트 유형별 가이드

### Unit Test (단위 테스트)

**범위**: 단일 함수/메서드/클래스
**속도**: 매우 빠름 (밀리초)
**의존성**: 모두 Mock

```python
# tests/unit/test_calculator.py
import pytest
from myapp.calculator import Calculator


class TestCalculator:
    """Calculator 클래스 단위 테스트."""

    def test_add_returns_sum_of_two_numbers(self):
        """두 숫자의 합을 반환한다."""
        calc = Calculator()

        result = calc.add(2, 3)

        assert result == 5

    def test_divide_raises_error_on_zero_divisor(self):
        """0으로 나누면 ZeroDivisionError 발생."""
        calc = Calculator()

        with pytest.raises(ZeroDivisionError):
            calc.divide(10, 0)
```

### Integration Test (통합 테스트)

**범위**: 여러 컴포넌트 상호작용
**속도**: 중간 (초)
**의존성**: 일부 실제 (DB, 캐시 등)

```python
# tests/integration/test_user_service.py
import pytest
from myapp.services import UserService
from myapp.repositories import UserRepository


class TestUserServiceIntegration:
    """UserService 통합 테스트."""

    @pytest.fixture
    def user_service(self, test_db):
        """테스트용 UserService 생성."""
        repo = UserRepository(test_db)
        return UserService(repo)

    def test_create_user_persists_to_database(self, user_service, test_db):
        """사용자 생성 시 DB에 저장된다."""
        user_service.create_user("test@example.com", "Test User")

        # 실제 DB에서 조회하여 확인
        saved_user = test_db.query("SELECT * FROM users WHERE email = ?",
                                    ("test@example.com",))

        assert saved_user is not None
        assert saved_user.name == "Test User"
```

### E2E Test (End-to-End 테스트)

**범위**: 전체 시스템 플로우
**속도**: 느림 (수십 초)
**의존성**: 모두 실제

```python
# tests/e2e/test_user_flow.py
import pytest
from playwright.sync_api import Page


class TestUserFlow:
    """사용자 플로우 E2E 테스트."""

    def test_user_can_register_and_login(self, page: Page):
        """회원가입 후 로그인 가능."""
        # 회원가입
        page.goto("/register")
        page.fill("#email", "new@example.com")
        page.fill("#password", "SecurePass123!")
        page.click("button[type=submit]")

        # 로그인
        page.goto("/login")
        page.fill("#email", "new@example.com")
        page.fill("#password", "SecurePass123!")
        page.click("button[type=submit]")

        # 대시보드 확인
        assert page.url.endswith("/dashboard")
        assert page.locator("h1").text_content() == "Welcome!"
```

---

## 테스트 네이밍 컨벤션

### Python (pytest)

```python
# 파일명: test_{module}.py
# tests/unit/test_user_service.py

# 클래스명: Test{ClassName}
class TestUserService:

    # 메서드명: test_{action}_{expected_result}
    def test_get_user_returns_user_when_exists(self):
        pass

    def test_get_user_returns_none_when_not_found(self):
        pass

    def test_create_user_raises_error_on_duplicate_email(self):
        pass
```

**패턴**: `test_{동작}_{예상_결과}` 또는 `test_{동작}_{조건}_{예상_결과}`

```python
# ✅ GOOD
def test_login_succeeds_with_valid_credentials(self):
def test_login_fails_with_invalid_password(self):
def test_delete_user_removes_all_related_data(self):

# ❌ BAD
def test_login(self):           # 무엇을 테스트하는지 불명확
def test_1(self):               # 의미 없는 이름
def login_test(self):           # test_로 시작 안 함
```

### TypeScript (Jest/Vitest)

```typescript
// 파일명: {module}.test.ts 또는 {module}.spec.ts
// src/services/user.service.test.ts

describe("UserService", () => {
  describe("getUser", () => {
    it("should return user when exists", () => {
      // ...
    });

    it("should return null when not found", () => {
      // ...
    });
  });

  describe("createUser", () => {
    it("should throw error on duplicate email", () => {
      // ...
    });
  });
});
```

**패턴**: `describe("{클래스/함수}")` + `it("should {예상_동작}")`

```typescript
// ✅ GOOD
it("should return 200 OK for valid request")
it("should throw ValidationError for invalid input")
it("should call repository.save exactly once")

// ❌ BAD
it("works")                    // 무엇이 작동하는지 불명확
it("test create user")         // "should"로 시작 안 함
```

---

## 테스트 구조 (AAA 패턴)

```python
def test_calculate_discount_applies_percentage(self):
    """할인율을 적용한 가격을 반환한다."""
    # Arrange (준비)
    calculator = PriceCalculator()
    original_price = 100.0
    discount_percent = 20

    # Act (실행)
    discounted_price = calculator.apply_discount(
        original_price,
        discount_percent,
    )

    # Assert (검증)
    assert discounted_price == 80.0
```

---

## 테스트 픽스처

### Python

```python
# conftest.py
import pytest
from myapp.database import Database


@pytest.fixture
def test_db():
    """테스트용 인메모리 DB."""
    db = Database(":memory:")
    db.create_tables()
    yield db
    db.close()


@pytest.fixture
def sample_user(test_db):
    """테스트용 샘플 사용자."""
    return test_db.create_user(
        email="test@example.com",
        name="Test User",
    )
```

### TypeScript

```typescript
// test/fixtures/user.fixture.ts
export const sampleUser = {
  id: 1,
  email: "test@example.com",
  name: "Test User",
};

// test/setup.ts
beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});
```

---

## 실행 명령어

```bash
# Python
uv run pytest                          # 전체 실행
uv run pytest tests/unit/              # 단위 테스트만
uv run pytest -v                       # 상세 출력
uv run pytest --cov=src --cov-report=html  # 커버리지

# TypeScript
npm test                               # 전체 실행
npm test -- --watch                    # 감시 모드
npm test -- --coverage                 # 커버리지
npm run test:e2e                       # E2E 테스트
```

---

## 참고 자료

- [pytest Documentation](https://docs.pytest.org/)
- [Jest Documentation](https://jestjs.io/)
- [TDD by Kent Beck](https://www.amazon.com/Test-Driven-Development-Kent-Beck/dp/0321146530)

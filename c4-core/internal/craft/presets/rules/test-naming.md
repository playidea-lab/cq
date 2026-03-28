# Rule: test-naming
> 테스트 함수명은 `Test_시나리오_기대결과` 패턴을 따른다.

## 규칙

- 테스트 함수명은 테스트 대상/시나리오/기대 결과를 포함
- `Test` 접두사 (Go), `test_` 접두사 (Python), `it`/`describe` (TypeScript)
- 애매한 이름 (`TestBasic`, `testCase1`) 금지

## 패턴

```
Test_<대상함수>_<시나리오>_<기대결과>
Test_<대상함수>_<시나리오>  (결과가 자명할 때)
```

## 예시

### Go
```go
// 금지
func TestCreateUser(t *testing.T) { ... }
func TestCase1(t *testing.T) { ... }
func TestBasic(t *testing.T) { ... }

// 허용
func Test_CreateUser_WithValidInput_ReturnsUser(t *testing.T) { ... }
func Test_CreateUser_WhenEmailDuplicated_ReturnsError(t *testing.T) { ... }
func Test_DeleteUser_WhenUserNotFound_Returns404(t *testing.T) { ... }
```

### Python
```python
# 금지
def test_login():
def test1():

# 허용
def test_login_with_valid_credentials_returns_token():
def test_login_with_wrong_password_raises_unauthorized():
def test_create_user_when_email_exists_raises_conflict():
```

### TypeScript (Jest)
```typescript
// 허용
describe('UserService', () => {
    describe('createUser', () => {
        it('creates user when input is valid', async () => { ... });
        it('throws ConflictError when email already exists', async () => { ... });
    });
});
```

## 이유

- 테스트 실패 시 이름만 보고 어디서 무엇이 실패했는지 알 수 있다
- 테스트 문서화 효과: 코드 동작을 명세처럼 읽을 수 있다
- 중복 테스트 방지: 이름이 명확하면 같은 케이스를 두 번 쓰지 않는다

# CUSTOMIZE: 팀 네이밍 컨벤션, 한국어/영어 혼용 정책, 허용 패턴 변형

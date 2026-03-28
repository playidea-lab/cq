# /c4-quick으로 버그 수정

CQ의 quick 워크플로우를 사용하여 버그를 진단하고 수정하는 단계별 예제입니다.

---

## 시나리오

팀에서 JWT 토큰이 만료되었을 때 인증 미들웨어가 401 대신 HTTP 500을 반환한다고 보고했습니다. 오류가 로그에 나타납니다:

```
ERROR: token parse failed: token is expired by 2h30m0s
panic: runtime error: index out of range [0] with length 0
```

해야 할 것:
1. 문제 재현
2. panic 수정 및 적절한 401 반환
3. 유효성 검사를 통과하여 제출

---

## 1단계: 빠른 태스크 시작

```
/c4-quick "fix: 만료된 JWT에서 auth 미들웨어 panic — 401을 반환해야 함"
```

출력:

```
Task created: T-042
Title: fix: 만료된 JWT에서 auth 미들웨어 panic — 401을 반환해야 함
Scope: auto-detected (go)
Status: in_progress
```

태스크가 즉시 클레임됩니다. `/c4-claim`을 별도로 실행할 필요가 없습니다.

---

## 2단계: 버그 재현

프로덕션 코드를 건드리기 전에 panic을 유발하는 테스트를 작성합니다:

```go
// internal/middleware/auth_test.go

func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
    // 이미 만료된 토큰 생성
    token := generateTestToken(t, time.Now().Add(-3*time.Hour))

    req := httptest.NewRequest("GET", "/api/data", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()

    handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rec.Code)
    }
}
```

실행:

```bash
cd c4-core && go test ./internal/middleware/... -run TestAuthMiddleware_ExpiredToken
```

```
--- FAIL: TestAuthMiddleware_ExpiredToken_Returns401 (0.00s)
panic: runtime error: index out of range [0] with length 0 [recovered]
        goroutine 7 [running]:
        ...
FAIL    github.com/pilab/cq/internal/middleware [build failed]
```

버그 확인.

---

## 3단계: 근본 원인 찾기

`internal/middleware/auth.go`를 열어 토큰이 파싱되는 부분을 확인합니다:

```go
// 수정 전 — 버그 있는 코드
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.Header.Get("Authorization"), " ")
        tokenStr := parts[1]  // 헤더가 없거나 잘못된 형식이면 panic

        claims, err := parseJWT(tokenStr)
        if err != nil {
            // 버그: err가 무시되고 실행이 계속됨
        }

        userID := claims.UserID  // claims가 nil이면 panic
        ctx := context.WithValue(r.Context(), userIDKey, userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

두 가지 버그:
1. `parts`의 범위 검사 없음 — `Authorization` 헤더가 없거나 공백이 없으면 panic
2. `err != nil` 블록에서 return하지 않음 — nil 포인터의 `claims.UserID`로 실행 계속

---

## 4단계: 코드 수정

```go
// 수정 후 — 고친 코드
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
        if len(parts) != 2 || parts[0] != "Bearer" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        claims, err := parseJWT(parts[1])
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

주요 변경사항:
- `SplitN(..., 2)`: 분할을 제한하고 공백이 포함된 토큰도 온전히 유지
- 각 오류 경로 후 명시적 `return`
- nil 포인터 역참조 없음

---

## 5단계: 테스트 실행

```bash
cd c4-core && go test ./internal/middleware/... -run TestAuthMiddleware_ExpiredToken -v
```

```
=== RUN   TestAuthMiddleware_ExpiredToken_Returns401
--- PASS: TestAuthMiddleware_ExpiredToken_Returns401 (0.00s)
PASS
ok      github.com/pilab/cq/internal/middleware  0.012s
```

헤더 누락 케이스에 대한 테스트도 추가합니다:

```go
func TestAuthMiddleware_MissingHeader_Returns401(t *testing.T) {
    req := httptest.NewRequest("GET", "/api/data", nil)
    // Authorization 헤더 없음
    rec := httptest.NewRecorder()

    AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })).ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Errorf("expected 401, got %d", rec.Code)
    }
}
```

```bash
go test ./internal/middleware/... -v
# 모든 테스트 통과
```

---

## 6단계: 유효성 검사

```
/c4-validate
```

```
Running validations...
  go-build:  PASS
  go-vet:    PASS
  go-test:   PASS  (42 tests, 0.8s)

All validations passed.
```

---

## 7단계: 제출

```
/c4-submit
```

```
Submitting T-042...
  Commit: f3a9b21 "fix(auth): return 401 on expired/missing JWT, prevent panic"
  Validation: all passed
  Status: done

Task T-042 completed.
```

---

## 8단계: 확인

```
/c4-status
```

```
Done:
  T-042  fix: 만료된 JWT에서 auth 미들웨어 panic   f3a9b21
```

---

## 핵심 패턴

**수정 전에 항상 재현.** 실패하는 테스트를 먼저 작성하면 버그가 존재함을 증명하고 나중의 회귀를 방지합니다.

**한 번에 하나씩 버그 수정.** 두 가지 수정(범위 검사 + 오류 return)이 같은 함수를 수정하고 같은 근본 원인을 해결하는 경우 하나의 커밋에 담아도 됩니다.

**모든 테스트 경로 유효성 검사.** CQ의 `/c4-validate`가 전체 테스트 스위트를 실행합니다. 수정된 케이스의 테스트 통과만으로는 충분하지 않습니다 — 전체 스위트가 초록색을 유지해야 합니다.

---

## 다음 단계

- **여러 파일, 명확한 요구사항**: [기능 계획](feature-planning.md)
- **복잡한 조사**: 근본 원인을 모를 때 `/c4-swarm --investigate` 실행
- **사용 결정 트리**: [사용 가이드 §1](../usage-guide.md)

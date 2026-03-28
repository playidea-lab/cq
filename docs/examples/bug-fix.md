# Bug Fix with /c4-quick

A step-by-step example of diagnosing and fixing a bug using CQ's quick workflow.

---

## Scenario

Your team reports that the authentication middleware returns HTTP 500 instead of 401 when a JWT token is expired. The error appears in logs as:

```
ERROR: token parse failed: token is expired by 2h30m0s
panic: runtime error: index out of range [0] with length 0
```

You need to:
1. Reproduce the issue
2. Fix the panic and return a proper 401
3. Submit with validation passing

---

## Step 1: Start a Quick Task

```
/c4-quick "fix: auth middleware panics on expired JWT — should return 401"
```

Output:

```
Task created: T-042
Title: fix: auth middleware panics on expired JWT — should return 401
Scope: auto-detected (go)
Status: in_progress
```

The task is claimed immediately. You don't need `/c4-claim` separately.

---

## Step 2: Reproduce the Bug

Write a test that triggers the panic before touching any production code:

```go
// internal/middleware/auth_test.go

func TestAuthMiddleware_ExpiredToken_Returns401(t *testing.T) {
    // Generate an already-expired token
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

Run it:

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

Bug confirmed.

---

## Step 3: Find the Root Cause

Open `internal/middleware/auth.go`. Look for where the token is parsed:

```go
// BEFORE — buggy code
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        parts := strings.Split(r.Header.Get("Authorization"), " ")
        tokenStr := parts[1]  // panics if header is missing or malformed

        claims, err := parseJWT(tokenStr)
        if err != nil {
            // BUG: err is swallowed, execution continues with nil claims
        }

        userID := claims.UserID  // panics when claims is nil
        ctx := context.WithValue(r.Context(), userIDKey, userID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Two bugs:
1. No bounds check on `parts` — panics if the `Authorization` header is missing or has no space
2. `err != nil` block doesn't return — execution falls through to `claims.UserID` on nil pointer

---

## Step 4: Fix the Code

```go
// AFTER — fixed
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

Key changes:
- `SplitN(..., 2)` limits splits and keeps the token intact if it contains spaces
- Explicit `return` after each error path
- No more nil pointer dereference

---

## Step 5: Run the Test

```bash
cd c4-core && go test ./internal/middleware/... -run TestAuthMiddleware_ExpiredToken -v
```

```
=== RUN   TestAuthMiddleware_ExpiredToken_Returns401
--- PASS: TestAuthMiddleware_ExpiredToken_Returns401 (0.00s)
PASS
ok      github.com/pilab/cq/internal/middleware  0.012s
```

Also add a test for the missing-header case:

```go
func TestAuthMiddleware_MissingHeader_Returns401(t *testing.T) {
    req := httptest.NewRequest("GET", "/api/data", nil)
    // No Authorization header
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
# All tests pass
```

---

## Step 6: Validate

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

## Step 7: Submit

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

## Step 8: Confirm

```
/c4-status
```

```
Done:
  T-042  fix: auth middleware panics on expired JWT   f3a9b21
```

---

## Key Patterns

**Always reproduce before fixing.** Writing a failing test first proves the bug exists and prevents future regressions.

**Fix one bug at a time.** Two separate fixes (bounds check + error return) are fine in one commit when they affect the same function and address the same root cause.

**Validate all test paths.** CQ's `/c4-validate` runs the full test suite. A passing test for the fixed case isn't enough — the entire suite must stay green.

---

## Next Steps

- **Multiple files, clearer requirements**: [Feature Planning](feature-planning.md)
- **Complex investigation**: Run `/c4-swarm --investigate` when the root cause is unknown
- **Usage decision tree**: [Usage Guide §1](../usage-guide.md)

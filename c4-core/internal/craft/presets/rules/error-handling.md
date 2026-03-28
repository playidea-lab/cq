# Rule: error-handling
> 에러 무시 금지. 모든 에러는 처리하거나 상위로 전파한다.

## 규칙

- 빈 catch/except 블록 금지
- Go: `_` 에러 할당 금지 (단, 명시적으로 무시해도 되는 경우 주석 필수)
- Python: `except: pass` 또는 `except Exception: pass` 금지
- TypeScript: `.catch(() => {})` 빈 핸들러 금지

## 금지 패턴

### Go
```go
// 금지
result, _ := doSomething()

// 금지
if err != nil {
    // 아무것도 안 함
}
```

### Python
```python
# 금지
try:
    risky_operation()
except:
    pass

# 금지
try:
    risky_operation()
except Exception:
    pass
```

### TypeScript
```typescript
// 금지
await doSomething().catch(() => {});

// 금지
try {
    await risky();
} catch (e) {
    // TODO: handle this
}
```

## 허용 패턴

### Go
```go
// 허용 — 에러 처리
result, err := doSomething()
if err != nil {
    return fmt.Errorf("doSomething failed: %w", err)
}

// 허용 — 의도적 무시 (주석 필수)
_ = conn.Close() // best-effort cleanup, ignore error
```

### Python
```python
# 허용
try:
    result = risky_operation()
except ValueError as e:
    logger.error("invalid value: %s", e)
    raise
```

### TypeScript
```typescript
// 허용
try {
    await risky();
} catch (error) {
    logger.error('operation failed', { error });
    throw error;
}
```

## 탐지 방법

```bash
grep -rn "except:\s*$\|except Exception:\s*pass" --include="*.py" .
grep -rn ",\s*_\s*:=\|if err != nil {$" --include="*.go" .
```

# CUSTOMIZE: 허용하는 무시 패턴 (cleanup 코드 등), 로깅 라이브러리 지정

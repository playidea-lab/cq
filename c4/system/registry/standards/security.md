# Security Rules

> 모든 코드 변경 시 자동 적용되는 보안 체크리스트입니다.

## 심각도 체계

| 심각도 | 처리 | 설명 |
|--------|------|------|
| **CRITICAL** | 커밋 차단 | 즉시 수정 필수, 병합 불가 |
| **HIGH** | 검토 필요 | PR 리뷰에서 반드시 확인 |
| **MEDIUM** | 권장 | 개선 권장, 병합 가능 |

---

## CRITICAL (커밋 차단)

### 1. 하드코딩된 시크릿 금지

**절대 금지 항목:**
- API 키, 토큰, 비밀번호
- 데이터베이스 연결 문자열
- 암호화 키, 개인키

```python
# ❌ BAD
API_KEY = "sk-1234567890abcdef"
password = "admin123"
DB_URL = "postgresql://user:pass@host/db"

# ✅ GOOD
API_KEY = os.environ.get("API_KEY")
password = get_secret("db_password")
DB_URL = settings.DATABASE_URL
```

**검사 명령:**
```bash
grep -rn --include="*.py" --include="*.ts" --include="*.js" \
  -E "(password|api_key|secret|token)\s*=\s*[\"'][^\"']+[\"']" . \
  | grep -v -E "(test_|_test\.|\.env\.example|mock)"
```

### 2. SQL 인젝션 방지

**금지 패턴:**
- 문자열 포맷팅으로 쿼리 생성
- 사용자 입력 직접 삽입

```python
# ❌ BAD - SQL Injection 취약
query = f"SELECT * FROM users WHERE id = {user_id}"
cursor.execute(f"DELETE FROM items WHERE name = '{name}'")

# ✅ GOOD - 파라미터화 쿼리
query = "SELECT * FROM users WHERE id = ?"
cursor.execute(query, (user_id,))

# ✅ GOOD - ORM 사용
user = session.query(User).filter(User.id == user_id).first()
```

**검사 명령:**
```bash
grep -rn --include="*.py" \
  -E "execute\s*\(\s*f[\"']|execute\s*\([\"'].*%s" .
```

### 3. 명령어 인젝션 방지

**금지 패턴:**
- 사용자 입력을 쉘 명령에 직접 사용
- `shell=True`와 함께 사용자 입력 사용

```python
# ❌ BAD - Command Injection 취약
os.system(f"rm -rf {user_input}")
subprocess.run(f"echo {message}", shell=True)

# ✅ GOOD - 입력 검증 + 리스트 형태
subprocess.run(["echo", sanitized_message], shell=False)

# ✅ GOOD - 화이트리스트 검증
if filename in ALLOWED_FILES:
    subprocess.run(["cat", filename])
```

**검사 명령:**
```bash
grep -rn --include="*.py" \
  -E "os\.system\s*\(.*\{|subprocess.*shell\s*=\s*True.*\{" .
```

---

## HIGH (검토 필요)

### 4. XSS (Cross-Site Scripting) 방지

**필수 조치:**
- 모든 사용자 입력 이스케이프
- CSP (Content Security Policy) 헤더 설정
- innerHTML 대신 textContent 사용

```javascript
// ❌ BAD - XSS 취약
element.innerHTML = userInput;
document.write(userMessage);

// ✅ GOOD - 이스케이프 처리
element.textContent = userInput;
element.innerHTML = DOMPurify.sanitize(userInput);
```

**검사 명령:**
```bash
grep -rn --include="*.js" --include="*.ts" --include="*.tsx" \
  -E "innerHTML\s*=|document\.write\(" .
```

### 5. CSRF (Cross-Site Request Forgery) 방지

**필수 조치:**
- 상태 변경 요청에 CSRF 토큰 포함
- SameSite 쿠키 속성 설정
- Referer/Origin 헤더 검증

```python
# ✅ GOOD - CSRF 토큰 검증
@app.post("/api/transfer")
async def transfer(request: Request, csrf_token: str = Form(...)):
    if not verify_csrf_token(csrf_token, request.session):
        raise HTTPException(status_code=403, detail="Invalid CSRF token")
    # 처리 로직
```

### 6. 인증/인가 검증

**필수 조치:**
- 모든 보호 엔드포인트에 인증 미들웨어
- 리소스 접근 시 권한 확인
- 최소 권한 원칙 적용

```python
# ✅ GOOD - 인증 + 인가 검증
@app.get("/api/users/{user_id}")
async def get_user(
    user_id: int,
    current_user: User = Depends(get_current_user)  # 인증
):
    if current_user.id != user_id and not current_user.is_admin:  # 인가
        raise HTTPException(status_code=403, detail="Not authorized")
    return await get_user_by_id(user_id)
```

---

## MEDIUM (권장)

### 7. 에러 메시지 정보 노출 금지

**권장 조치:**
- 프로덕션에서 상세 에러 숨김
- 스택 트레이스 로깅만 (응답에 포함 X)
- 사용자에게는 일반적인 메시지만 표시

```python
# ❌ BAD - 내부 정보 노출
except Exception as e:
    return {"error": str(e), "stack": traceback.format_exc()}

# ✅ GOOD - 일반 메시지 + 로깅
except Exception as e:
    logger.error(f"Error: {e}", exc_info=True)
    return {"error": "An error occurred. Please try again."}
```

### 8. 민감 데이터 로깅 금지

**권장 조치:**
- 비밀번호, 토큰 로깅 금지
- 개인정보 마스킹
- 로그 레벨 적절히 설정

```python
# ❌ BAD - 민감 정보 로깅
logger.info(f"User login: {email}, password: {password}")

# ✅ GOOD - 마스킹 처리
logger.info(f"User login: {email}, password: ***")
```

---

## 위반 시 처리 플로우

```
코드 변경
    ↓
CRITICAL 검사 ──→ 발견 시 → ❌ 커밋 차단, 즉시 수정
    ↓
HIGH 검사 ──────→ 발견 시 → ⚠️ PR 리뷰에서 필수 확인
    ↓
MEDIUM 검사 ───→ 발견 시 → 💡 개선 권장
    ↓
✅ 통과
```

---

## 자동 검사 스크립트

```bash
#!/bin/bash
# security-check.sh

echo "🔒 Security Check Starting..."

CRITICAL_FOUND=0
HIGH_FOUND=0

# CRITICAL: 하드코딩된 시크릿
if grep -rn --include="*.py" --include="*.ts" --include="*.js" \
  -E "(password|api_key|secret|token)\s*=\s*[\"'][^\"']+[\"']" . \
  | grep -v -E "(test_|_test\.|\.env\.example|mock)" | head -5; then
    echo "❌ CRITICAL: Hardcoded secrets found!"
    CRITICAL_FOUND=1
fi

# CRITICAL: SQL 인젝션 패턴
if grep -rn --include="*.py" \
  -E "execute\s*\(\s*f[\"']" . | head -5; then
    echo "❌ CRITICAL: SQL injection pattern found!"
    CRITICAL_FOUND=1
fi

# HIGH: XSS 패턴
if grep -rn --include="*.js" --include="*.ts" --include="*.tsx" \
  -E "innerHTML\s*=" . | grep -v "sanitize" | head -5; then
    echo "⚠️ HIGH: Potential XSS found!"
    HIGH_FOUND=1
fi

if [ $CRITICAL_FOUND -eq 1 ]; then
    echo "❌ CRITICAL issues found - commit blocked"
    exit 1
elif [ $HIGH_FOUND -eq 1 ]; then
    echo "⚠️ HIGH issues found - review required"
    exit 0
else
    echo "✅ Security check passed"
    exit 0
fi
```

---

## 참고 자료

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- [CWE Top 25](https://cwe.mitre.org/top25/)

# C4 Run Validations

프로젝트 검증(lint, tests, security 등)을 실행합니다.

## Arguments

```
/c4-validate [validation-names...]
```

- 이름 없이 실행: 모든 필수 검증 실행
- 특정 검증 지정: `lint`, `unit`, `security` 등

---

## 심각도 체계 (Severity Levels)

검증 결과는 3단계 심각도로 분류됩니다:

| 심각도 | 처리 | 설명 | 예시 |
|--------|------|------|------|
| **CRITICAL** | ❌ 커밋 차단 | 즉시 수정 필수, 병합 불가 | 하드코딩 시크릿, SQL 인젝션 |
| **HIGH** | ⚠️ 검토 필요 | PR 리뷰에서 반드시 확인 | XSS, 미사용 import, 타입 에러 |
| **MEDIUM** | 💡 권장 | 개선 권장, 병합 가능 | 코드 스타일, 문서 누락 |

### 심각도별 처리 규칙

```
CRITICAL 발견 → 즉시 중단, 수정 필수
HIGH 발견     → 경고 표시, 리뷰어에게 알림
MEDIUM 발견   → 정보 표시, 자동 진행 가능
```

---

## 검증 유형

### 기본 검증

| 검증 | 명령어 | 심각도 | 설명 |
|------|--------|--------|------|
| **lint** | `ruff check` / `eslint` | HIGH | 코드 스타일, 구문 오류 |
| **unit** | `pytest` / `jest` | HIGH | 단위 테스트 |
| **type** | `mypy` / `tsc --noEmit` | HIGH | 타입 검사 |

### 보안 검증

| 검증 | 명령어 | 심각도 | 설명 |
|------|--------|--------|------|
| **security-secrets** | grep 패턴 | CRITICAL | 하드코딩된 시크릿 탐지 |
| **security-sql** | grep 패턴 | CRITICAL | SQL 인젝션 패턴 탐지 |
| **security-cmd** | grep 패턴 | CRITICAL | 명령어 인젝션 패턴 탐지 |
| **security-xss** | grep 패턴 | HIGH | XSS 취약점 패턴 탐지 |
| **security-deps** | `pip-audit` / `npm audit` | HIGH | 의존성 취약점 |

### 확장 검증 (선택)

| 검증 | 명령어 | 심각도 | 설명 |
|------|--------|--------|------|
| **integration** | `pytest tests/integration/` | HIGH | 통합 테스트 |
| **e2e** | `playwright test` | MEDIUM | E2E 테스트 |
| **coverage** | `pytest --cov` | MEDIUM | 커버리지 체크 |

---

## 검증 실패 시 처리 플로우

```
/c4-validate 실행
    ↓
검증 실행 (lint, unit, security...)
    ↓
결과 수집
    ↓
┌─────────────────────────────────────────────────┐
│ CRITICAL 발견?                                   │
│   YES → ❌ 즉시 중단, 에러 출력, 수정 필요         │
│   NO  → 계속                                     │
└─────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────┐
│ HIGH 발견?                                       │
│   YES → ⚠️ 경고 표시, 수정 권고                   │
│   NO  → 계속                                     │
└─────────────────────────────────────────────────┘
    ↓
┌─────────────────────────────────────────────────┐
│ MEDIUM 발견?                                     │
│   YES → 💡 정보 표시                             │
│   NO  → 계속                                     │
└─────────────────────────────────────────────────┘
    ↓
✅ 검증 완료 (요약 출력)
```

### Worker Loop에서의 처리

```python
# Worker가 검증 실행
validation = mcp__c4__c4_run_validation(["lint", "unit", "security"])

if validation.has_critical:
    # CRITICAL: 즉시 수정 시도
    fix_critical_issues(validation.critical_errors)
    # 재검증
    validation = mcp__c4__c4_run_validation(["lint", "unit", "security"])

if validation.has_high:
    # HIGH: 수정 시도, 실패 시 경고와 함께 진행
    try:
        fix_high_issues(validation.high_errors)
    except:
        log_warning("HIGH issues not fixed, will be reviewed")

if not validation.success:
    # 여전히 실패 → 재시도 또는 BLOCKED
    retry_count += 1
    if retry_count >= MAX_RETRIES:
        mark_blocked(task)
```

---

## 보안 검증 상세

### security-secrets (CRITICAL)

하드코딩된 시크릿을 탐지합니다.

**탐지 패턴:**
```bash
grep -rn --include="*.py" --include="*.ts" --include="*.js" \
  -E "(password|api_key|secret|token)\s*=\s*[\"'][^\"']+[\"']" . \
  | grep -v -E "(test_|_test\\.|\\.env\\.example|mock)"
```

**예시 (차단됨):**
```python
# ❌ CRITICAL
API_KEY = "sk-1234567890abcdef"
DB_PASSWORD = "admin123"
```

### security-sql (CRITICAL)

SQL 인젝션 패턴을 탐지합니다.

**탐지 패턴:**
```bash
grep -rn --include="*.py" \
  -E "execute\s*\(\s*f[\"']|execute\s*\([\"'].*%s" .
```

**예시 (차단됨):**
```python
# ❌ CRITICAL
cursor.execute(f"SELECT * FROM users WHERE id = {user_id}")
```

### security-cmd (CRITICAL)

명령어 인젝션 패턴을 탐지합니다.

**탐지 패턴:**
```bash
grep -rn --include="*.py" \
  -E "os\.system\s*\(.*\{|subprocess.*shell\s*=\s*True.*\{" .
```

**예시 (차단됨):**
```python
# ❌ CRITICAL
os.system(f"rm -rf {user_input}")
```

### security-xss (HIGH)

XSS 취약점 패턴을 탐지합니다.

**탐지 패턴:**
```bash
grep -rn --include="*.js" --include="*.ts" --include="*.tsx" \
  -E "innerHTML\s*=|document\.write\(" .
```

**예시 (검토 필요):**
```javascript
// ⚠️ HIGH
element.innerHTML = userInput;
```

---

## Instructions

1. `$ARGUMENTS`에서 검증 이름 파싱
2. 이름 없으면 config의 required 검증 사용
3. `mcp__c4__c4_run_validation(names)` 호출
4. 결과 표시:
   - 각 검증 이름, 상태 (pass/fail), 심각도
   - 실행 시간
   - 실패 시 에러 출력
5. 요약: X/Y 검증 통과, CRITICAL/HIGH/MEDIUM 개수

---

## Usage

```bash
/c4-validate              # 모든 필수 검증
/c4-validate lint         # lint만
/c4-validate lint unit    # lint와 unit
/c4-validate security     # 보안 검증만 (secrets, sql, cmd, xss)
```

---

## Configuration

`.c4/config.yaml`에서 검증 설정:

```yaml
validation:
  commands:
    # 기본 검증
    lint: uv run ruff check src/ tests/
    unit: uv run pytest tests/ -v
    type: uv run mypy src/

    # 보안 검증
    security-secrets: |
      ! grep -rn --include="*.py" -E "(password|api_key|secret)\s*=\s*[\"'][^\"']+[\"']" . | grep -v test
    security-sql: |
      ! grep -rn --include="*.py" -E "execute\s*\(\s*f[\"']" .

  # 필수 검증 (항상 실행)
  required:
    - lint
    - unit
    - security-secrets

  # 심각도 매핑
  severity:
    security-secrets: critical
    security-sql: critical
    security-cmd: critical
    lint: high
    unit: high
    type: high
    security-xss: high
    coverage: medium
```

---

## 출력 예시

```
🔍 Running validations: lint, unit, security-secrets

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ lint                    PASS    (2.3s)
✅ unit                    PASS    (15.7s)
❌ security-secrets        FAIL    (0.5s)  [CRITICAL]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

❌ CRITICAL: security-secrets failed
   src/config.py:15: API_KEY = "sk-1234567890"
   → 하드코딩된 시크릿 발견. 환경변수 사용 필요.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 Summary: 2/3 passed
   CRITICAL: 1  HIGH: 0  MEDIUM: 0
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

⛔ Commit blocked due to CRITICAL issues.
```

---

## 참고

- [OWASP Top 10](https://owasp.org/www-project-top-ten/)
- `.claude/rules/security.md` - 보안 체크리스트 상세

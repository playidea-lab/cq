---
name: security-audit
description: |
  보안 점검 워크플로우. 인증/인가/입력검증/시크릿/의존성 취약점을 순서대로 검토.
  트리거: "보안 점검", "security audit", "보안 리뷰", "취약점 확인"
allowed-tools: Read, Glob, Grep, Bash
---
# Security Audit

코드베이스의 보안 취약점을 체계적으로 점검합니다.

## 실행 순서

### Step 1: 시크릿 하드코딩 탐지

```bash
# API 키, 토큰, 비밀번호 패턴
grep -rE "(api_key|apikey|secret|password|token|pwd)\s*=\s*['\"][^'\"]{8,}" \
  --include="*.go" --include="*.py" --include="*.ts" .

# AWS 키 패턴
grep -rE "AKIA[0-9A-Z]{16}" .

# .env 파일이 git에 포함됐는지 확인
git ls-files | grep -E "\.env$|credentials|secret"
```

- [ ] 하드코딩된 시크릿 없음
- [ ] .gitignore에 .env, *.key, *.pem 포함
- [ ] 환경변수로 시크릿 주입

### Step 2: 인증/인가 확인

코드에서 인증 처리 부분 탐색:

```bash
grep -rn "auth\|jwt\|token\|bearer\|middleware" --include="*.go" .
```

- [ ] 모든 비공개 엔드포인트에 인증 미들웨어 적용
- [ ] JWT 서명 검증 필수 (alg:none 허용 금지)
- [ ] 권한 체크가 데이터 레이어 이전에 수행
- [ ] 세션 만료 처리

### Step 3: 입력 검증

```bash
grep -rn "req.Body\|r.FormValue\|request.body\|req.params" \
  --include="*.go" --include="*.ts" .
```

- [ ] 사용자 입력 길이/형식 검증
- [ ] SQL 쿼리: parameterized query 사용 (문자열 연결 금지)
- [ ] HTML/JS 출력 시 이스케이프 처리 (XSS 방지)
- [ ] 파일 경로 입력 시 path traversal 방지

### Step 4: 의존성 취약점 스캔

```bash
# Go
govulncheck ./...

# Python
uv run pip-audit
# 또는
uv run safety check

# Node.js
pnpm audit

# Docker 이미지
trivy image <image-name>
```

- [ ] 알려진 CVE 없음 (또는 조치 계획 있음)
- [ ] 의존성 lock 파일 커밋됨

### Step 5: HTTPS/TLS

- [ ] 프로덕션에서 HTTP → HTTPS 리다이렉트
- [ ] HSTS 헤더 설정
- [ ] 인증서 만료일 확인

```bash
echo | openssl s_client -connect <domain>:443 2>/dev/null | openssl x509 -noout -dates
```

### Step 6: 보안 헤더 확인

```bash
curl -I https://<service-url>/ | grep -iE "x-frame|content-security|x-content-type|strict-transport"
```

- [ ] `X-Frame-Options: DENY`
- [ ] `X-Content-Type-Options: nosniff`
- [ ] `Content-Security-Policy` 설정
- [ ] `Strict-Transport-Security` 설정

### Step 7: 감사 결과 정리

```
## 보안 감사 결과

**점검 일시**: YYYY-MM-DD
**점검 범위**: <서비스/모듈>

### Critical (즉시 수정)
- ...

### High (1주 내)
- ...

### Medium (다음 스프린트)
- ...

### 통과
- ...
```

# CUSTOMIZE: 추가 점검 항목, 스캔 도구, 보안 정책 규정
# 예: OWASP ZAP 스캔 통합, SAST 도구 추가 (Semgrep, CodeQL)

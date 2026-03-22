---
name: security-audit
description: |
  보안 감사 체크리스트 및 보고서 작성 워크플로우. OWASP Top 10, 인증/권한, 입력 검증, 시크릿
  관리 등을 체계적으로 점검합니다. "보안 감사", "security audit", "취약점 점검",
  "보안 리뷰", "OWASP 체크", "보안 점검" 등의 요청에 트리거됩니다.
---
# Security Audit

보안 감사 체크리스트 + 보고서 작성 워크플로우.

## 트리거

"보안 감사", "security audit", "취약점 점검", "보안 리뷰", "OWASP 체크"

## Steps

### 1. 범위 설정

- 대상: 전체 시스템 / 특정 서비스 / 특정 기능
- 유형: 코드 리뷰 / 설정 점검 / 의존성 스캔 / 침투 테스트
- 기준: OWASP Top 10, CWE Top 25

### 2. 인증/인가 점검

- [ ] 인증 우회 가능한 엔드포인트 없는가?
- [ ] JWT 서명 검증이 모든 경로에서 동작하는가?
- [ ] `alg: none` 허용되지 않는가?
- [ ] 토큰 만료 시간이 적절한가? (access: 15분~1시간, refresh: 7~30일)
- [ ] 권한 체크가 리소스 소유자 수준까지 되는가? (IDOR 방지)
- [ ] 관리자 API가 분리되어 있는가?

### 3. 입력 검증 점검

- [ ] SQL Injection: parameterized query 사용 확인
- [ ] XSS: 출력 인코딩/이스케이프 확인
- [ ] Command Injection: 사용자 입력이 쉘 명령에 들어가지 않는가?
- [ ] Path Traversal: `../` 방지, `filepath.Clean()` 사용
- [ ] SSRF: 사용자 제공 URL로 내부 요청하지 않는가?
- [ ] 파일 업로드: 타입 검증, 크기 제한, 실행 불가 저장 경로

### 4. 데이터 보호 점검

- [ ] 전송 중 암호화 (TLS 1.2+)
- [ ] 저장 시 암호화 (PII, 비밀번호 — bcrypt/argon2)
- [ ] 로그에 시크릿/PII 노출 없는가?
- [ ] 에러 메시지에 내부 구현 상세 노출 없는가?
- [ ] 개인정보 최소 수집 원칙 준수

### 5. 의존성 점검

```bash
# Go
govulncheck ./...

# Python
uv run pip-audit

# Node
pnpm audit

# Container
trivy image <image>
```

- [ ] Critical/High CVE 없는가?
- [ ] 사용하지 않는 의존성 제거
- [ ] lock 파일(go.sum, uv.lock, pnpm-lock.yaml) 커밋 확인

### 6. 설정 점검

- [ ] CORS: 와일드카드(`*`) 아닌 명시적 도메인
- [ ] Rate limiting 설정
- [ ] 보안 헤더: CSP, X-Frame-Options, X-Content-Type-Options
- [ ] 쿠키: HttpOnly, Secure, SameSite
- [ ] 디버그 모드 프로덕션 비활성화

### 7. 시크릿 스캐닝

```bash
# gitleaks — git 히스토리 전체 스캔
gitleaks detect --source . --verbose

# trufflehog — 엔트로피 기반 탐지
trufflehog filesystem --directory .
```

- [ ] git 히스토리에 시크릿 노출 없는가?
- [ ] pre-commit hook에 시크릿 스캐너 설정되어 있는가?
- [ ] 과거 유출된 키가 revoke되었는가?

### 8. 보고서

```markdown
## 보안 감사 보고서
- **일시**: YYYY-MM-DD
- **범위**: [대상 서비스/기능]
- **결과 요약**: Critical N개, High N개, Medium N개, Low N개

### 발견 사항

#### [SEV] 제목
- **심각도**: Critical / High / Medium / Low
- **위치**: [파일:줄번호 또는 엔드포인트]
- **설명**: [취약점 설명]
- **재현**: [재현 방법]
- **수정**: [권장 수정 방법]
- **상태**: Open / Fixed / Accepted Risk
```

## 안티패턴

- 프로덕션 환경에서 침투 테스트 (스테이징에서 수행)
- "우리는 작아서 공격 안 받는다" 가정
- 취약점 발견 후 비공개로 방치

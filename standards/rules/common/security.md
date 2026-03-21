# Security Rules

> 모든 프로젝트에 적용되는 보안 규칙.

## 시크릿 관리

- 코드에 API 키, 비밀번호, 토큰, 인증서를 하드코딩하지 않는다.
- `.env`, `credentials.json`, `*.pem`, `*.key` 파일은 `.gitignore`에 포함.
- 시크릿은 환경변수 또는 시크릿 매니저(Vault, AWS Secrets Manager)로 관리.
- CI/CD 시크릿은 GitLab CI Variables 또는 동등한 서비스 사용.

## 입력 검증

- 사용자 입력은 항상 검증 후 사용 (화이트리스트 방식 우선).
- SQL 쿼리: parameterized query 필수. 문자열 연결 금지.
- 명령어 실행: `os.system()`, `exec()`, `eval()` 사용 시 입력 sanitize 필수.
- 파일 경로: path traversal (`../`) 방지. `filepath.Clean()` 또는 동등 함수 사용.

## 인증/권한

- 인증 로직은 자체 구현하지 않는다 (검증된 라이브러리 사용).
- JWT 토큰: 서명 검증 필수. `alg: none` 허용 금지.
- API 엔드포인트: 기본값은 인증 필요. 공개 엔드포인트는 명시적 표시.

## 의존성

- 알려진 취약점이 있는 의존성 사용 금지.
- 의존성 추가 시 라이선스 확인 (GPL 주의).
- lock 파일 (go.sum, uv.lock, pnpm-lock.yaml) 커밋 필수.

## 로깅

- 시크릿, PII(개인식별정보), 토큰을 로그에 출력하지 않는다.
- 에러 메시지에 내부 구현 상세를 노출하지 않는다.

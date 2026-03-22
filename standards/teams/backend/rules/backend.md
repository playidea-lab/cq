# Backend Team Rules

> Go(주) + Python(FastAPI) 백엔드 서비스 개발 규칙.
> 적용: `cq init --team backend`

---

## API 설계

- RESTful: 리소스 중심 URL. 동사 금지 (`/users`, not `/getUsers`).
- 응답 형식: `{ "data": ..., "error": ... }` 통일.
- 버전: URL prefix (`/api/v1/`). 하위 호환 유지.
- 페이지네이션: cursor 기반 우선. offset은 관리 API에서만.

## 에러 처리

- 내부 에러를 클라이언트에 노출하지 않는다.
- 에러 코드: 도메인별 코드 체계 사용 (예: `USER_NOT_FOUND`, `PAYMENT_FAILED`).
- 4xx는 클라이언트 잘못, 5xx는 서버 잘못. 혼용 금지.
- 5xx 발생 시 알림 필수 (로깅 + 모니터링).

## 데이터베이스

- DDL 변경은 마이그레이션 파일로 관리 (수동 ALTER 금지).
- 마이그레이션은 idempotent + rollback 가능.
- N+1 쿼리 금지 — JOIN 또는 batch fetch.
- 인덱스 변경 시 slow query 영향 분석 필수.

## 인증/인가

- JWT 검증은 미들웨어 레벨에서.
- 리소스 접근 권한: 엔드포인트마다 명시적 체크.
- 관리자 API: 별도 라우터 그룹으로 분리.

## Go 서비스

- graceful shutdown: `signal.NotifyContext` + `server.Shutdown`.
- health check: `/healthz` (liveness), `/readyz` (readiness).
- config: 환경변수 → 구조체. 시작 시 검증.

## Python(FastAPI) 서비스

- Pydantic 모델로 요청/응답 스키마 정의.
- 비동기 I/O: `httpx.AsyncClient`, `asyncpg`.
- 동기 블로킹 호출: `run_in_executor`로 감싸기.
- CORS: 허용 origin 명시 (와일드카드 금지 in prod).

## 배포

- Dockerfile: multi-stage build. 최종 이미지에 빌드 도구 포함 금지.
- 환경 구분: 환경변수로만. 코드 분기 금지.
- readiness probe 통과 전까지 트래픽 차단.

## CQ 연동 (CQ 프로젝트인 경우)

| 작업 | CQ 도구/스킬 |
|------|-------------|
| API/서비스 설계 | `/c4-plan` |
| 구현 실행 | `/c4-run` |
| 빌드·린트·테스트 검증 | `/c4-validate` |
| 구현 마무리 (빌드, 커밋) | `/c4-finish` |
| 기존 패턴/장애 이력 조회 | `c4_knowledge_search` |

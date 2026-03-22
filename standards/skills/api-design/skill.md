# API Design

REST API 설계 가이드.

## 트리거

"API 설계", "api design", "엔드포인트 설계", "REST API"

## Steps

### 1. 리소스 식별

- 도메인 모델에서 리소스(명사) 추출
- 리소스 간 관계 정의 (1:N, N:M, 포함)
- URL 경로 설계: `/{리소스}/{id}/{하위리소스}`

### 2. 엔드포인트 설계

**REST 규칙:**
- 리소스명은 복수형 (`/users`, not `/user`)
- HTTP 메서드로 동작 구분 (GET/POST/PUT/PATCH/DELETE)
- 동사 URL 금지 (`/users/123` not `/getUser?id=123`)

**버저닝:**
- URL 경로: `/v1/users` (권장)
- 헤더: `Accept: application/vnd.api.v1+json` (대안)
- 메이저 변경만 버전업. 하위 호환 변경은 같은 버전.

### 3. 요청/응답 설계

**요청:**
- Content-Type: `application/json`
- 페이지네이션: cursor 기반 (`?cursor=xxx&limit=20`), offset 금지 (대규모 데이터)
- 필터링: 쿼리 파라미터 (`?status=active&created_after=2026-01-01`)
- 정렬: `?sort=created_at&order=desc`

**응답:**
```json
{
  "data": { ... },
  "meta": { "cursor": "...", "has_more": true },
  "error": null
}
```

**에러:**
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "이메일 형식이 올바르지 않습니다",
    "details": [{"field": "email", "reason": "invalid_format"}]
  }
}
```

### 4. HTTP 상태 코드

| 코드 | 용도 |
|------|------|
| 200 | 성공 (GET, PUT, PATCH) |
| 201 | 생성 성공 (POST) |
| 204 | 삭제 성공 (DELETE) |
| 400 | 잘못된 요청 (validation) |
| 401 | 인증 필요 |
| 403 | 권한 없음 |
| 404 | 리소스 없음 |
| 409 | 충돌 (중복 생성) |
| 422 | 처리 불가 (비즈니스 로직 에러) |
| 429 | Rate limit 초과 |
| 500 | 서버 에러 (5xx는 내부 상세 노출 금지) |

### 5. 인증/인가

- 인증: Bearer token (`Authorization: Bearer <jwt>`)
- 인가: 엔드포인트별 권한 체크 (미들웨어)
- 공개 API: 명시적으로 표시. 기본값은 인증 필요.
- Rate limiting: IP + 사용자 기반, 응답 헤더에 잔여량 표시

### 6. 문서화

- OpenAPI/Swagger 스펙 필수
- 모든 엔드포인트에 요청/응답 예시
- 에러 코드 목록 + 대응 방법
- 인증 방법 가이드

## 안티패턴

- RPC 스타일 URL (`/createUser`, `/deleteUser`)
- 에러를 항상 200으로 반환
- API 키를 쿼리 파라미터로 전달
- 하위 호환 깨는 변경을 같은 버전에서

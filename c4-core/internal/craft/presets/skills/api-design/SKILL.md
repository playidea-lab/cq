---
name: api-design
description: |
  REST API 설계 체크리스트. Naming, Versioning, Error codes, Request/Response 설계 검토.
  트리거: "API 설계", "REST 설계", "api design", "엔드포인트 설계"
allowed-tools: Read, Write, Edit, Glob, Grep
---
# API Design

REST API를 일관성 있게 설계합니다.

## 실행 순서

### Step 1: 리소스 도출

API가 다루는 핵심 리소스를 명사로 정의:

```
Users, Orders, Products, Sessions, ...
```

### Step 2: URL 설계

#### Naming 규칙
- 소문자, 하이픈 사용: `/user-profiles` (not `/userProfiles`)
- 복수형 명사: `/users` (not `/user`)
- 계층 관계: `/users/{id}/orders`
- 동사 금지: `/users/create` 사용하지 말 것

#### HTTP 메서드 매핑
```
GET    /users          — 목록 조회
POST   /users          — 생성
GET    /users/{id}     — 단건 조회
PUT    /users/{id}     — 전체 수정
PATCH  /users/{id}     — 부분 수정
DELETE /users/{id}     — 삭제
```

### Step 3: 버전 관리

```
# URL 버전 (권장)
/api/v1/users
/api/v2/users

# 헤더 버전 (선택)
Accept: application/vnd.api+json;version=2
```

- v1 → v2 breaking change 시 이전 버전 최소 6개월 유지
- Deprecation 헤더 사용: `Deprecation: true`

### Step 4: 요청/응답 형식

#### 응답 구조
```json
// 성공 — 단건
{
  "data": { "id": "123", "name": "Alice" }
}

// 성공 — 목록
{
  "data": [...],
  "meta": { "total": 100, "page": 1, "limit": 20 }
}

// 에러
{
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "사용자를 찾을 수 없습니다",
    "details": []
  }
}
```

### Step 5: HTTP 상태 코드

| 코드 | 용도 |
|------|------|
| 200 | 성공 (조회, 수정) |
| 201 | 생성 성공 |
| 204 | 성공 (삭제, 응답 없음) |
| 400 | 잘못된 요청 (입력 오류) |
| 401 | 인증 필요 |
| 403 | 권한 없음 |
| 404 | 리소스 없음 |
| 409 | 충돌 (중복) |
| 422 | 유효성 검사 실패 |
| 429 | 요청 한도 초과 |
| 500 | 서버 에러 |

### Step 6: 설계 체크리스트

- [ ] URL이 명사형인가?
- [ ] HTTP 메서드가 의미에 맞는가?
- [ ] 버전이 명시됐는가?
- [ ] 에러 응답 형식이 일관적인가?
- [ ] 인증 방식이 정의됐는가? (Bearer, API Key)
- [ ] 페이지네이션 방식이 결정됐는가? (cursor/offset)
- [ ] Rate limiting이 고려됐는가?
- [ ] 필드명이 camelCase로 일관적인가?

### Step 7: 문서화

OpenAPI (Swagger) 스펙 작성:

```yaml
openapi: 3.0.0
info:
  title: <API Name>
  version: "1.0"
paths:
  /users:
    get:
      summary: List users
      ...
```

# CUSTOMIZE: API 인증 방식, 페이지네이션 전략, 에러 코드 체계 지정
# 예: JWT Bearer, API Key in header, OAuth2

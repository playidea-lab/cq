# C4 Keycloak Identity Provider

Keycloak 기반의 인증/인가 시스템으로 C4 플랫폼의 사용자 관리와 SSO를 담당합니다.

## Quick Start

```bash
# 1. 환경 변수 설정
cp .env.example .env
# .env 파일을 열어 필요한 값들을 설정

# 2. 서버 시작 (개발 모드)
docker compose up -d

# 3. 상태 확인
docker compose ps
docker compose logs -f keycloak
```

**Admin Console**: http://localhost:8080/admin
**기본 계정**: admin / admin (첫 로그인 시 변경 필요)

## 아키텍처

```
┌─────────────────────────────────────────────────────────────┐
│                        C4 Platform                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   ┌─────────┐     ┌─────────┐     ┌─────────┐             │
│   │ C4 Web  │     │ C4 API  │     │ C4 CLI  │             │
│   │  (SPA)  │     │(Backend)│     │  (CLI)  │             │
│   └────┬────┘     └────┬────┘     └────┬────┘             │
│        │               │               │                   │
│        └───────────────┼───────────────┘                   │
│                        │                                   │
│                        ▼                                   │
│   ┌─────────────────────────────────────────────┐         │
│   │              Keycloak (OIDC)                │         │
│   │  ┌─────────────────────────────────────┐   │         │
│   │  │            C4 Realm                 │   │         │
│   │  │  • c4-api (confidential client)     │   │         │
│   │  │  • c4-web (public SPA client)       │   │         │
│   │  │  • c4-cli (public CLI client)       │   │         │
│   │  └─────────────────────────────────────┘   │         │
│   │                    │                        │         │
│   │     ┌──────────────┼──────────────┐        │         │
│   │     ▼              ▼              ▼        │         │
│   │  ┌──────┐    ┌──────────┐    ┌────────┐   │         │
│   │  │GitHub│    │  Google  │    │Username│   │         │
│   │  │ IdP  │    │   IdP    │    │Password│   │         │
│   │  └──────┘    └──────────┘    └────────┘   │         │
│   └─────────────────────────────────────────────┘         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## 클라이언트 구성

### c4-api (Backend Service)
- **Type**: Confidential Client
- **Auth**: Client Secret
- **Features**: Service Account, Direct Access Grant
- **용도**: 백엔드 API 서버의 토큰 검증 및 서비스 간 통신

### c4-web (Frontend SPA)
- **Type**: Public Client
- **Auth**: PKCE (Proof Key for Code Exchange)
- **Features**: Standard Flow
- **용도**: React/Vue 등 SPA 애플리케이션의 사용자 인증

### c4-cli (CLI Tool)
- **Type**: Public Client
- **Auth**: PKCE + Device Authorization
- **Features**: Direct Access Grant
- **용도**: CLI 도구에서 사용자 인증

## 역할 (Roles)

| Role | Description | 권한 |
|------|-------------|------|
| `admin` | 플랫폼 관리자 | 전체 관리 기능 |
| `supervisor` | 슈퍼바이저 | 체크포인트 리뷰, 태스크 관리 |
| `worker` | 워커 | 할당된 태스크 실행 |
| `viewer` | 뷰어 | 읽기 전용 접근 |

## Identity Providers

### GitHub OAuth
1. [GitHub Developer Settings](https://github.com/settings/developers)에서 OAuth App 생성
2. **Authorization callback URL**: `http://localhost:8080/realms/c4/broker/github/endpoint`
3. `.env`에 Client ID/Secret 설정

### Google OAuth
1. [Google Cloud Console](https://console.cloud.google.com/apis/credentials)에서 OAuth 2.0 Client ID 생성
2. **Authorized redirect URI**: `http://localhost:8080/realms/c4/broker/google/endpoint`
3. `.env`에 Client ID/Secret 설정

## API 연동

### Python (FastAPI)
```python
from fastapi import Depends, HTTPException
from fastapi.security import OAuth2AuthorizationCodeBearer
import httpx

oauth2_scheme = OAuth2AuthorizationCodeBearer(
    authorizationUrl="http://localhost:8080/realms/c4/protocol/openid-connect/auth",
    tokenUrl="http://localhost:8080/realms/c4/protocol/openid-connect/token",
)

async def verify_token(token: str = Depends(oauth2_scheme)):
    async with httpx.AsyncClient() as client:
        response = await client.get(
            "http://localhost:8080/realms/c4/protocol/openid-connect/userinfo",
            headers={"Authorization": f"Bearer {token}"}
        )
        if response.status_code != 200:
            raise HTTPException(status_code=401, detail="Invalid token")
        return response.json()
```

### JavaScript/TypeScript (Frontend)
```typescript
import Keycloak from 'keycloak-js';

const keycloak = new Keycloak({
  url: 'http://localhost:8080',
  realm: 'c4',
  clientId: 'c4-web',
});

await keycloak.init({
  onLoad: 'check-sso',
  pkceMethod: 'S256',
});

// API 호출 시 토큰 사용
const response = await fetch('/api/data', {
  headers: {
    Authorization: `Bearer ${keycloak.token}`,
  },
});
```

## Endpoints

| Endpoint | URL |
|----------|-----|
| Admin Console | http://localhost:8080/admin |
| Account Console | http://localhost:8080/realms/c4/account |
| OpenID Configuration | http://localhost:8080/realms/c4/.well-known/openid-configuration |
| Authorization | http://localhost:8080/realms/c4/protocol/openid-connect/auth |
| Token | http://localhost:8080/realms/c4/protocol/openid-connect/token |
| UserInfo | http://localhost:8080/realms/c4/protocol/openid-connect/userinfo |
| Logout | http://localhost:8080/realms/c4/protocol/openid-connect/logout |

## Production Deployment

```bash
# 프로덕션 프로파일 사용
docker compose --profile production up -d

# TLS 인증서 준비 (certs/ 디렉토리)
mkdir -p certs
# server.crt.pem, server.key.pem 파일 배치

# 환경 변수 설정 (필수!)
export KC_ADMIN_PASSWORD=strong-password
export POSTGRES_PASSWORD=strong-password
export KC_HOSTNAME=auth.yourdomain.com
```

## Troubleshooting

### Keycloak이 시작되지 않음
```bash
# 로그 확인
docker compose logs keycloak

# DB 연결 확인
docker compose exec keycloak-db psql -U keycloak -c "SELECT 1"
```

### Realm 가져오기 실패
```bash
# Realm 설정 검증 (JSON syntax)
cat realm-config/c4-realm.json | jq .

# 수동 가져오기
docker compose exec keycloak \
  /opt/keycloak/bin/kc.sh import --file /opt/keycloak/data/import/c4-realm.json
```

### 토큰 검증 실패
- CORS 설정 확인 (Web Origins)
- Redirect URI 확인
- Client Secret 확인 (confidential client)

## 기본 테스트 계정

| Username | Password | Roles |
|----------|----------|-------|
| c4-admin | admin | admin, supervisor, worker |
| c4-worker | worker | worker |

**주의**: 첫 로그인 시 비밀번호 변경 필요 (temporary: true)

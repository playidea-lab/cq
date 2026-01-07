# C4 Cloud Documentation

C4 Cloud는 C4의 호스팅 SaaS 버전입니다.

## Documents

| Document | Description |
|----------|-------------|
| [PRD.md](./PRD.md) | Product Requirements Document |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System Architecture |

## Key Features (vs Local)

```
C4 Local (CLI)              C4 Cloud (SaaS)
─────────────────           ─────────────────
터미널 설치 필요      →      웹 브라우저만
API 키 직접 관리      →      C4가 관리
로컬 파일 결과        →      GitHub 자동 push
수동 워커 추가        →      슬라이더로 조절
무료                  →      구독 + 사용량
```

## Target Launch

- **v2.0 (MVP)**: 기본 웹 대시보드 + GitHub 연동
- **v2.5**: 멀티 워커 + 팀 기능
- **v3.0**: Enterprise (SSO, 온프레미스)

## Tech Stack (Planned)

- **Frontend**: Next.js, Vercel
- **Backend**: FastAPI, Fly.io
- **Database**: PostgreSQL (Supabase)
- **Auth**: Clerk
- **Payments**: Stripe

# C4 Cloud Documentation

C4 Cloud는 C4의 호스팅 SaaS 버전입니다.

> **현재 상태**: 계획 단계. 로컬 버전(v0.1.0) 완성 후 개발 예정.
>
> 로드맵은 [../ROADMAP.md](../ROADMAP.md) 참조.

## Documents

| Document | Description |
|----------|-------------|
| [PRD.md](./PRD.md) | Product Requirements Document |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | System Architecture |

## 로컬 vs 클라우드

| 기능 | C4 Local (v0.1) | C4 Cloud (계획) |
|------|-----------------|-----------------|
| 설치 | `uv sync` + MCP 설정 | 웹 브라우저만 |
| API 키 | 사용자 관리 | C4가 관리 |
| 결과물 | 로컬 파일 | GitHub 자동 push |
| 워커 | 같은 머신 | 슬라이더로 조절 |
| 팀 협업 | 제한적 | 완전 지원 |
| 비용 | 무료 | 구독 + 사용량 |

## 개발 단계

### Phase 1: State Store 추상화 (v0.2.0) ✅ 완료

로컬 버전에서 원격 state 지원:

```yaml
# .c4/config.yaml
store:
  backend: supabase  # sqlite (기본), local_file, supabase
  supabase_url: https://xxx.supabase.co
  supabase_key: your-anon-key
```

**또는 환경 변수:**
```bash
export C4_STORE_BACKEND=supabase
export SUPABASE_URL=https://xxx.supabase.co
export SUPABASE_KEY=your-anon-key
```

**설치:**
```bash
uv add "c4[cloud]"  # supabase 의존성 설치
```

**구현된 기능:**
- ✅ StateStore Protocol + atomic_modify
- ✅ SupabaseStateStore (PostgreSQL + Real-time)
- ✅ Store Factory (환경변수/config.yaml 지원)
- ✅ Optimistic locking (동시성 제어)

### Phase 2: Cloud MVP (v1.0.0)

- 웹 대시보드
- GitHub 연동
- Worker Pool 관리

### Phase 3: Enterprise

- SSO
- 온프레미스
- 감사 로그

## Tech Stack (Planned)

| Layer | Technology |
|-------|------------|
| Frontend | Next.js, Vercel |
| Backend | FastAPI, Fly.io |
| Database | PostgreSQL (Supabase) |
| Cache/Lock | Redis |
| Auth | Clerk |
| Payments | Stripe |

## MCP 연결 옵션

Cloud 버전에서 Claude Code 연결:

```bash
# Option A: HTTP Transport (권장)
claude mcp add --transport http c4-cloud https://api.c4.dev/mcp

# Option B: 로컬 Daemon + Cloud State
# 로컬에서 Daemon 실행, state만 cloud에 저장
```

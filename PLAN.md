# C4 Cloud 개발 계획

## 개요

C4를 다양한 사용자 그룹(개인 IDE, 개인 채팅 UI, 팀)에 맞게 확장하기 위한 개발 계획.

## 현재 상태: Phase 4 - 인증 및 보안 강화 (v0.6.6)

### 완료된 Phase

| Phase | 제목 | 상태 |
|-------|------|------|
| Phase 1 | Core Architecture | ✅ 완료 |
| Phase 2 | Multi-Worker Support | ✅ 완료 |
| Phase 3 | Git 통합 및 자동화 | ✅ 완료 |

---

## Tasks (Phase 3: Git 통합 및 자동화) ✅

### T-301: Git 필수 설치 체크 및 안내
- `install.sh`에서 Git 존재 여부 확인 및 설치 가이드 제공 완료.

### T-302: c4 init Git 자동화
- `c4 init` 시 `.git` 초기화, `.gitignore` 생성 및 초기 커밋 자동 수행 완료.

### T-303: 자동 커밋 및 태깅 시스템
- 태스크 완료 시 자동 커밋 및 체크포인트 도달 시 `c4/CP-XXX` 태그 생성 로직 구현 완료.

### T-304: 롤백 기능 (`c4 rollback`)
- 특정 체크포인트로 하드/소프트 리셋하는 명령 구현 및 안정화 완료.

---

## 현재 진행 Phase: Phase 4 - 인증 및 클라우드 동기화 🚧

### T-401: Keycloak 기반 SSO 연동
- Keycloak을 통한 사용자 인증 및 JWT 토큰 관리 시스템 구축.

### T-402: Supabase 원격 상태 동기화
- 로컬 SQLite 상태를 원격 Supabase와 양방향 동기화 (Conflict 해결 포함).

### T-403: c4 login / logout 명령
- CLI 로그인 및 세션 관리, 액세스 토큰 갱신 로직.

### T-404: 비용 및 사용량 리포팅
- LLM 토큰 소모량 및 비용 추적 대시보드 기초 작업.

---

## 향후 Phase: Phase 5 - 샌드박스 및 대시보드 (v0.7.0) 📋

### T-501: 격리된 실행 환경 (Sandbox)
- Docker/Firecracker 기반 일회성 워크스페이스 구축.

### T-502: 웹 대시보드 MVP
- 프로젝트 진행 상황 시각화 및 실시간 로그 스트리밍 (Next.js).

### T-503: 자동 알림 시스템
- 태스크 완료/체크포인트 도달 시 Slack/Discord 알림 연동.

### Phase 7: 클라우드 실행 환경 (SaaS)
- Fly.io 기반 샌드박스 워커 실행
- 사용량 기반 과금 (Stripe)

---

## References
- [docs/ROADMAP.md](docs/ROADMAP.md) - 전체 로드맵
- [GEMINI.md](GEMINI.md) - Gemini 전용 개발 가이드
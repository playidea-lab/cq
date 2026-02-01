# C4 로드맵 요약

## 핵심 가치
> "개발자 1명이 5명 팀의 생산성을 갖는다"

C4 = AI Worker Pool + State Machine + Quality Gates

## 타겟 사용자

| 세그먼트 | 핵심 Pain Point | C4 가치 |
|---------|----------------|---------|
| Solo Developer | 시간 부족, 멀티태스킹 | 24시간 일하는 팀원 |
| Startup | 리소스 제한, 속도 압박 | 개발 속도 3-5x |
| Agency | 다중 프로젝트, 품질 관리 | 표준화된 워크플로우 |

## Phase 완료 현황

### 완료됨

| Phase | 설명 | 상태 |
|-------|------|------|
| Phase 1 | GitHub App 통합 | ✅ 완료 |
| Phase 2 | 통합 프레임워크 | ✅ 완료 |
| Phase 5 | SSO/SAML 인증 | ✅ 완료 |

### 진행 중

| Phase | 설명 | API | UI | 남은 태스크 |
|-------|------|-----|-----|------------|
| Phase 3 | 팀 관리 | ✅ 100% | 🟡 | T-3004 |
| Phase 4 | 타임트래킹/감사로그 | ✅ 100% | 🟡 | T-4003 |

### 미시작

| Phase | 설명 |
|-------|------|
| Phase 6 | 추가 기능 (Billing, Analytics) |
| Phase 7 | Self-Documenting Platform |

## 다음 단계 (남은 태스크)

| Task | 설명 | 우선순위 |
|------|------|---------|
| T-3004 | Teams 관리 UI 컴포넌트 | 1 |
| T-4003 | Reports/Analytics UI | 2 |

### T-3004: Teams 관리 UI
- TeamCard 컴포넌트
- TeamList 페이지
- TeamCreate/Edit 폼
- Member 관리 UI

### T-4003: Reports/Analytics UI
- Reports 대시보드
- Activity 타임라인
- 통계 시각화

## 기술 스택

| 영역 | 기술 |
|------|------|
| Backend | Python, FastAPI, SQLAlchemy |
| Frontend | Next.js, React, TypeScript |
| Database | PostgreSQL (Supabase) |
| Auth | Supabase Auth + SSO |
| Infra | Docker, Terraform |

## 아키텍처 요약

```
┌─────────────┐     ┌─────────────┐
│   Web UI    │────▶│   API       │
│  (Next.js)  │     │  (FastAPI)  │
└─────────────┘     └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
       ┌──────────┐ ┌──────────┐ ┌──────────┐
       │ Supabase │ │ Workers  │ │  GitHub  │
       │   (DB)   │ │  (AI)    │ │   App    │
       └──────────┘ └──────────┘ └──────────┘
```

## 참조
- 전체 분석: [`docs/archive/c4-user-scenario-analysis.md`](./archive/c4-user-scenario-analysis.md)
- API 문서: [`c4/api/`](../c4/api/)
- Web UI: [`web/`](../web/)

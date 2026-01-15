# C4 Cloud 개발 계획

## 개요

C4를 다양한 사용자 그룹(개인 IDE, 개인 채팅 UI, 팀)에 맞게 확장하기 위한 개발 로드맵.

### 핵심 결정 사항

| 항목 | 결정 | 비고 |
|------|------|------|
| **인증** | Supabase Auth | Keycloak 대신 (스택 단순화) |
| **DB** | Supabase PostgreSQL | 팀 협업 시 |
| **실시간** | Supabase Realtime | 상태/이벤트 동기화 |
| **Git** | 필수 설치 + 자동 형상관리 | GitHub 계정은 팀만 필수 |
| **Supervisor 비용** | 팀장/지정자 개인키 | 위임 가능 |
| **클라우드 인프라** | Fly.io Machines | 빠른 시작, 사용량 과금 |
| **샌드박스** | Docker + gVisor | 보안/성능 균형 |
| **LLM 비용** | BYOK 기본 + Managed 옵션 | Managed는 +20% 마크업 |

### 팀 협업 설계 결정

| 항목 | 결정 |
|------|------|
| Supervisor 역할 | 위임 가능 (supervisor_id 필드) |
| 태스크 가시성 | 조회 전체, 실행 할당만 |
| 태스크 할당 | 부하 기반 자동 + 수동 오버라이드 |
| Repair 태스크 | original_worker 제외 + 힌트 |
| 체크포인트 | 태스크에 명시적 마킹 |
| 충돌 방지 | 스코프 분리 + Git merge |

### 사용자 그룹

| 그룹 | 대상 | 인터페이스 | 실행 위치 | 필요 Phase |
|------|------|------------|-----------|------------|
| **1. 개인 IDE** | 개발자 | Claude Code/Cursor | 로컬 | 현재 지원 |
| **2. 개인 채팅 (클라우드)** | 비개발자 | 웹 채팅 UI | 클라우드 | Phase 5, 7 |
| **3. 개인 채팅 (로컬)** | IDE 없이 로컬 원함 | 웹 채팅 UI + CLI | 로컬 | Phase 4, 5 |
| **4. 팀** | 회사/팀 | IDE/웹 + CLI | 로컬 + 클라우드 | Phase 4, 5, 6 |

---

## 현재 상태 (v0.5.0)

- [x] MCP Server (Claude Code 통합)
- [x] State Machine (INIT → COMPLETE)
- [x] Multi-Worker (SQLite WAL)
- [x] Agent Routing
- [x] EARS Requirements + ADR
- [x] Verification System
- [x] 다중 플랫폼 지원 (Claude Code, Cursor)
- [x] 멀티 LLM 백엔드 (LiteLLM)

---

## Phase 3: Git 통합 강화

### 목표
- Git을 C4 내부 인프라로 활용
- 사용자 몰라도 자동 형상 관리

### 태스크

#### T-301: Git 필수 설치
- **Scope**: `install.sh`
- **DoD**:
  - [ ] Git 설치 여부 체크
  - [ ] 없으면 OS별 자동 설치 (macOS: xcode-select, Linux: apt/yum)
  - [ ] 설치 실패 시 명확한 에러 메시지

#### T-302: c4 init Git 자동화
- **Scope**: `c4/cli.py`
- **DoD**:
  - [ ] `c4 init` 시 `.git/` 없으면 `git init` 자동 실행
  - [ ] `.gitignore` 생성 (`.c4/locks/`, `.c4/workers/`, `*.log`)
  - [ ] 초기 커밋 생성: `[C4] Project initialized`

#### T-303: 자동 커밋 시스템
- **Scope**: `c4/daemon/workers.py`, `c4/hooks.py`
- **DoD**:
  - [ ] 태스크 완료 시 자동 커밋: `[C4] task_XXX: {task_name}`
  - [ ] 체크포인트 통과 시 태그 생성: `c4/checkpoint/CP-XXX`
  - [ ] 수정 완료 시 커밋: `[C4] repair: {description}`

#### T-304: 롤백 기능
- **Scope**: `c4/cli.py`
- **DoD**:
  - [ ] `c4 rollback <checkpoint>` 명령 추가
  - [ ] `git reset --hard c4/checkpoint/CP-XXX` 실행
  - [ ] 롤백 전 확인 프롬프트

#### T-305: Git 통합 테스트
- **Scope**: `tests/integration/test_git_integration.py`
- **DoD**:
  - [ ] 자동 커밋 테스트
  - [ ] 체크포인트 태그 테스트
  - [ ] 롤백 테스트

---

## Phase 4: 인증 시스템 (Supabase Auth)

### 목표
- Supabase Auth 기반 통합 인증
- 개인/팀 모두 동일한 인증 플로우
- GitHub OAuth로 Git 작업 토큰 자동 획득

### 태스크

#### T-401: Supabase 프로젝트 설정
- **Scope**: `infra/supabase/`
- **DoD**:
  - [ ] Supabase 프로젝트 생성
  - [ ] Auth Provider 설정: GitHub, Google
  - [ ] 환경변수 관리 (SUPABASE_URL, SUPABASE_KEY)

#### T-402: CLI 로그인 구현
- **Scope**: `c4/cli.py`, `c4/auth/`
- **DoD**:
  - [ ] `c4 login` 명령 구현
  - [ ] 브라우저 OAuth 플로우 (PKCE)
  - [ ] 세션 저장: `~/.c4/session.json`
  - [ ] `c4 logout` 명령 구현

#### T-403: Supabase 클라이언트
- **Scope**: `c4/auth/supabase_client.py`
- **DoD**:
  - [ ] supabase-py 클라이언트 래퍼
  - [ ] 토큰 자동 갱신 (refresh token)
  - [ ] 세션 만료 시 재로그인 프롬프트

#### T-404: GitHub 토큰 연동
- **Scope**: `c4/auth/github.py`
- **DoD**:
  - [ ] Supabase provider_token으로 GitHub 접근
  - [ ] Git 작업 시 토큰 자동 사용
  - [ ] 토큰 갱신 로직

#### T-405: 인증 테스트
- **Scope**: `tests/integration/test_auth.py`
- **DoD**:
  - [ ] 로그인 플로우 테스트
  - [ ] 토큰 갱신 테스트
  - [ ] GitHub 토큰 연동 테스트

---

## Phase 5: 채팅 UI

### 목표
- IDE 없이 웹에서 C4 사용
- 클라우드/로컬 실행 선택 가능

### 태스크

#### T-501: Chat API 서버
- **Scope**: `c4/api/`
- **DoD**:
  - [ ] FastAPI 기반 Chat API
  - [ ] SSE 스트림 응답 (실시간 진행상황)
  - [ ] 엔드포인트: `POST /api/chat/message`
  - [ ] 엔드포인트: `GET /api/project/{id}/files`
  - [ ] 엔드포인트: `GET /api/project/{id}/download`

#### T-502: 로컬 UI 서버
- **Scope**: `c4/cli.py`, `c4/ui/`
- **DoD**:
  - [ ] `c4 ui` 명령 구현
  - [ ] 로컬 웹 서버 시작: `http://localhost:4000`
  - [ ] 정적 파일 서빙 (React 빌드)

#### T-503: 웹 프론트엔드
- **Scope**: `web/` (새 디렉토리)
- **DoD**:
  - [ ] Next.js 프로젝트 설정
  - [ ] 채팅 UI 컴포넌트
  - [ ] 프로젝트 목록/상세 페이지
  - [ ] 파일 트리 뷰어
  - [ ] 진행률 표시
  - [ ] ZIP 다운로드 버튼

#### T-504: 로컬 연결 기능
- **Scope**: `c4/cli.py`
- **DoD**:
  - [ ] `c4 connect <project_id>` 명령 구현
  - [ ] 웹 프로젝트와 로컬 워커 연결
  - [ ] WebSocket으로 명령 수신

#### T-505: 채팅 UI 테스트
- **Scope**: `tests/e2e/test_chat_ui.py`
- **DoD**:
  - [ ] Chat API 테스트
  - [ ] 로컬 연결 테스트
  - [ ] E2E 플로우 테스트

---

## Phase 6: 팀 협업

### 목표
- Supabase 기반 상태 공유
- 중앙 Supervisor 리뷰 (팀장 개인키 사용)
- 다중 워커 태스크 분배 (Peer Review)

### Supabase 스키마

```sql
-- 팀
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    created_by UUID REFERENCES auth.users(id),
    supervisor_id UUID REFERENCES auth.users(id),  -- Supervisor 역할자
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 팀 멤버
CREATE TABLE team_members (
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID REFERENCES auth.users(id) ON DELETE CASCADE,
    role TEXT DEFAULT 'member',  -- owner, admin, member
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

-- 프로젝트
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id),
    owner_id UUID REFERENCES auth.users(id),
    name TEXT NOT NULL,
    github_repo TEXT,
    status TEXT DEFAULT 'INIT',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- C4 상태
CREATE TABLE c4_state (
    project_id UUID PRIMARY KEY REFERENCES projects(id),
    state_json JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 태스크
CREATE TABLE c4_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id),
    task_id TEXT NOT NULL,
    task_json JSONB NOT NULL,
    status TEXT DEFAULT 'pending',
    assigned_to UUID REFERENCES auth.users(id),
    original_worker UUID,  -- Peer Review용
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (project_id, task_id)
);

-- 워커 세션
CREATE TABLE c4_workers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id),
    user_id UUID REFERENCES auth.users(id),
    worker_id TEXT NOT NULL,
    state TEXT DEFAULT 'idle',
    current_task TEXT,
    last_seen TIMESTAMPTZ DEFAULT NOW()
);

-- 이벤트 로그
CREATE TABLE c4_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID REFERENCES projects(id),
    event_type TEXT NOT NULL,
    event_data JSONB,
    created_by UUID REFERENCES auth.users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Row Level Security (RLS)

```sql
-- 프로젝트: 본인 또는 팀 멤버만 접근
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
CREATE POLICY "프로젝트 접근" ON projects FOR ALL USING (
    owner_id = auth.uid() OR 
    team_id IN (SELECT team_id FROM team_members WHERE user_id = auth.uid())
);

-- 다른 테이블도 동일한 패턴으로 RLS 적용
```

### 태스크

#### T-601: Supabase 스키마 구축
- **Scope**: `infra/supabase/migrations/`
- **DoD**:
  - [ ] 위 스키마 마이그레이션 파일 생성
  - [ ] RLS 정책 설정
  - [ ] Realtime 활성화 (c4_state, c4_events)

#### T-602: SupabaseStateStore
- **Scope**: `c4/store/supabase.py`
- **DoD**:
  - [ ] `SupabaseStateStore` 클래스 구현
  - [ ] StateStore 프로토콜 준수
  - [ ] Realtime 구독 지원

#### T-603: SupabaseLockStore
- **Scope**: `c4/store/supabase.py`
- **DoD**:
  - [ ] `SupabaseLockStore` 클래스 구현
  - [ ] Row-level lock 또는 advisory lock
  - [ ] TTL 기반 자동 해제

#### T-604: 팀/프로젝트 관리
- **Scope**: `c4/cli.py`, `c4/team/`
- **DoD**:
  - [ ] `c4 team create/list/invite` 명령
  - [ ] `c4 init --team <team_id>` 옵션
  - [ ] 팀원 역할 관리 (owner/admin/member)

#### T-605: 중앙 Supervisor
- **Scope**: `c4/supervisor/cloud_supervisor.py`
- **DoD**:
  - [ ] 팀장(supervisor_id) 개인키로 리뷰 실행
  - [ ] 체크포인트 리뷰 처리
  - [ ] GitHub PR에 리뷰 코멘트 작성
  - [ ] 수정 태스크 생성 (original_worker 기록)

#### T-606: 태스크 분배 로직
- **Scope**: `c4/daemon/task_distributor.py`
- **DoD**:
  - [ ] 우선순위 기반 태스크 할당
  - [ ] 수정 태스크는 original_worker 제외 (Peer Review)
  - [ ] 워커 idle 감지 및 태스크 재할당

#### T-607: GitHub 권한 관리
- **Scope**: `c4/integrations/github.py`
- **DoD**:
  - [ ] Organization 멤버십 확인
  - [ ] Collaborator 자동 초대
  - [ ] 권한 체크 후 태스크 할당

#### T-608: 팀 대시보드
- **Scope**: `web/`
- **DoD**:
  - [ ] 팀 프로젝트 목록
  - [ ] 실시간 진행 상황 (Supabase Realtime)
  - [ ] 워커별 태스크 현황
  - [ ] 체크포인트 리뷰 상태

#### T-609: 팀 협업 테스트
- **Scope**: `tests/integration/test_team_collaboration.py`
- **DoD**:
  - [ ] 다중 워커 태스크 분배 테스트
  - [ ] Peer Review 로직 테스트
  - [ ] 중앙 리뷰 플로우 테스트

#### T-610: SQLite → Supabase 마이그레이션
- **Scope**: `c4/store/migration.py`
- **DoD**:
  - [ ] 로컬 프로젝트 → 팀 전환 시 데이터 이관
  - [ ] c4_state, c4_tasks 마이그레이션
  - [ ] 롤백 지원

---

## Phase 7: 클라우드 실행

### 목표
- 설치 없이 웹에서 바로 사용
- 클라우드 워커 (샌드박스)
- 사용량 기반 과금

### 아키텍처

```
┌─────────────────────────────────────────────────────────┐
│                    클라우드 실행 흐름                    │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  [사용자 요청] ─────────────────────────────────────┐   │
│       │                                              │   │
│       ▼                                              │   │
│  ┌─────────┐    ┌─────────┐    ┌─────────────────┐  │   │
│  │ Web UI  │───▶│ FastAPI │───▶│ Fly.io Machine  │  │   │
│  └─────────┘    └─────────┘    │ (Docker+gVisor) │  │   │
│                      │         └────────┬────────┘  │   │
│                      │                  │           │   │
│                      ▼                  ▼           │   │
│               ┌─────────────┐    ┌─────────────┐   │   │
│               │  Supabase   │    │ Git Clone   │   │   │
│               │ (상태/과금) │    │ or Upload   │   │   │
│               └─────────────┘    └─────────────┘   │   │
│                                         │           │   │
│                                         ▼           │   │
│                                  ┌─────────────┐   │   │
│                                  │ 결과물 반환 │◀──┘   │
│                                  │ PR / ZIP    │       │
│                                  └─────────────┘       │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### 샌드박스 보안 레이어

```
Layer 1: 컨테이너 격리 (Docker)
Layer 2: 런타임 샌드박스 (gVisor)
Layer 3: 네트워크 격리 (Egress 제한: GitHub, npm, pypi만)
Layer 4: 리소스 제한 (CPU 2core, Mem 4GB, Time 30min)
```

### 코드 소스 옵션

| 사용자 유형 | 소스 | 결과 반환 |
|------------|------|----------|
| 팀 (Git 연동) | GitHub clone | 자동 PR |
| 개인 (Git 연동) | GitHub clone | 브랜치 push |
| 개인 (업로드) | ZIP 업로드 (10MB) | ZIP 다운로드 |

### 과금 모델

#### LLM API 비용 옵션

| 옵션 | 설명 | 대상 |
|------|------|------|
| **BYOK** | 본인 API 키 등록 | 키 있는 사용자 |
| **Managed API** | C4가 대행 (+20%) | 키 없는 사용자 |
| **팀 공용키** | 팀장 키 공유 | 팀 |

#### Managed API 요금

| 모델 | 원가 (Input/Output) | C4 가격 |
|------|---------------------|---------|
| Claude Sonnet | $3 / $15 per 1M | $3.6 / $18 |
| Claude Opus | $15 / $75 per 1M | $18 / $90 |
| GPT-4o | $2.5 / $10 per 1M | $3 / $12 |
| Gemini Pro | $1.25 / $5 per 1M | $1.5 / $6 |

#### 플랜별 요금

| 플랜 | 가격 | 클라우드 실행 | LLM | 크레딧 |
|------|------|--------------|-----|--------|
| **Free** | $0 | 100분/월 | BYOK만 | - |
| **Pro** | $29/월 | 1000분/월 | BYOK/Managed | $10 |
| **Team** | $19/user/월 | 1000분/user | BYOK/Managed/공용 | $20/팀 |
| **Enterprise** | 문의 | 무제한 | 볼륨 할인 | 협의 |

### 태스크

#### T-701: 클라우드 워커 인프라
- **Scope**: `infra/workers/`
- **DoD**:
  - [ ] Fly.io Machines 설정
  - [ ] 워커 Docker 이미지 (Python + Node + Git)
  - [ ] 동적 스케일링 (0 → N)
  - [ ] 헬스체크 및 자동 재시작

#### T-702: 샌드박스 환경
- **Scope**: `c4/sandbox/`
- **DoD**:
  - [ ] gVisor 런타임 적용
  - [ ] 네트워크 Egress 제한 (allowlist)
  - [ ] 파일시스템 격리 (tmpfs)
  - [ ] 리소스 제한 (cgroups)

#### T-703: 과금 시스템
- **Scope**: `c4/billing/`
- **DoD**:
  - [ ] Stripe 연동 (구독 + 사용량)
  - [ ] BYOK / Managed 선택 UI
  - [ ] 토큰 사용량 추적
  - [ ] 실행 시간 추적
  - [ ] 플랜별 제한 적용
  - [ ] 인보이스 자동 생성

#### T-704: Managed API 프록시
- **Scope**: `c4/api/llm_proxy.py`
- **DoD**:
  - [ ] LLM API 프록시 서버
  - [ ] 사용량 미터링
  - [ ] Rate limiting (플랜별)
  - [ ] 에러 핸들링

#### T-705: 결과물 전달
- **Scope**: `c4/api/delivery.py`
- **DoD**:
  - [ ] ZIP 파일 생성 및 S3 업로드
  - [ ] 다운로드 링크 생성 (24시간 유효)
  - [ ] GitHub push (사용자 토큰)
  - [ ] PR 자동 생성

#### T-706: 클라우드 모니터링
- **Scope**: `infra/monitoring/`
- **DoD**:
  - [ ] Sentry 연동 (에러 추적)
  - [ ] 메트릭 수집 (Prometheus)
  - [ ] 대시보드 (Grafana)
  - [ ] 알림 (Slack/Discord)

#### T-707: 클라우드 테스트
- **Scope**: `tests/e2e/test_cloud_execution.py`
- **DoD**:
  - [ ] 클라우드 워커 실행 테스트
  - [ ] 샌드박스 격리 테스트
  - [ ] 과금 미터링 테스트
  - [ ] Managed API 테스트

---

## 마일스톤

```
현재 (v0.5)     Phase 3      Phase 4      Phase 5      Phase 6      Phase 7
    │              │            │            │            │            │
    │ Multi-LLM    │ Git 통합   │ 인증       │ 채팅 UI    │ 팀 협업    │ 클라우드
    │ 플랫폼 지원  │            │ (Supabase) │            │ (Supabase) │ (SaaS)
    ▼              ▼            ▼            ▼            ▼            ▼
┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐
│ v0.5.0  │─▶│ v0.6.0  │─▶│ v0.7.0  │─▶│ v0.8.0  │─▶│ v1.0.0  │─▶│ v2.0.0  │
│         │  │         │  │         │  │         │  │ (Team)  │  │ (Cloud) │
└─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘
```

### 버전별 지원 사용자 그룹

| 버전 | Phase | 그룹 1 (IDE) | 그룹 2 (채팅/클라우드) | 그룹 3 (채팅/로컬) | 그룹 4 (팀) |
|------|-------|--------------|------------------------|---------------------|-------------|
| v0.5 | 현재 | ✅ | ❌ | ❌ | ❌ |
| v0.6 | 3 | ✅ | ❌ | ❌ | ❌ |
| v0.7 | 4 | ✅ | ❌ | ❌ (인증만) | ❌ |
| v0.8 | 5a | ✅ | ❌ | ✅ | ❌ |
| v1.0 | 5b+6 | ✅ | ❌ | ✅ | ✅ |
| v2.0 | 7 | ✅ | ✅ | ✅ | ✅ |

**참고**: 
- v0.7: 인증 기능만 추가, 채팅 UI 없음
- v0.8: 기본 채팅 UI 추가 (개인용)
- v1.0: 팀 협업 + 팀 대시보드
- v2.0: 클라우드 실행 (설치 없이 사용)

---

## 기술 스택

| 레이어 | 현재 | Phase 6+ |
|--------|------|----------|
| **CLI** | Python (Typer) | 동일 |
| **상태 저장** | SQLite | SQLite + Supabase |
| **인증** | 없음 | Supabase Auth |
| **DB** | SQLite | Supabase PostgreSQL |
| **실시간** | 없음 | Supabase Realtime |
| **웹 프론트엔드** | 없음 | Next.js |
| **웹 백엔드** | 없음 | FastAPI |
| **워커** | 로컬 | 로컬 + Fly.io |
| **과금** | 없음 | Stripe |
| **모니터링** | 없음 | Sentry + Prometheus |

---

## 의존성

```
Phase 3 (Git)
    │
    ▼
Phase 4 (인증)
    │
    ├────────────────────────┐
    ▼                        ▼
Phase 5a (기본 UI)      Phase 6a (팀 백엔드)
T-501~503               T-601~607
    │                        │
    └───────────┬────────────┘
                ▼
        Phase 5b + 6b (팀 대시보드)
        T-504, T-608~610
                │
                ▼
        Phase 7 (클라우드)
```

**실행 순서**:
1. Phase 3: Git 통합 (독립)
2. Phase 4: 인증 (Phase 3 완료 후)
3. Phase 5a + 6a: 병렬 진행 가능
4. Phase 5b + 6b: 5a, 6a 완료 후
5. Phase 7: 전체 완료 후

---

## 참고 문서

- [docs/cloud/ARCHITECTURE.md](ARCHITECTURE.md) - 클라우드 아키텍처
- [docs/cloud/PRD.md](PRD.md) - 제품 요구사항
- [docs/ROADMAP.md](../ROADMAP.md) - 전체 로드맵
- [c4/store/protocol.py](../../c4/store/protocol.py) - StateStore 프로토콜

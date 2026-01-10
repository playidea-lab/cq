# C4 Roadmap

## Current Version: v0.4.0 (Agent Routing)

현재 버전은 **도메인 기반 에이전트 라우팅**을 지원합니다.

### 지원 기능

- MCP Server (Claude Code 통합) - 9개 도구
- State Machine (INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ↔ CHECKPOINT → COMPLETE)
- Multi-Worker (SQLite WAL 모드, race-condition free)
- Agent Routing (Phase 4) - 도메인별 에이전트 자동 선택 및 체이닝
- Validation Runner (lint, unit tests)
- Checkpoint System (APPROVE, REQUEST_CHANGES, REPLAN, REDESIGN)
- Slash Commands (10개)
- Stop Hook (자동 실행 유지)
- Auto Supervisor Loop

---

## 완료된 Phase

### Phase 1: Core Foundation ✅

**목표**: 기본 상태 머신 및 MCP 서버

- State Machine (INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- MCP 도구 (c4_status, c4_get_task, c4_submit 등)
- LocalFile StateStore
- 기본 Validation Runner

### Phase 2: Multi-Worker Support ✅

**목표**: 동시 작업 지원

- SQLite 기반 StateStore (WAL 모드)
- Scope Lock (동시 작업 충돌 방지)
- Worker Manager (stale recovery)
- Atomic 태스크 할당

### Phase 3: Auto Supervisor ✅

**목표**: 자동화된 체크포인트 처리

- Supervisor Loop (백그라운드 실행)
- Claude CLI Backend
- Stop Hook (작업 완료까지 유지)
- Checkpoint Queue / Repair Queue

### Phase 4: Agent Routing ✅ (Current)

**목표**: 도메인별 특화 에이전트 자동 선택

- `c4/supervisor/agent_router.py` - 도메인 → 에이전트 매핑
- Agent Chaining (frontend → test → reviewer)
- Task Type Overrides (debug → debugger)
- Handoff Instructions

**구현된 도메인**:

| Domain | Primary Agent | Chain |
|--------|--------------|-------|
| web-frontend | frontend-developer | frontend → test → reviewer |
| web-backend | backend-architect | architect → python → test → reviewer |
| fullstack | backend-architect | backend → frontend → test → reviewer |
| ml-dl | ml-engineer | ml → python → test |
| mobile-app | mobile-developer | mobile → test → reviewer |
| infra | cloud-architect | cloud → deployment |
| library | python-pro | python → docs → test → reviewer |
| unknown | general-purpose | general → reviewer |

---

## Phase 5: Enhanced Discovery & Design (계획)

**목표**: 자동화된 요구사항 수집 및 아키텍처 설계

### 5.1 EARS Requirements Gathering

**Easy Approach to Requirements Syntax** 기반 요구사항 수집:

```yaml
# .c4/specs/{feature}/requirements.yaml
feature: user-authentication
requirements:
  - type: ubiquitous
    text: "The system shall hash all passwords using bcrypt"
  - type: event-driven
    text: "When a user fails login 3 times, the system shall lock the account"
  - type: state-driven
    text: "While the user is logged in, the system shall refresh tokens every 15 minutes"
  - type: optional
    text: "Where 2FA is enabled, the system shall require verification code"
  - type: unwanted
    text: "The system shall not store plain-text passwords"
```

### 5.2 Architecture Decision Records

자동 ADR 생성 및 관리:

```yaml
# .c4/specs/{feature}/design.yaml
architecture:
  decisions:
    - id: ADR-001
      title: "Use JWT for authentication"
      context: "Need stateless auth for microservices"
      options:
        - name: JWT
          pros: ["Stateless", "Scalable"]
          cons: ["Token size", "Cannot revoke"]
        - name: Session
          pros: ["Simple", "Easy revoke"]
          cons: ["Stateful", "Scaling issues"]
      decision: JWT
      rationale: "Microservices architecture requires stateless auth"
```

### 5.3 Component Specification

컴포넌트 설계 자동화:

```yaml
components:
  - name: AuthService
    type: service
    responsibilities:
      - "User authentication"
      - "Token management"
    dependencies:
      - UserRepository
      - TokenStore
    interfaces:
      - name: authenticate
        input: { email: string, password: string }
        output: { token: string, user: User }
```

### 5.4 Verification System

체크포인트 시 자동 검증:

```yaml
# config.yaml
verifications:
  enabled: true
  items:
    - type: http
      name: "API Health"
      config:
        url: "http://localhost:8000/health"
        expected_status: 200
    - type: command
      name: "DB Migration"
      config:
        command: "uv run alembic current"
        expected_output: "head"
```

### 예상 구현 항목

| 기능 | 설명 | 우선순위 |
|------|------|----------|
| EARS Parser | 요구사항 템플릿 및 검증 | P0 |
| ADR Generator | 아키텍처 결정 자동 기록 | P0 |
| Component Designer | 컴포넌트 설계 템플릿 | P1 |
| HTTP Verifier | API 헬스체크 | P1 |
| Command Verifier | 명령어 기반 검증 | P1 |
| Design Reviewer | 설계 품질 검토 | P2 |

---

## Phase 6: Team Collaboration (장기 계획)

**목표**: 팀원 간 협업 지원

### State Store 추상화

```python
class StateStore(Protocol):
    def load(self, project_id: str) -> C4State: ...
    def save(self, state: C4State) -> None: ...
    def acquire_lock(self, scope: str, ttl: int) -> bool: ...
    def release_lock(self, scope: str) -> None: ...
```

### 지원 Backend

| Backend | 용도 | 복잡도 |
|---------|------|--------|
| SQLite | 현재 (기본) | 낮음 |
| Supabase | 팀 협업 | 중간 |
| Redis | 고성능 | 중간 |
| PostgreSQL | Cloud 준비 | 높음 |

### 아키텍처

```text
┌─────────────┐        ┌─────────────┐
│ Claude Code │        │ Claude Code │
│ + C4 Daemon │        │ + C4 Daemon │
└──────┬──────┘        └──────┬──────┘
       │                      │
       └──────────┬───────────┘
                  ▼
           ┌────────────┐
           │  Supabase  │
           │  (State)   │
           └────────────┘
```

---

## Phase 7: C4 Cloud (장기 계획)

**목표**: 완전 관리형 SaaS 버전

### 주요 기능

- Web Dashboard
- 원격 Worker Pool
- GitHub 통합 (Auto PR)
- 사용량 기반 과금
- 팀/조직 관리

### 아키텍처

```text
┌─────────────────────────────────────────────────────────────┐
│                      C4 Cloud                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Web Console │  │ API Gateway │  │ Worker Orchestrator │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                          │                                   │
│              ┌───────────┴───────────┐                      │
│              ▼                       ▼                      │
│       ┌────────────┐          ┌────────────┐               │
│       │ PostgreSQL │          │   Redis    │               │
│       │  (State)   │          │  (Locks)   │               │
│       └────────────┘          └────────────┘               │
└─────────────────────────────────────────────────────────────┘
```

---

## Migration Path

```text
v0.1-0.3        v0.4 (현재)       v0.5            v0.6+
    │               │               │               │
    │  Multi-Worker │  Agent Routing│  Team         │
    │  SQLite       │  + Chaining   │  Collaboration│
    ▼               ▼               ▼               ▼
┌─────────┐   ┌─────────┐    ┌─────────┐    ┌─────────┐
│ Local   │──▶│ Agent   │───▶│ Supabase│───▶│ Cloud   │
│ Files   │   │ Routing │    │ / Redis │    │ Managed │
└─────────┘   └─────────┘    └─────────┘    └─────────┘
```

---

## 우선순위

| 기능 | 우선순위 | 상태 |
|------|----------|------|
| 단일 사용자 완성 | P0 | ✅ 완료 |
| Multi-Worker | P0 | ✅ 완료 |
| Auto Supervisor | P0 | ✅ 완료 |
| Agent Routing | P0 | ✅ 완료 |
| 문서화 | P0 | 🔄 진행중 |
| EARS Requirements | P1 | 📋 Phase 5 |
| ADR Generator | P1 | 📋 Phase 5 |
| Verification System | P1 | 📋 Phase 5 |
| Team Collaboration | P2 | 📋 Phase 6 |
| Cloud API | P3 | 📋 Phase 7 |

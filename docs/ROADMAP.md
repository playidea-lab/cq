# C4 Roadmap

## Current Version: v0.1.0 (Single-User Local)

현재 버전은 **단일 사용자 로컬 환경**에 최적화되어 있습니다.

### 지원 기능
- MCP Server (Claude Code 통합)
- State Machine (INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE)
- Multi-Worker (같은 머신, 같은 Daemon)
- Validation Runner (lint, unit tests)
- Checkpoint System (APPROVE, REQUEST_CHANGES, REPLAN)
- Slash Commands (/c4-status, /c4-worker, /c4-submit 등)

### 제한 사항
- 단일 머신에서만 동작
- 팀 협업 시 Git 충돌 가능 (.c4/state.json)
- 원격 동기화 미지원

---

## v0.2.0 (Team Collaboration) - 계획

### 목표
팀원 간 협업 지원

### 주요 기능

#### State Store 추상화
```python
class StateStore(Protocol):
    def load(self, project_id: str) -> C4State: ...
    def save(self, state: C4State) -> None: ...
    def acquire_lock(self, scope: str, ttl: int) -> bool: ...
    def release_lock(self, scope: str) -> None: ...
```

#### 지원 Backend
| Backend | 용도 | 복잡도 |
|---------|------|--------|
| LocalFile | 현재 (기본) | 낮음 |
| Supabase | 팀 협업 | 중간 |
| Redis | 고성능 | 중간 |
| PostgreSQL | Cloud 준비 | 높음 |

#### 설정 예시
```yaml
# .c4/config.yaml
sync:
  backend: supabase
  url: https://xxx.supabase.co
  key: ${SUPABASE_KEY}
```

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

## v1.0.0 (C4 Cloud) - 장기 계획

### 목표
완전 관리형 SaaS 버전

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

### MCP 연결 옵션
```bash
# Option A: HTTP Transport (권장)
claude mcp add --transport http c4-cloud https://api.c4.dev/mcp

# Option B: 로컬 Daemon + Cloud State
# config.yaml에서 backend: cloud 설정
```

---

## Migration Path

```text
v0.1.0 (현재)     v0.2.0 (팀)        v1.0.0 (Cloud)
    │                 │                  │
    │  State Store    │   Cloud API      │
    │  추상화 추가     │   연결           │
    ▼                 ▼                  ▼
┌─────────┐     ┌─────────┐       ┌─────────┐
│ Local   │ ──► │ Supabase│ ───►  │ Cloud   │
│ Files   │     │ / Redis │       │ Managed │
└─────────┘     └─────────┘       └─────────┘
```

각 단계에서:
1. 기존 기능 100% 호환
2. 설정만 변경하면 업그레이드
3. 데이터 마이그레이션 도구 제공

---

## 우선순위

| 기능 | 우선순위 | 상태 |
|------|----------|------|
| 단일 사용자 완성 | P0 | ✅ 완료 |
| 문서화 | P0 | 🔄 진행중 |
| State Store 추상화 | P1 | 📋 계획 |
| Supabase 통합 | P1 | 📋 계획 |
| Cloud API | P2 | 📋 계획 |
| Web Dashboard | P2 | 📋 계획 |

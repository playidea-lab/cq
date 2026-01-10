# C4 Development Phases

이 문서는 C4의 개발 단계별 구현 내용을 상세히 기록합니다.

---

## Phase 1: Core Foundation ✅

> **완료일**: 2025-12
> **버전**: v0.1.0

### 목표

기본 상태 머신 및 MCP 서버 구현

### 구현 내용

#### 1.1 State Machine

```
INIT → PLAN → EXECUTE → CHECKPOINT → COMPLETE
                ↓            ↓
              HALTED ←───────┘
```

**상태 정의**:
- `INIT`: 프로젝트 초기화
- `PLAN`: 태스크 계획 수립
- `EXECUTE`: 워커 실행
- `CHECKPOINT`: Supervisor 리뷰
- `HALTED`: 실행 중지
- `COMPLETE`: 프로젝트 완료

#### 1.2 MCP 도구

| 도구 | 설명 |
|------|------|
| `c4_status` | 상태 조회 |
| `c4_get_task` | 태스크 할당 |
| `c4_submit` | 태스크 제출 |
| `c4_add_todo` | 태스크 추가 |
| `c4_checkpoint` | 체크포인트 결정 |
| `c4_run_validation` | 검증 실행 |

#### 1.3 StateStore

```python
class LocalFileStateStore:
    def load(self, project_id: str) -> C4State: ...
    def save(self, state: C4State) -> None: ...
```

- 파일 기반 저장 (`.c4/state.json`)
- 단일 워커 전용

#### 1.4 Validation Runner

```yaml
validations:
  lint:
    command: "uv run ruff check"
  unit:
    command: "uv run pytest tests/unit"
```

### 파일

```
c4/
├── mcp_server.py      # C4Daemon
├── state_machine.py   # StateMachine
├── validation.py      # ValidationRunner
├── models/
│   ├── enums.py       # ProjectStatus
│   ├── task.py        # Task
│   ├── state.py       # C4State
│   └── responses.py   # TaskAssignment
└── store/
    ├── protocol.py    # StateStore ABC
    └── local_file.py  # LocalFileStateStore
```

---

## Phase 2: Multi-Worker Support ✅

> **완료일**: 2026-01
> **버전**: v0.2.0

### 목표

동시 작업 및 race condition 방지

### 구현 내용

#### 2.1 SQLite StateStore

```python
class SQLiteStateStore:
    def __init__(self, db_path: Path):
        # WAL 모드로 동시성 지원
        self._conn.execute("PRAGMA journal_mode=WAL")

    def atomic_modify(self, project_id: str, modifier: Callable) -> C4State:
        # 원자적 상태 수정
```

**테이블 구조**:
```sql
CREATE TABLE c4_state (
    project_id TEXT PRIMARY KEY,
    data TEXT NOT NULL,  -- JSON
    updated_at TEXT NOT NULL
);
```

#### 2.2 Scope Lock

```python
class SQLiteLockStore:
    def acquire_scope_lock(
        self, project_id: str, scope: str, owner: str, ttl_seconds: int
    ) -> bool: ...

    def release_scope_lock(self, project_id: str, scope: str) -> bool: ...
```

**테이블 구조**:
```sql
CREATE TABLE c4_locks (
    project_id TEXT,
    scope TEXT,
    owner TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (project_id, scope)
);
```

**동작**:
- 같은 scope에 하나의 워커만 작업 가능
- TTL 30분 (자동 만료)
- Stale 워커 복구 시 락 자동 해제

#### 2.3 Worker Manager

```python
class WorkerManager:
    def register(self, worker_id: str) -> WorkerInfo: ...
    def heartbeat(self, worker_id: str) -> None: ...
    def recover_stale_workers(self, stale_timeout: int) -> list[dict]: ...
```

**Stale Recovery**:
- 30분 무응답 워커 감지
- 해당 워커의 태스크를 `pending`으로 복귀
- Scope Lock 해제

#### 2.4 Task Store

```python
class SQLiteTaskStore:
    def get_pending_tasks(self, project_id: str) -> list[Task]: ...
    def assign_task(self, project_id: str, task_id: str, worker_id: str) -> Task: ...
    def complete_task(self, project_id: str, task_id: str) -> Task: ...
```

### 파일

```
c4/
├── store/
│   └── sqlite.py      # SQLiteStateStore, SQLiteLockStore, SQLiteTaskStore
└── daemon/
    └── workers.py     # WorkerManager
```

---

## Phase 3: Auto Supervisor ✅

> **완료일**: 2026-01
> **버전**: v0.3.0

### 목표

자동화된 체크포인트 처리 및 연속 실행

### 구현 내용

#### 3.1 Supervisor Loop

```python
class SupervisorLoop:
    async def run(self):
        while not self._stop_event.is_set():
            # 1. checkpoint_queue 처리
            await self._process_checkpoint_queue()

            # 2. repair_queue 처리
            await self._process_repair_queue()

            # 3. stale 워커 복구
            await self._recover_stale_workers()

            await asyncio.sleep(self._poll_interval)
```

**비동기 큐**:
- `checkpoint_queue`: Supervisor 리뷰 대기
- `repair_queue`: 복구 필요 태스크

#### 3.2 Claude CLI Backend

```python
class ClaudeCliBackend(SupervisorBackend):
    async def review_checkpoint(
        self, prompt: str, context: dict
    ) -> SupervisorResponse:
        # Claude CLI 호출
        result = subprocess.run(
            ["claude", "--print", "-p", prompt],
            capture_output=True
        )
        return self._parse_response(result.stdout)
```

#### 3.3 Stop Hook

```python
# .claude/settings.json
{
  "hooks": {
    "Stop": [{
      "type": "command",
      "command": "uv --directory $C4_DIR run python -m c4.hooks.stop"
    }]
  }
}
```

**동작**:
- Claude Code 종료 시도 시 실행
- 남은 태스크가 있으면 종료 차단
- 모든 태스크 완료 시 종료 허용

#### 3.4 c4_start 도구

```python
@mcp.tool()
def c4_start() -> dict:
    """PLAN/HALTED → EXECUTE 전환"""
    daemon.state_machine.transition("c4_run")
    daemon._start_supervisor_loop()
    return {"success": True, "status": "EXECUTE"}
```

### 파일

```
c4/
├── daemon/
│   └── supervisor_loop.py  # SupervisorLoop
├── supervisor/
│   ├── backend.py          # SupervisorBackend ABC
│   ├── claude_backend.py   # ClaudeCliBackend
│   └── mock_backend.py     # MockBackend
└── hooks/
    └── stop.py             # Stop Hook
```

---

## Phase 4: Agent Routing ✅

> **완료일**: 2026-01-10
> **버전**: v0.4.0

### 목표

도메인별 특화 에이전트 자동 선택 및 체이닝

### 구현 내용

#### 4.1 Agent Router

```python
# c4/supervisor/agent_router.py

DOMAIN_AGENT_MAP = {
    "web-frontend": AgentChainConfig(
        primary="frontend-developer",
        chain=["frontend-developer", "test-automator", "code-reviewer"],
        description="React/Vue/Angular components",
        handoff_instructions="Pass component specs..."
    ),
    "web-backend": AgentChainConfig(
        primary="backend-architect",
        chain=["backend-architect", "python-pro", "test-automator", "code-reviewer"],
        description="REST APIs and backend services",
        handoff_instructions="Pass API specs..."
    ),
    # ... 기타 도메인
}

def get_recommended_agent(domain: str | Domain | None) -> AgentChainConfig:
    """도메인에 맞는 에이전트 설정 반환"""
```

#### 4.2 도메인 매핑

| Domain | Primary Agent | Chain |
|--------|--------------|-------|
| `web-frontend` | `frontend-developer` | frontend → test → reviewer |
| `web-backend` | `backend-architect` | architect → python → test → reviewer |
| `fullstack` | `backend-architect` | backend → frontend → test → reviewer |
| `ml-dl` | `ml-engineer` | ml → python → test |
| `mobile-app` | `mobile-developer` | mobile → test → reviewer |
| `infra` | `cloud-architect` | cloud → deployment |
| `library` | `python-pro` | python → docs → test → reviewer |
| `firmware` | `general-purpose` | general → test |
| `unknown` | `general-purpose` | general → reviewer |

#### 4.3 Task Type Overrides

```python
TASK_TYPE_AGENT_OVERRIDES = {
    "debug": "debugger",
    "security": "security-auditor",
    "performance": "performance-engineer",
    "refactor": "code-refactorer",
    "test": "test-automator",
    "deploy": "deployment-engineer",
    "database": "database-optimizer",
    "docs": "api-documenter",
}
```

#### 4.4 TaskAssignment 확장

```python
class TaskAssignment(BaseModel):
    task_id: str
    title: str
    scope: str | None
    dod: str
    validations: list[str]
    branch: str
    # Phase 4 필드
    recommended_agent: str | None = None
    agent_chain: list[str] | None = None
    domain: str | None = None
    handoff_instructions: str | None = None
```

#### 4.5 c4_get_task 확장

```python
def c4_get_task(self, worker_id: str) -> TaskAssignment | None:
    task = self._assign_next_task(worker_id)
    if not task:
        return None

    # Phase 4: 에이전트 라우팅 정보 추가
    domain = task.domain or self._config.domain
    agent_config = get_recommended_agent(domain)

    return TaskAssignment(
        task_id=task.id,
        title=task.title,
        dod=task.dod,
        # ...
        recommended_agent=agent_config.primary,
        agent_chain=agent_config.chain,
        domain=domain,
        handoff_instructions=agent_config.handoff_instructions,
    )
```

#### 4.6 Agent Handoff

```python
@dataclass
class AgentHandoff:
    from_agent: str
    to_agent: str
    summary: str
    files_modified: list[str] = field(default_factory=list)
    next_steps: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)

    def to_prompt(self) -> str:
        """마크다운 형식의 핸드오프 컨텍스트 생성"""
```

### 파일

```
c4/
├── supervisor/
│   └── agent_router.py    # NEW: AgentRouter
├── models/
│   ├── task.py            # MODIFIED: domain 필드 추가
│   └── responses.py       # MODIFIED: agent routing 필드 추가
└── mcp_server.py          # MODIFIED: c4_get_task 확장

tests/
├── unit/
│   └── test_agent_router.py       # NEW
└── integration/
    └── test_agent_routing.py      # NEW
```

### 테스트

```bash
# 에이전트 라우팅 테스트
uv run pytest tests/unit/test_agent_router.py -v
uv run pytest tests/integration/test_agent_routing.py -v
```

---

## Phase 5: Enhanced Discovery & Design ✅

> **완료일**: 2026-01-10
> **버전**: v0.5.0

### 목표

자동화된 요구사항 수집, 아키텍처 설계, 런타임 검증

### 구현 내용

#### 5.1 EARS Requirements (`c4/discovery/specs.py`)

**Easy Approach to Requirements Syntax** 기반 요구사항 수집:

```python
class EARSPattern(str, Enum):
    UBIQUITOUS = "ubiquitous"      # The system shall...
    STATE_DRIVEN = "state-driven"  # While X, the system shall...
    EVENT_DRIVEN = "event-driven"  # When X, the system shall...
    OPTIONAL = "optional"          # Where X, the system shall...
    UNWANTED = "unwanted"          # The system shall not...

class EARSRequirement(BaseModel):
    id: str
    pattern: EARSPattern
    text: str
    rationale: str | None = None
    priority: str = "should"  # must, should, could
```

| 타입 | 패턴 | 예시 |
|------|------|------|
| Ubiquitous | The system shall... | "The system shall hash passwords" |
| Event-driven | When X, the system shall... | "When login fails 3 times, lock account" |
| State-driven | While X, the system shall... | "While logged in, refresh tokens" |
| Optional | Where X, the system shall... | "Where 2FA enabled, require code" |
| Unwanted | The system shall not... | "The system shall not store plain-text" |

#### 5.2 ADR (Architecture Decision Records) (`c4/discovery/design.py`)

```python
class DesignDecision(BaseModel):
    id: str                              # ADR-001
    question: str                        # 무엇을 결정해야 하는가?
    decision: str                        # 결정 내용
    rationale: str                       # 이유
    alternatives_considered: list[str]   # 검토한 대안들
    timestamp: datetime

class ArchitectureOption(BaseModel):
    name: str
    description: str
    pros: list[str]
    cons: list[str]
    estimated_effort: str | None = None
```

**YAML 형식**:
```yaml
decisions:
  - id: ADR-001
    question: "Choose authentication method"
    decision: "Use JWT"
    rationale: "Microservices architecture requires stateless auth"
    alternatives_considered: ["Session cookies", "OAuth only"]
```

#### 5.3 Component Specification (`c4/discovery/design.py`)

```python
class ComponentDesign(BaseModel):
    name: str
    type: str                    # service, repository, controller
    responsibilities: list[str]
    interfaces: list[dict]       # API 인터페이스
    dependencies: list[str]
    notes: str | None = None

class DataFlowStep(BaseModel):
    from_component: str
    to_component: str
    description: str
    data_type: str | None = None
```

#### 5.4 Verification System (`c4/supervisor/verifier.py`)

**6개 Verifier 구현**:

| Verifier | 타입 | 설명 |
|----------|------|------|
| `HttpVerifier` | `http` | API 헬스체크, 응답 검증 |
| `CliVerifier` | `cli` | 명령어 실행 및 출력 검증 |
| `BrowserVerifier` | `browser` | Playwright E2E 테스트 |
| `VisualVerifier` | `visual` | 스크린샷 비교 |
| `MetricsVerifier` | `metrics` | ML/DL 성능 지표 검증 |
| `DryrunVerifier` | `dryrun` | Terraform plan 등 dry-run |

```python
# HTTP Verifier 예시
config = {
    "url": "http://localhost:8000/health",
    "method": "GET",
    "expected_status": 200,
    "expected_body": "ok"
}
result = HttpVerifier().verify(config)

# CLI Verifier 예시
config = {
    "command": "uv run pytest tests/unit",
    "expected_exit_code": 0
}
result = CliVerifier().verify(config)

# Metrics Verifier 예시
config = {
    "metrics": {"accuracy": 0.95, "latency_ms": 50},
    "thresholds": {
        "accuracy": {"min": 0.90},
        "latency_ms": {"max": 100}
    }
}
result = MetricsVerifier().verify(config)
```

**도메인별 기본 검증**:
```python
DOMAIN_DEFAULT_VERIFICATIONS = {
    "web-frontend": [{"type": "browser"}, {"type": "visual"}],
    "web-backend": [{"type": "http"}, {"type": "cli"}],
    "ml-dl": [{"type": "cli"}, {"type": "metrics"}],
    "infra": [{"type": "cli"}, {"type": "dryrun"}],
}
```

#### 5.5 State Machine 확장

DISCOVERY와 DESIGN 상태 추가:

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ↔ CHECKPOINT → COMPLETE
```

**새로운 전이**:
- `discovery_complete`: DISCOVERY → DESIGN
- `design_approved`: DESIGN → PLAN
- `design_rejected`: DESIGN → DISCOVERY (재인터뷰)
- `redesign`: CHECKPOINT → DESIGN (설계 재검토)

#### 5.6 MCP 도구 (10개 추가)

| 도구 | 설명 |
|------|------|
| `c4_save_spec` | Feature 명세 저장 |
| `c4_list_specs` | 모든 Feature 명세 목록 |
| `c4_get_spec` | Feature 명세 조회 |
| `c4_add_verification` | Feature에 검증 추가 |
| `c4_get_feature_verifications` | Feature 검증 목록 |
| `c4_discovery_complete` | Discovery → Design 전환 |
| `c4_save_design` | Design 저장 |
| `c4_get_design` | Design 조회 |
| `c4_list_designs` | 모든 Design 목록 |
| `c4_design_complete` | Design → Plan 전환 |

### 파일

```
c4/
├── discovery/
│   ├── __init__.py
│   ├── models.py      # Domain, DomainSignal, ProjectOverview
│   ├── specs.py       # EARSPattern, EARSRequirement, FeatureSpec, SpecStore
│   └── design.py      # ArchitectureOption, ComponentDesign, DesignDecision, DesignStore
├── supervisor/
│   └── verifier.py    # VerifierRegistry, HttpVerifier, CliVerifier, BrowserVerifier, ...
└── mcp_server.py      # 10개 Phase 5 MCP 도구

tests/
├── unit/
│   ├── test_discovery.py  # EARS, FeatureSpec, SpecStore 테스트
│   └── test_verifier.py   # 6개 Verifier 테스트
```

### 테스트

```bash
# Discovery 테스트 (30개)
uv run pytest tests/unit/test_discovery.py -v

# Verifier 테스트 (46개)
uv run pytest tests/unit/test_verifier.py -v
```

---

## Phase 6: Team Collaboration (장기)

> **상태**: 📋 계획
> **예상 버전**: v0.6.0

### 목표

팀원 간 실시간 협업

### 예상 기능

- Supabase/Redis StateStore
- 분산 Lock
- 실시간 상태 동기화
- 팀 대시보드

---

## Phase 7: C4 Cloud (장기)

> **상태**: 📋 계획
> **예상 버전**: v1.0.0

### 목표

완전 관리형 SaaS

### 예상 기능

- Web Dashboard
- 원격 Worker Pool
- GitHub 통합
- 과금 시스템

---

## 버전 히스토리

| 버전 | Phase | 주요 기능 | 날짜 |
|------|-------|-----------|------|
| v0.1.0 | 1 | Core Foundation | 2025-12 |
| v0.2.0 | 2 | Multi-Worker | 2026-01 |
| v0.3.0 | 3 | Auto Supervisor | 2026-01 |
| v0.4.0 | 4 | Agent Routing | 2026-01-10 |
| v0.5.0 | 5 | Discovery & Design | 2026-01-10 |
| v0.6.0 | 6 | Team Collaboration | (계획) |
| v1.0.0 | 7 | C4 Cloud | (계획) |

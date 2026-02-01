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
    "review": "code-reviewer",        # Review-as-Task 지원
    "code-review": "code-reviewer",
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

## Phase 5.1: Dogfooding Fixes ✅

> **완료일**: 2026-01-10
> **버전**: v0.5.1

### 목표

도그푸딩 테스트 결과 발견된 버그 수정 및 DX 개선

### 구현 내용

#### 5.1.1 Event ID 원자적 생성

**문제**: 멀티 워커 환경에서 이벤트 ID 중복 발생 (race condition)

**해결**: SQLite `atomic_modify` 사용

```python
# c4/state_machine.py
def _get_next_event_id(self) -> str:
    """SQLite를 통해 원자적으로 다음 이벤트 ID 획득"""
    with self._store.atomic_modify(self._project_id) as state:
        next_id = state.metrics.events_emitted + 1
        state.metrics.events_emitted = next_id
        return f"{next_id:06d}"
```

- Derived Status Pattern과 일관성 유지
- 파일 기반 폴백 (SQLite 미사용 시)

#### 5.1.2 기본 체크포인트 자동 생성

**문제**: 체크포인트 미설정 시 Supervisor 리뷰 생략됨

**해결**: `c4 init` 시 기본 체크포인트 자동 생성

```python
# c4/models/checkpoint.py
DEFAULT_CHECKPOINTS = [
    CheckpointConfig(
        id="CP-REVIEW",
        description="코드 리뷰 완료 후 Supervisor 검토",
        required_tasks=[],
        required_validations=["lint"],
    ),
    CheckpointConfig(
        id="CP-FINAL",
        description="모든 작업 완료 후 최종 검토",
        required_tasks=[],
        required_validations=["lint", "unit"],
    ),
]
```

- `with_default_checkpoints=False` 옵션으로 비활성화 가능
- CheckpointConfig에 `description` 필드 추가

#### 5.1.3 Review Parser 모듈

**문제**: 리뷰 리포트의 이슈가 수동으로만 태스크로 변환됨

**해결**: `c4/review_parser.py` 모듈 추가

```python
from c4.review_parser import parse_review_report, issues_to_task_titles

# 리뷰 리포트 파싱
issues = parse_review_report(Path(".c4/review-report.md"))

# Critical/High 이슈만 태스크로 변환
tasks = issues_to_task_titles(issues, min_severity="High")
# ["[Critical] Security vulnerability in auth", "[High] Missing error handling"]

# REQUEST_CHANGES용 형식
changes = issues_to_required_changes(issues, min_severity="High")
```

**지원 기능**:
- Markdown 리뷰 리포트 파싱
- 심각도별 필터링 (Critical, High, Medium, Low)
- 태스크 제목 또는 required_changes 형식 변환

### 파일

```
c4/
├── state_machine.py       # MODIFIED: atomic event ID
├── models/
│   └── checkpoint.py      # MODIFIED: DEFAULT_CHECKPOINTS, description
├── mcp_server.py          # MODIFIED: with_default_checkpoints 옵션
└── review_parser.py       # NEW: 리뷰 리포트 파서

tests/
└── unit/
    └── test_review_parser.py  # NEW: 16개 테스트
```

### 테스트

```bash
# 리뷰 파서 테스트
uv run pytest tests/unit/test_review_parser.py -v
```

---

## Phase 5.5: Skill System Enhancement ✅

> **완료일**: 2026-01-15
> **버전**: v0.5.5

### 목표

확장 가능한 스킬 시스템으로 에이전트 라우팅 고도화

### 구현 내용

#### 5.5.1 스킬 스키마 V2

```python
# c4/supervisor/agent_graph/schema/skill.schema.yaml
class SkillConfig:
    name: str
    impact: str  # critical, high, medium, low
    domains: list[str]  # 적용 도메인
    rules: list[Rule]  # 임베딩 규칙
    dependencies: Dependencies  # required, optional
```

**Impact 우선순위**:
- `critical`: 2.0 (보안, 데이터 무결성)
- `high`: 1.5 (핵심 기능)
- `medium`: 1.0 (일반 개선)
- `low`: 0.5 (선택적)

#### 5.5.2 도메인별 스킬 (18개)

| 카테고리 | 스킬 |
|----------|------|
| **범용** | debugging, testing, code-review, error-handling, security-scanning |
| **ML/DL** | experiment-tracking, model-optimization |
| **Data Science** | data-analysis, feature-engineering, statistical-testing |
| **Frontend** | react-optimization, accessibility |
| **Backend** | api-design, database-optimization, authentication, caching-strategy |
| **Infra** | deployment, monitoring, container-orchestration |

#### 5.5.3 스킬 매처

```python
# c4/supervisor/agent_graph/skill_matcher.py
class SkillMatcher:
    def match(self, task: Task, context: Context) -> list[MatchedSkill]:
        # 공식: score = impact_weight × (1 + domain_boost) + rule_bonus
```

#### 5.5.4 스킬 CLI

```bash
c4 skill list              # 스킬 목록
c4 skill validate          # 스킬 검증
c4 skill info <name>       # 스킬 상세
```

### 파일

```
c4/supervisor/agent_graph/
├── schema/
│   └── skill.schema.yaml     # 스킬 스키마 V2
├── skills/
│   ├── _meta/                # 범용 스킬
│   ├── frontend/             # 프론트엔드
│   ├── backend/              # 백엔드
│   ├── ml-dl/                # ML/DL
│   ├── data-science/         # 데이터 사이언스
│   ├── infra/                # 인프라
│   └── _groups.yaml          # 스킬 그룹
└── skill_matcher.py          # SkillMatcher
```

---

## Phase 6: Team Collaboration ✅

> **완료일**: 2026-01-25
> **버전**: v0.6.0

### 목표

Supabase 기반 팀원 간 실시간 협업

### 구현 내용

#### 6.1 Supabase State Store

```python
# c4/store/supabase.py
class SupabaseStateStore:
    """분산 프로젝트 상태 관리 (RLS 적용)"""
    async def load(self, project_id: str) -> C4State: ...
    async def save(self, state: C4State) -> None: ...
    async def atomic_modify(self, project_id: str, modifier: Callable) -> C4State: ...

class SupabaseLockStore:
    """분산 잠금 관리 (RLS 적용)"""
    async def acquire_scope_lock(self, project_id: str, scope: str, owner: str) -> bool: ...
    async def release_scope_lock(self, project_id: str, scope: str) -> bool: ...
```

#### 6.2 팀 관리 서비스

```python
# c4/services/teams.py
class TeamService:
    async def create_team(self, name: str, owner_id: str) -> Team: ...
    async def add_member(self, team_id: str, user_id: str, role: Role) -> Member: ...
    async def update_member_role(self, team_id: str, user_id: str, role: Role) -> None: ...
```

**역할 (RBAC)**:
- `owner`: 전체 권한
- `admin`: 멤버 관리, 설정 변경
- `member`: 태스크 실행, 리뷰
- `viewer`: 읽기 전용

#### 6.3 Cloud Supervisor

```python
# c4/supervisor/cloud_supervisor.py
class CloudSupervisor:
    """팀 전체 리뷰 및 체크포인트 관리"""
    async def process_team_checkpoints(self, team_id: str) -> None: ...
    async def distribute_reviews(self, team_id: str) -> None: ...
```

#### 6.4 Task Dispatcher

```python
# c4/daemon/task_dispatcher.py
class TaskDispatcher:
    """우선순위 기반 태스크 분배"""
    async def dispatch(self, team_id: str) -> list[TaskAssignment]: ...
    async def balance_workload(self, team_id: str) -> None: ...
```

#### 6.5 GitHub 권한 통합

```python
# c4/integrations/github.py
class GitHubClient:
    async def sync_team_permissions(self, team_id: str) -> None: ...
    async def create_pr(self, repo: str, branch: str, title: str) -> PR: ...

class GitHubAutomation:
    async def auto_create_issue(self, task: Task) -> Issue: ...
```

#### 6.6 Branding Middleware

```python
# c4/api/middleware/branding.py
class BrandingMiddleware:
    """커스텀 도메인 기반 브랜딩 적용"""
    async def __call__(self, request: Request, call_next) -> Response:
        domain = self._extract_domain(request.headers.get("host"))
        branding = await self.cache.get(domain) or await self._fetch_branding(domain)
        request.state.branding = branding
        return await call_next(request)

class BrandingCache:
    """TTL 캐시 (기본 60초)"""
```

### DB 마이그레이션

```
infra/supabase/migrations/
├── 00001_c4_projects.sql      # 프로젝트 테이블
├── 00002_c4_tasks.sql         # 태스크 테이블
├── 00003_c4_events.sql        # 이벤트 로그
├── 00004_teams_and_members.sql # 팀/멤버 테이블
├── 00005_team_settings.sql    # 팀 설정
└── 00006_team_branding.sql    # 브랜딩 설정
```

### 파일

```
c4/
├── store/
│   └── supabase.py           # SupabaseStateStore, SupabaseLockStore
├── services/
│   ├── teams.py              # TeamService
│   └── branding.py           # BrandingService
├── supervisor/
│   └── cloud_supervisor.py   # CloudSupervisor
├── daemon/
│   └── task_dispatcher.py    # TaskDispatcher
├── integrations/
│   ├── github.py             # GitHubClient, GitHubAutomation
│   └── gitlab.py             # GitLabClient, MRReviewService
└── api/
    ├── middleware/
    │   └── branding.py       # BrandingMiddleware
    └── routes/
        ├── teams.py          # Team API
        └── branding.py       # Branding API
```

---

## Phase 6.5: MCP Advanced Tools ✅

> **완료일**: 2026-01-25
> **버전**: v0.6.0

### 목표

코드 분석 및 문서화 자동화

### 구현 내용

#### 6.5.1 Code Analysis Engine

```python
# c4/services/code_analysis/
class PythonParser:
    """Python AST 분석"""
    def parse(self, source: str) -> SymbolTable: ...
    def get_dependencies(self, source: str) -> DependencyGraph: ...

class TypeScriptParser:
    """TypeScript 구문 분석"""
    def parse(self, source: str) -> SymbolTable: ...
```

**분석 기능**:
- 심볼 테이블 추출 (클래스, 함수, 변수)
- 의존성 그래프 생성
- 호출 관계 분석

#### 6.5.2 Documentation Server (MCP)

```python
# c4/mcp/docs_server.py
@mcp.tool()
def query_docs(query: str, scope: str | None = None) -> list[DocResult]:
    """문서 검색/쿼리"""

@mcp.tool()
def create_snapshot(name: str) -> Snapshot:
    """코드베이스 스냅샷 생성"""

@mcp.tool()
def get_usage_examples(symbol: str) -> list[Example]:
    """심볼 사용 예시 추출"""
```

#### 6.5.3 Gap Analyzer (MCP)

```python
# c4/mcp/gap_analyzer.py
@mcp.tool()
def analyze_spec_gaps(feature: str) -> GapReport:
    """EARS 요구사항 갭 분석"""

@mcp.tool()
def generate_tests_from_spec(feature: str) -> list[TestCase]:
    """명세 → 테스트 생성"""

@mcp.tool()
def link_impl_to_spec(impl_file: str, spec_id: str) -> LinkResult:
    """구현-명세 연결"""

@mcp.tool()
def verify_spec_completion(feature: str) -> CompletionReport:
    """완료 검증"""
```

#### 6.5.4 Public Docs API

```python
# c4/api/routes/docs.py
# Context7 스타일 REST API

@router.get("/api/docs/search")
async def search_docs(q: str) -> list[DocResult]: ...

@router.post("/api/docs/snapshots")
async def create_snapshot(name: str) -> Snapshot: ...

@router.get("/api/docs/snapshots/{snapshot_id}")
async def get_snapshot(snapshot_id: str) -> Snapshot: ...
```

#### 6.5.5 Semantic Search Engine

```python
# c4/docs/semantic_search.py
class SemanticSearcher:
    """TF-IDF 기반 자연어 코드 검색"""

    def search(self, query: str, scope: str | None = None) -> list[SearchResult]:
        """자연어 쿼리로 코드 검색"""

    def find_related_symbols(self, symbol: str) -> list[RelatedSymbol]:
        """관련 심볼 찾기"""

    def search_by_type(self, symbol_type: str) -> list[Symbol]:
        """타입별 심볼 검색"""
```

**기능**:
- 프로그래밍 동의어 확장 (auth → authentication, db → database 등)
- 범위 지정 검색 (symbols, docs, code, files)
- TF-IDF 기반 관련도 랭킹

**MCP 도구**:
- `c4_semantic_search`: 자연어 코드 검색
- `c4_find_related_symbols`: 관련 심볼 찾기
- `c4_search_by_type`: 타입별 심볼 검색

#### 6.5.6 Call Graph Analyzer

```python
# c4/docs/call_graph.py
class CallGraphAnalyzer:
    """함수 호출 관계 분석"""

    def get_callers(self, symbol: str) -> list[Caller]:
        """호출자 찾기"""

    def get_callees(self, symbol: str) -> list[Callee]:
        """피호출자 찾기"""

    def find_call_paths(self, from_symbol: str, to_symbol: str) -> list[Path]:
        """호출 경로 찾기"""

    def get_stats(self) -> CallGraphStats:
        """호출 그래프 통계 (핫스팟, 진입점, 고립 함수)"""

    def generate_diagram(self, symbol: str, depth: int = 2) -> str:
        """Mermaid 다이어그램 생성"""
```

**MCP 도구**:
- `c4_get_callers`: 호출자 찾기
- `c4_get_callees`: 피호출자 찾기
- `c4_find_call_paths`: 호출 경로 찾기
- `c4_call_graph_stats`: 호출 그래프 통계
- `c4_call_graph_diagram`: Mermaid 다이어그램

#### 6.5.7 Long-Running Worker Detection

```python
# c4/daemon/workers.py
class WorkerManager:
    """Worker 관리 + 이상 탐지"""

    def detect_long_running(self, threshold_minutes: int = 30) -> list[WorkerInfo]:
        """장기 실행 Worker 탐지"""

    def check_heartbeat_anomaly(self) -> list[AnomalyReport]:
        """Heartbeat 이상 탐지"""

    def recover_stale_workers(self) -> int:
        """Stale Worker 복구"""
```

**기능**:
- Worker heartbeat 모니터링
- 장기 실행 태스크 자동 감지
- Stale worker 복구 메커니즘

### 파일

```
c4/
├── services/
│   └── code_analysis/
│       ├── __init__.py
│       ├── python_parser.py    # PythonParser
│       ├── typescript_parser.py # TypeScriptParser
│       ├── symbol_table.py     # SymbolTable
│       └── dependency_graph.py # DependencyGraph
├── docs/
│   ├── semantic_search.py      # SemanticSearcher (TF-IDF)
│   └── call_graph.py           # CallGraphAnalyzer
├── daemon/
│   └── workers.py              # Long-Running Worker Detection
├── mcp/
│   ├── docs_server.py          # Documentation MCP
│   ├── gap_analyzer.py         # Gap Analyzer MCP
│   └── code_tools.py           # Code Analysis MCP (12개 도구)
└── api/
    └── routes/
        └── docs.py             # Public Docs API
```

### MCP 도구 (12개 신규)

| 도구 | 설명 |
|------|------|
| `c4_semantic_search` | 자연어 코드 검색 |
| `c4_find_related_symbols` | 관련 심볼 찾기 |
| `c4_search_by_type` | 타입별 심볼 검색 |
| `c4_get_callers` | 호출자 찾기 |
| `c4_get_callees` | 피호출자 찾기 |
| `c4_find_call_paths` | 호출 경로 찾기 |
| `c4_call_graph_stats` | 호출 그래프 통계 |
| `c4_call_graph_diagram` | Mermaid 다이어그램 |
| `c4_find_definition` | 심볼 정의 찾기 |
| `c4_find_references` | 참조 찾기 |
| `c4_analyze_file` | 파일 심볼 분석 |
| `c4_get_dependencies` | 의존성 분석 |

---

## Phase 7: C4 Cloud (장기)

> **상태**: 📋 계획
> **예상 버전**: v0.7.0

### 목표

완전 관리형 SaaS

### 예상 기능

- Web Dashboard
- 원격 Worker Pool
- GitHub 통합 강화
- 사용량 기반 과금
- 팀/조직 관리

---

## 버전 히스토리

| 버전 | Phase | 주요 기능 | 날짜 |
|------|-------|-----------|------|
| v0.1.0 | 1 | Core Foundation | 2025-12 |
| v0.2.0 | 2 | Multi-Worker | 2026-01 |
| v0.3.0 | 3 | Auto Supervisor | 2026-01 |
| v0.4.0 | 4 | Agent Routing | 2026-01-10 |
| v0.5.0 | 5 | Discovery & Design | 2026-01-10 |
| v0.5.1 | 5.1 | Dogfooding Fixes | 2026-01-10 |
| v0.5.5 | 5.5 | Skill System Enhancement | 2026-01-15 |
| v0.6.0 | 6 + 6.5 | Team Collaboration + MCP Advanced Tools | 2026-01-25 |
| v0.7.0 | 7 | C4 Cloud | (계획) |

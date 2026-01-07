# C4 Cloud Architecture

## 1. Overview

C4 Cloud는 C4의 호스팅 버전으로, 웹 대시보드를 통해 AI 프로젝트를 관리하고 실행합니다.

### 1.1 핵심 차별점 (vs 로컬 버전)

| 기능 | 로컬 (v0) | Cloud |
|------|----------|-------|
| 실행 환경 | 사용자 터미널 | 클라우드 워커 |
| API 키 | 사용자 직접 관리 | C4가 관리 |
| 상태 확인 | `c4 status` CLI | 웹 대시보드 |
| 워커 수 | 수동 터미널 추가 | 슬라이더로 조절 |
| 결과물 | 로컬 파일 | GitHub 자동 push |
| 과금 | 무료 | 구독 + 사용량 |

---

## 2. System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         C4 Cloud                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐   │
│  │   Frontend   │────▶│   API Server │────▶│   Worker     │   │
│  │   (Next.js)  │     │   (FastAPI)  │     │   Manager    │   │
│  └──────────────┘     └──────────────┘     └──────────────┘   │
│         │                    │                    │            │
│         │                    ▼                    ▼            │
│         │             ┌──────────────┐     ┌──────────────┐   │
│         │             │   Database   │     │   Worker     │   │
│         │             │  (Postgres)  │     │   Pool       │   │
│         │             └──────────────┘     └──────────────┘   │
│         │                    │                    │            │
│         ▼                    ▼                    ▼            │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐   │
│  │   Auth       │     │   Queue      │     │   LLM APIs   │   │
│  │   (Clerk)    │     │   (Redis)    │     │ Claude/GPT   │   │
│  └──────────────┘     └──────────────┘     └──────────────┘   │
│                                                   │            │
│                                                   ▼            │
│                                            ┌──────────────┐   │
│                                            │   GitHub     │   │
│                                            │   Integration│   │
│                                            └──────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. Core Components

### 3.1 Frontend (Web Dashboard)

**기술 스택:** Next.js 14 + TypeScript + Tailwind + shadcn/ui

**주요 화면:**

```
/dashboard
├── /projects              # 프로젝트 목록
│   └── /[id]              # 프로젝트 상세
│       ├── /plan          # 계획 편집
│       ├── /run           # 실행 모니터링
│       ├── /logs          # 실행 로그
│       └── /settings      # 프로젝트 설정
├── /settings
│   ├── /api-keys          # API 키 관리
│   ├── /github            # GitHub 연동
│   └── /billing           # 결제/플랜
└── /team                  # 팀 관리 (Team 플랜)
```

**핵심 UI 컴포넌트:**

```tsx
// 워커 수 조절
<WorkerSlider
  min={1}
  max={plan.maxWorkers}
  value={workers}
  onChange={setWorkers}
/>

// 실시간 로그 스트림
<LogStream projectId={id} />

// 프로젝트 상태
<ProjectStatus
  state="running"
  progress={65}
  currentTask="Implementing auth module"
/>
```

### 3.2 API Server

**기술 스택:** FastAPI + SQLAlchemy + Pydantic

**주요 엔드포인트:**

```
POST   /api/projects                    # 프로젝트 생성
GET    /api/projects/{id}               # 프로젝트 조회
POST   /api/projects/{id}/run           # 실행 시작
POST   /api/projects/{id}/stop          # 실행 중지
PATCH  /api/projects/{id}/workers       # 워커 수 조절
GET    /api/projects/{id}/logs          # 로그 조회 (SSE)

POST   /api/github/connect              # GitHub 연동
POST   /api/github/repos                # 리포지토리 목록
POST   /api/projects/{id}/push          # 결과 push

GET    /api/usage                       # 사용량 조회
POST   /api/billing/subscribe           # 구독
```

### 3.3 Worker Manager

**역할:** 워커 풀 관리, 작업 분배, 스케일링

```python
class WorkerManager:
    async def scale_workers(self, project_id: str, count: int):
        """워커 수 동적 조절"""
        current = await self.get_worker_count(project_id)

        if count > current:
            # 워커 추가
            for _ in range(count - current):
                await self.spawn_worker(project_id)
        else:
            # 워커 축소 (graceful)
            await self.reduce_workers(project_id, current - count)

    async def spawn_worker(self, project_id: str):
        """새 워커 생성"""
        worker = Worker(
            project_id=project_id,
            llm_client=self.get_llm_client(),
        )
        await self.queue.add(worker)
```

### 3.4 Worker

**역할:** 실제 task 실행 (로컬 버전의 Worker와 동일 로직)

```python
class CloudWorker:
    def __init__(self, project_id: str, llm_client: LLMClient):
        self.project_id = project_id
        self.llm = llm_client
        self.workspace = self.setup_workspace()

    async def run_task(self, task: Task):
        """task 실행 (Ralph Loop)"""
        while not task.is_complete:
            # 1. LLM에게 작업 요청
            response = await self.llm.complete(task.prompt)

            # 2. 코드 실행/테스트
            result = await self.execute(response)

            # 3. 결과에 따라 반복 또는 완료
            if result.success:
                task.complete()
            else:
                task.add_feedback(result.error)
```

### 3.5 LLM Router

**역할:** 여러 LLM 프로바이더 통합 관리

```python
class LLMRouter:
    providers = {
        "claude": AnthropicClient,
        "gpt": OpenAIClient,
        "codex": OpenAIClient,  # code-specific
    }

    async def complete(self,
                       prompt: str,
                       model: str = "claude-sonnet",
                       task_type: str = "general"):
        """모델 라우팅"""

        # task_type에 따른 최적 모델 선택
        if task_type == "planning":
            model = "claude-opus"
        elif task_type == "coding":
            model = "claude-sonnet"  # or "codex"
        elif task_type == "review":
            model = "claude-opus"

        provider = self.get_provider(model)
        return await provider.complete(prompt, model)
```

---

## 4. GitHub Integration

### 4.1 연동 플로우

```
1. 사용자가 GitHub 연동 버튼 클릭
2. GitHub OAuth 인증
3. 리포지토리 선택 (또는 새로 생성)
4. C4가 해당 리포 write 권한 획득
5. 프로젝트 완료 시 자동 push
```

### 4.2 Push 전략

```python
class GitHubIntegration:
    async def push_results(self, project: Project):
        """프로젝트 결과를 GitHub에 push"""

        repo = await self.get_repo(project.github_repo)

        # 1. 브랜치 생성
        branch = f"c4/{project.id}/{datetime.now():%Y%m%d}"
        await repo.create_branch(branch)

        # 2. 변경사항 커밋
        await repo.commit(
            branch=branch,
            files=project.output_files,
            message=f"[C4] {project.name} - {project.summary}"
        )

        # 3. PR 생성 (옵션)
        if project.settings.auto_pr:
            await repo.create_pr(
                title=f"[C4] {project.name}",
                body=self.generate_pr_body(project),
                head=branch,
                base="main"
            )
```

### 4.3 PR 템플릿

```markdown
## C4 Project Completion

**Project:** {project.name}
**Duration:** {project.duration}
**Tasks Completed:** {project.task_count}

### Summary
{project.summary}

### Changes
{file_change_list}

### Test Results
- Passed: {tests.passed}
- Failed: {tests.failed}

---
Generated by [C4 Cloud](https://c4.dev)
```

---

## 5. Data Models

### 5.1 Core Models

```python
class User(BaseModel):
    id: UUID
    email: str
    github_id: Optional[str]
    plan: Plan  # free, pro, team, enterprise
    created_at: datetime

class Project(BaseModel):
    id: UUID
    user_id: UUID
    name: str
    description: str
    state: ProjectState  # planning, running, paused, completed

    # GitHub 연동
    github_repo: Optional[str]
    github_branch: Optional[str]
    auto_push: bool = True
    auto_pr: bool = False

    # 설정
    worker_count: int = 1
    model_config: ModelConfig

    # 문서
    plan_md: str
    checkpoints_md: str
    done_md: str

class Task(BaseModel):
    id: UUID
    project_id: UUID
    name: str
    scope: str
    state: TaskState  # pending, in_progress, completed, failed
    worker_id: Optional[UUID]
    attempts: int = 0

class Worker(BaseModel):
    id: UUID
    project_id: UUID
    state: WorkerState  # idle, working, stopped
    current_task_id: Optional[UUID]

class Usage(BaseModel):
    user_id: UUID
    period: str  # "2025-01"
    api_calls: int
    tokens_used: int
    cost_usd: Decimal
```

### 5.2 Database Schema

```sql
-- 핵심 테이블
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email VARCHAR UNIQUE NOT NULL,
    github_id VARCHAR,
    plan VARCHAR DEFAULT 'free',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    name VARCHAR NOT NULL,
    state VARCHAR DEFAULT 'planning',
    github_repo VARCHAR,
    worker_count INT DEFAULT 1,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    project_id UUID REFERENCES projects(id),
    name VARCHAR NOT NULL,
    state VARCHAR DEFAULT 'pending',
    worker_id UUID,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE usage (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    period VARCHAR NOT NULL,  -- '2025-01'
    tokens_used BIGINT DEFAULT 0,
    cost_usd DECIMAL(10,4) DEFAULT 0
);
```

---

## 6. Infrastructure

### 6.1 Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | Next.js, Vercel |
| API | FastAPI, Railway/Fly.io |
| Database | PostgreSQL (Supabase/Neon) |
| Queue | Redis (Upstash) |
| Workers | Fly.io Machines / Modal |
| Auth | Clerk |
| Payments | Stripe |
| Monitoring | Sentry, Axiom |

### 6.2 Deployment Architecture

```
                    ┌─────────────┐
                    │   Vercel    │
                    │  (Frontend) │
                    └──────┬──────┘
                           │
                           ▼
┌──────────────────────────────────────────────────┐
│                   Fly.io                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐      │
│  │ API (3x) │  │ Worker   │  │ Worker   │ ...  │
│  └──────────┘  │ Machine  │  │ Machine  │      │
│                └──────────┘  └──────────┘      │
└──────────────────────────────────────────────────┘
        │                │
        ▼                ▼
┌──────────────┐  ┌──────────────┐
│   Supabase   │  │   Upstash    │
│  (Postgres)  │  │   (Redis)    │
└──────────────┘  └──────────────┘
```

### 6.3 Worker Scaling Strategy

```python
# Fly.io Machines API로 동적 스케일링
class FlyWorkerPool:
    async def scale(self, project_id: str, count: int):
        machines = await fly.machines.list(
            app="c4-workers",
            metadata={"project_id": project_id}
        )

        current = len(machines)

        if count > current:
            # 머신 추가
            for _ in range(count - current):
                await fly.machines.create(
                    app="c4-workers",
                    config={
                        "image": "c4/worker:latest",
                        "env": {"PROJECT_ID": project_id},
                        "auto_destroy": True,  # 유휴 시 자동 종료
                    }
                )
        elif count < current:
            # 머신 축소
            to_stop = machines[count:]
            for m in to_stop:
                await fly.machines.stop(m.id)
```

---

## 7. Security

### 7.1 API Key Management

```python
class APIKeyManager:
    """사용자 대신 API 키 관리"""

    def __init__(self):
        # C4의 마스터 API 키 (환경변수)
        self.anthropic_key = os.environ["ANTHROPIC_API_KEY"]
        self.openai_key = os.environ["OPENAI_API_KEY"]

    async def get_client(self, provider: str, user_id: str):
        """사용자별 rate limit 적용된 클라이언트"""
        user = await get_user(user_id)

        return LLMClient(
            api_key=self._get_key(provider),
            rate_limit=user.plan.rate_limit,
            budget=user.remaining_budget,
        )
```

### 7.2 Workspace Isolation

```python
class SecureWorkspace:
    """프로젝트별 격리된 실행 환경"""

    async def create(self, project_id: str):
        # 각 프로젝트는 별도 컨테이너/VM에서 실행
        return await create_sandbox(
            project_id=project_id,
            network="isolated",  # 외부 네트워크 차단 (GitHub만 허용)
            filesystem="ephemeral",  # 실행 후 삭제
        )
```

---

## 8. Pricing Tiers

```yaml
Free:
  price: $0
  credits: $10/월 (체험용)
  workers: 1
  projects: 3
  models: [claude-haiku, gpt-3.5]
  github: 공개 리포만
  support: Community

Pro:
  price: $30/월
  credits: $50/월 포함
  workers: 5
  projects: 무제한
  models: 모든 모델
  github: 공개 + 비공개
  support: Email

Team:
  price: $100/월
  credits: $200/월 포함
  workers: 20
  projects: 무제한
  models: 모든 모델
  github: 모두 + Organization
  features:
    - 팀 멤버 관리
    - 감사 로그
    - 프로젝트 공유
  support: Priority

Enterprise:
  price: 문의
  features:
    - 전용 인프라
    - SSO/SAML
    - SLA 보장
    - 온프레미스 옵션
    - 전담 지원
```

---

## 9. Roadmap

### Phase 1: MVP (v2.0)
- [ ] 기본 웹 대시보드
- [ ] 단일 워커 실행
- [ ] GitHub 연동 (push)
- [ ] Stripe 결제

### Phase 2: Scale (v2.5)
- [ ] 멀티 워커 스케일링
- [ ] 실시간 로그 스트림
- [ ] GitHub PR 자동 생성
- [ ] 팀 기능

### Phase 3: Enterprise (v3.0)
- [ ] SSO/SAML
- [ ] 감사 로그
- [ ] 온프레미스
- [ ] API 공개

---

## 10. Open Questions

- [ ] Worker 실행 환경: Fly Machines vs Modal vs Lambda?
- [ ] 코드 실행 보안: gVisor? Firecracker?
- [ ] GitHub 외 GitLab/Bitbucket 지원 시점?
- [ ] 무료 tier 남용 방지 전략?

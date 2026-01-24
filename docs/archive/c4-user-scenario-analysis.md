# C4 사용자 시나리오 분석: 구독형 종합 개발팀

> C4를 "AI 개발팀 구독 서비스"로 포지셔닝할 때, 다양한 사용자 유형별 시나리오 분석

---

## 1. Executive Summary

### C4 핵심 가치 제안

```
"개발자 1명이 5명 팀의 생산성을 갖는다"

C4 = AI Worker Pool + State Machine + Quality Gates
   = 자동화된 개발 파이프라인
   = 구독형 종합 개발팀
```

### 타겟 사용자 세그먼트

| 세그먼트 | 규모 | 핵심 Pain Point | C4 가치 |
|---------|------|----------------|---------|
| **Solo Developer** | 1명 | 시간 부족, 멀티태스킹 | 24시간 일하는 팀원 |
| **Startup** | 2-5명 | 리소스 제한, 속도 압박 | 개발 속도 3-5x |
| **Agency** | 5-20명 | 다중 프로젝트, 품질 일관성 | 표준화된 워크플로우 |
| **Enterprise** | 20+명 | 컴플라이언스, 추적성 | 감사 가능한 자동화 |
| **OSS Maintainer** | 1-3명 | PR 리뷰 부담, 컨트리뷰터 온보딩 | 자동 리뷰 + 가이드 |

---

## 2. 사용자 시나리오 상세 분석

### 2.1 Solo Developer (1인 개발자)

#### 페르소나: 김민수 (프리랜서, 32세)
- **상황**: SaaS 제품 MVP 개발 중, 3개월 deadline
- **현재 Pain**: 혼자서 프론트엔드/백엔드/인프라 모두 담당
- **예산**: 월 $200-500

#### 사용 시나리오

```
[Day 1: 프로젝트 시작]
김민수: /c4-init "SaaS 대시보드"
C4: Discovery 인터뷰 시작
     → 요구사항 EARS 패턴으로 정리
     → 5개 핵심 기능 도출

김민수: /c4-plan
C4: 20개 태스크 생성 (T-001 ~ T-020)
     → 의존성 그래프 자동 구성
     → 예상 완료: 15일 (Worker 1개 기준)

[Day 2-15: 자동 실행]
김민수: /c4-run
C4: (자동 루프)
     T-001: API 스켈레톤 → 완료
     T-002: DB 스키마 → 완료
     ...
     [CHECKPOINT] 핵심 기능 리뷰
김민수: /c4-checkpoint → APPROVE

[결과]
- 2주 만에 MVP 완성
- 김민수는 비즈니스 로직만 검토
- 테스트 커버리지 80%+ 자동 달성
```

#### C4 설정 (Solo 최적화)

```yaml
# .c4/config.yaml
workers:
  max_concurrent: 1  # 비용 최적화

checkpoints:
  - id: "CP-MVP"
    auto_approve: false  # 핵심 결정은 직접

review_as_task: true
max_revision: 2  # 빠른 이터레이션

llm:
  model: "claude-cli"  # 로컬 실행
```

#### 가격 민감도 분석

| 티어 | 월 비용 | Worker 수 | 적합성 |
|------|---------|----------|-------|
| **Hobby** | $0 (BYOK) | 1 | Solo 최적 |
| **Pro** | $49 | 2 | 병렬 작업 시 |
| **Team** | $199 | 5 | 과잉 |

---

### 2.2 Startup Team (스타트업, 2-5명)

#### 페르소나: 팀 "런치패드" (공동창업자 3명)
- **상황**: 시드 라운드 준비, 3개월 내 제품 출시 필요
- **현재 Pain**: CTO 1명이 모든 기술 결정, 개발자 2명은 주니어
- **예산**: 월 $500-2000

#### 사용 시나리오

```
[Week 1: 아키텍처 설계]
CTO: /c4-init "Fintech 앱"
C4: Discovery 인터뷰
     → "결제 처리", "KYC 인증", "대시보드" 3개 feature 도출

CTO: /c4-plan
C4: Design phase
     → 3가지 아키텍처 옵션 제시
     → CTO가 "마이크로서비스" 선택
     → 45개 태스크 생성

[Week 2-8: 병렬 개발]
             Worker-A (Backend)    Worker-B (Frontend)
Day 1-3      T-001~005: API         T-020~025: UI 컴포넌트
Day 4-7      T-006~010: 결제 연동   T-026~030: 결제 화면
Day 8-10     T-011~015: KYC         T-031~035: KYC 화면
             ...                     ...

[Week 9: 통합 테스트]
C4: [CHECKPOINT] 전체 통합 리뷰
     → E2E 테스트 자동 실행
     → 3개 이슈 발견 → REQUEST_CHANGES
     → Repair task 자동 생성 → 2일 내 해결

[결과]
- 9주 만에 MVP 출시 (기존 예상: 16주)
- 개발자 2명은 C4가 생성한 코드 리뷰에 집중
- CTO는 아키텍처 결정에만 시간 투자
```

#### C4 설정 (Startup 최적화)

```yaml
# 멀티 워커 설정
workers:
  max_concurrent: 3

# 도메인별 라우팅
agents:
  chains:
    web-frontend:
      primary: "frontend-developer"
      chain: ["frontend-developer", "test-automator"]
    web-backend:
      primary: "backend-architect"
      chain: ["backend-architect", "security-auditor"]

# 빠른 이터레이션
checkpoints:
  - id: "CP-DAILY"
    auto_approve: true  # 일일 체크는 자동
  - id: "CP-SPRINT"
    auto_approve: false  # 스프린트 리뷰는 수동

# GitHub 자동화
github:
  auto_pr: true
  reviewers: ["cto@company.com"]
```

#### ROI 계산

```
기존:
- 개발자 3명 × $8,000/월 = $24,000/월
- 16주 개발 = 4개월 = $96,000

C4 사용:
- 개발자 2명 × $8,000/월 = $16,000/월
- C4 Team 티어 = $500/월
- 9주 개발 = 2.25개월 = $37,125

절감: $58,875 (61% 절감)
```

---

### 2.3 Agency / Consulting (에이전시)

#### 페르소나: "디지털웨이브" (웹 에이전시, 15명)
- **상황**: 동시에 5-8개 클라이언트 프로젝트 진행
- **현재 Pain**: 프로젝트마다 품질 편차, 인수인계 문제
- **예산**: 월 $2000-10000

#### 사용 시나리오

```
[프로젝트 A: 이커머스]     [프로젝트 B: 의료 포털]
PM-A: /c4-init            PM-B: /c4-init
       ↓                         ↓
C4 Instance A              C4 Instance B
- Worker 2개               - Worker 3개
- 40 태스크               - 60 태스크
- 6주 예상                - 8주 예상

[중앙 대시보드]
┌─────────────────────────────────────────┐
│ Agency Dashboard (Multi-Project View)   │
├─────────┬─────────┬─────────┬──────────┤
│ Proj A  │ Proj B  │ Proj C  │ Proj D   │
│ 75%     │ 45%     │ 90%     │ 20%      │
│ ON TIME │ DELAYED │ COMPLETE│ STARTING │
└─────────┴─────────┴─────────┴──────────┘

[품질 일관성]
- 모든 프로젝트: 동일한 린팅 규칙
- 모든 프로젝트: 80%+ 테스트 커버리지
- 모든 프로젝트: 동일한 코드 스타일
```

#### C4 설정 (Agency 최적화)

```yaml
# 멀티 프로젝트 지원
store:
  backend: "supabase"
  team_id: "agency-digitalwave"

# 프로젝트 템플릿
templates:
  - name: "ecommerce"
    checkpoints: [...]
    validations: [...]
  - name: "healthcare"
    checkpoints: [...]  # HIPAA 컴플라이언스 포함

# 중앙 집중 모니터링
monitoring:
  dashboard_url: "https://dashboard.c4.dev"
  alerts:
    - type: "checkpoint_pending"
      notify: "pm@agency.com"
    - type: "validation_failed"
      notify: "tech-lead@agency.com"
```

#### 에이전시 전용 기능 필요

| 기능 | 현재 상태 | 필요 이유 |
|------|----------|----------|
| **Multi-Project Dashboard** | 🔴 없음 | 전체 프로젝트 현황 파악 |
| **Template Library** | 🟡 부분 | 프로젝트 유형별 빠른 시작 |
| **Client Portal** | 🔴 없음 | 클라이언트에게 진행상황 공유 |
| **Time Tracking** | 🔴 없음 | 빌링 근거 |
| **White-label** | 🔴 없음 | 에이전시 브랜딩 |

---

### 2.4 Enterprise (대기업)

#### 페르소나: "글로벌뱅크" IT 부서 (개발자 200명)
- **상황**: 레거시 시스템 현대화 프로젝트
- **현재 Pain**: 컴플라이언스, 감사 추적, 보안
- **예산**: 월 $10,000+

#### 사용 시나리오

```
[보안 및 컴플라이언스]
┌─────────────────────────────────────────────┐
│ C4 Enterprise: On-Premise Deployment        │
│                                             │
│ ┌─────────────┐  ┌─────────────────────┐   │
│ │ C4 Server   │  │ Private LLM         │   │
│ │ (Air-gapped)│  │ (Azure OpenAI)      │   │
│ └─────────────┘  └─────────────────────┘   │
│                                             │
│ ┌─────────────────────────────────────────┐ │
│ │ Audit Log: Every state transition       │ │
│ │            Every code change            │ │
│ │            Every decision               │ │
│ └─────────────────────────────────────────┘ │
└─────────────────────────────────────────────┘

[감사 추적]
Event: T-001 completed
  - Worker: worker-prod-12
  - Commit: abc123
  - Validation: lint=pass, unit=pass, security=pass
  - Approver: john.doe@globalbank.com
  - Timestamp: 2025-01-21T10:30:00Z
  - Signature: SHA256:xyz...
```

#### C4 설정 (Enterprise 최적화)

```yaml
# On-Premise 배포
deployment:
  mode: "on-premise"
  air_gapped: true

# 프라이빗 LLM
llm:
  provider: "azure-openai"
  endpoint: "https://globalbank.openai.azure.com"
  model: "gpt-4"

# 감사 로깅
audit:
  enabled: true
  storage: "s3://globalbank-audit-logs"
  retention_days: 2555  # 7년

# RBAC
access_control:
  roles:
    - name: "developer"
      permissions: ["read", "submit"]
    - name: "lead"
      permissions: ["read", "submit", "checkpoint"]
    - name: "admin"
      permissions: ["*"]

# 보안 스캔
validation:
  commands:
    security: "snyk test"
    sast: "semgrep --config=p/owasp-top-ten"
  required: ["lint", "unit", "security", "sast"]
```

#### Enterprise 전용 기능 필요

| 기능 | 현재 상태 | 필요 이유 |
|------|----------|----------|
| **SSO/SAML** | 🔴 없음 | 기업 인증 통합 |
| **RBAC** | 🟡 기본 | 세분화된 권한 관리 |
| **Audit Log Export** | 🟡 기본 | 컴플라이언스 보고 |
| **On-Premise Deploy** | 🟡 가능 | 데이터 주권 |
| **SLA Guarantee** | 🔴 없음 | 미션 크리티컬 |
| **Private LLM** | 🟢 지원 | 데이터 유출 방지 |

---

### 2.5 OSS Maintainer (오픈소스 메인테이너)

#### 페르소나: 박지훈 (인기 라이브러리 메인테이너)
- **상황**: GitHub Stars 10k+, 주간 PR 50+
- **현재 Pain**: PR 리뷰 부담, 컨트리뷰터 가이드 반복
- **예산**: 무료 (스폰서 수익으로 $100-500/월 가능)

#### 사용 시나리오

```
[PR 자동 리뷰]
Contributor: PR #1234 "Add feature X"
              ↓
C4 Bot (GitHub Action):
  1. 자동 린트/테스트 실행
  2. 코드 리뷰 코멘트 생성
  3. 가이드라인 준수 체크
  4. 메인테이너에게 요약 제공

박지훈: (C4 리뷰 보고 최종 결정만)
         Merge / Request Changes

[컨트리뷰터 온보딩]
New Contributor: "How do I set up dev env?"
C4 Bot: (자동 응답)
        "Check CONTRIBUTING.md: ..."
        "Run: npm install && npm test"
```

#### C4 설정 (OSS 최적화)

```yaml
# GitHub App 모드
github:
  app_mode: true
  auto_review: true
  auto_label: true

# 무료 티어 최적화
llm:
  model: "claude-haiku"  # 저비용

# PR 리뷰 특화
review:
  style: "constructive"
  check_guidelines: true
  guidelines_file: "CONTRIBUTING.md"

# 자동 응답
auto_response:
  enabled: true
  templates:
    - trigger: "setup"
      response: "See CONTRIBUTING.md#setup"
```

---

## 3. 기능 격차 분석 (Gap Analysis)

### 현재 C4 vs 세그먼트별 요구사항

| 기능 | Solo | Startup | Agency | Enterprise | OSS | 현재 상태 |
|------|:----:|:-------:|:------:|:----------:|:---:|:--------:|
| **Core Execution** | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ 완료 |
| **Multi-Worker** | ➖ | ✅ | ✅ | ✅ | ➖ | ✅ 완료 |
| **Web Dashboard** | ✅ | ✅ | 🟡 | 🟡 | ➖ | 🟡 기본 |
| **GitHub App** | ➖ | 🔴 | 🔴 | 🔴 | 🔴 | 🔴 없음 |
| **Discord Bot** | ➖ | 🟡 | 🔴 | 🟡 | ➖ | 🔴 없음 |
| **Slack Bot** | ➖ | ➖ | 🟡 | 🔴 | ➖ | 🔴 없음 |
| **Multi-Project** | ➖ | 🟡 | 🔴 | 🔴 | ➖ | 🔴 없음 |
| **SSO/RBAC** | ➖ | ➖ | 🟡 | 🔴 | ➖ | 🔴 없음 |
| **Audit Log** | ➖ | ➖ | 🟡 | 🔴 | ➖ | 🟡 기본 |
| **Template Library** | ✅ | ✅ | 🔴 | 🟡 | ➖ | 🟡 부분 |
| **Private LLM** | ➖ | ➖ | 🟡 | ✅ | ➖ | ✅ 지원 |
| **Client Portal** | ➖ | ➖ | 🔴 | ➖ | ➖ | 🔴 없음 |

**범례**: ✅ 충분/완료 | 🟡 개선 필요/부분 | 🔴 없음/필수 | ➖ 불필요

---

## 4. 플랫폼 통합 아키텍처

### C4 Server 중심 통합

```
                     ☁️ C4 Cloud Server
                    ┌─────────────────────────────────────┐
                    │                                     │
                    │  ┌─────────────┐  ┌─────────────┐  │
                    │  │ Webhook API │  │ C4 Core     │  │
                    │  │ (수신)      │  │ (상태관리)   │  │
                    │  └──────┬──────┘  └─────────────┘  │
                    │         │                          │
                    │  ┌──────┴──────┐                   │
                    │  │ LLM API     │                   │
                    │  │ (Claude)    │                   │
                    │  └─────────────┘                   │
                    └─────────────────────────────────────┘
                              ▲
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
   ┌──────────┐        ┌──────────┐        ┌──────────┐
   │ GitHub   │        │ Discord  │        │ Slack    │
   │ App      │        │ Bot      │        │ Bot      │
   └──────────┘        └──────────┘        └──────────┘
```

### 4.1 GitHub App 상세

**기능:**
| 기능 | 설명 |
|------|------|
| **PR 자동 리뷰** | 코드 변경 분석, 보안/품질 검사, 리뷰 코멘트 생성 |
| **자동 라벨링** | PR 내용 분석 → 적절한 라벨 부여 |
| **C4 태스크 연동** | 태스크 ↔ 브랜치/PR 양방향 동기화 |
| **컨트리뷰터 가이드** | 첫 기여자 자동 안내, FAQ 응답 |
| **CI 상태 보고** | 검증 결과를 GitHub Status로 표시 |

**사용자 상호작용:**
```markdown
# PR 코멘트 명령어
@c4-bot review          # 리뷰 요청
@c4-bot explain :45-60  # 코드 설명
@c4-bot suggest tests   # 테스트 제안
@c4-bot why flagged?    # 경고 이유 질문
```

**설치 후 효과:**
| 사용자 | 효과 |
|--------|------|
| 관리자 (C4 CLI 있음) | CLI + 자동 리뷰 모두 사용 |
| 팀원 (C4 CLI 있음) | CLI + 자동 리뷰 모두 사용 |
| 팀원 (C4 CLI 없음) | PR 코멘트로만 상호작용, 자동 리뷰 수혜 |
| 외부 기여자 (OSS) | PR 코멘트로 안내, 자동 리뷰 수혜 |

### 4.2 Discord Bot 상세

**기능:**
| 기능 | 설명 |
|------|------|
| **알림** | 태스크 완료, 체크포인트, 오류 알림 |
| **대화형 리뷰** | "왜 이렇게 리뷰했어?" 같은 후속 질문 |
| **상태 조회** | `/c4 status` 명령으로 프로젝트 현황 |
| **빠른 승인** | 체크포인트 APPROVE/REJECT 버튼 |

**채널 구조:**
```
#c4-notifications
  - 🟢 T-001 완료: 인증 API 구현
  - 🟡 CHECKPOINT: Phase 1 리뷰 대기
  - 🔴 T-005 실패: 테스트 3개 실패

#c4-reviews
  - PR #123 리뷰 완료 (2개 제안)
  - [자세히 보기] [승인] [변경요청]

#c4-chat
  - 자유로운 C4 대화
  - "왜 이 코드가 문제야?"
  - "더 나은 방법 추천해줘"
```

**Slash 명령어:**
```
/c4 status              # 프로젝트 상태
/c4 tasks               # 태스크 목록
/c4 review PR#123       # 특정 PR 리뷰 요청
/c4 approve CP-001      # 체크포인트 승인
/c4 explain <코드>      # 코드 설명
```

### 4.3 통합 시나리오

```
[시나리오: 팀원이 PR 올림]

1. 팀원: git push → PR 생성
         ↓
2. GitHub App: PR 감지 → C4 서버로 Webhook
         ↓
3. C4 Server: 코드 분석 → 리뷰 생성
         ↓
4. GitHub: PR에 리뷰 코멘트 추가
         ↓
5. Discord: #c4-reviews에 알림
   "PR #123 리뷰 완료 - 2개 제안, 1개 경고"
         ↓
6. 관리자: Discord에서 확인 또는 GitHub에서 확인
         ↓
7. 팀원: GitHub에서 "@c4-bot explain" 질문
         ↓
8. C4: 상세 설명 코멘트 추가
```

---

---

## 5. GitHub App 구현 계획 (Phase 1 - 최우선)

### 5.1 아키텍처

```
┌─────────────────┐     Webhook      ┌──────────────────────────────┐
│   GitHub        │ ───────────────→ │  C4 FastAPI Server           │
│   PR Events     │                  │  /webhooks/github            │
└─────────────────┘                  └────────────┬─────────────────┘
                                                  │
                                                  ▼
                                     ┌──────────────────────────────┐
                                     │  PR Review Service           │
                                     │  1. Diff 조회                │
                                     │  2. Claude API 분석          │
                                     │  3. 리뷰 코멘트 작성         │
                                     └──────────────────────────────┘
```

### 5.2 파일 구조

```
c4/
├── config/
│   └── github_app.py           # 신규: GitHub App 설정
├── integrations/
│   ├── github.py               # 기존: GitHubClient (재사용)
│   └── github_app.py           # 신규: GitHub App 인증 + API
├── api/
│   └── routes/
│       └── webhooks.py         # 신규: Webhook 엔드포인트
├── services/
│   └── pr_review.py            # 신규: PR 리뷰 서비스
```

### 5.3 핵심 컴포넌트

#### A. GitHub App 설정 (`c4/config/github_app.py`)

```python
@dataclass
class GitHubAppConfig:
    app_id: str
    private_key_path: Path
    webhook_secret: str

    @classmethod
    def from_env(cls) -> "GitHubAppConfig":
        # GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY_PATH, GITHUB_WEBHOOK_SECRET
```

#### B. Webhook 엔드포인트 (`c4/api/routes/webhooks.py`)

```python
@router.post("/github")
async def handle_github_webhook(
    request: Request,
    background_tasks: BackgroundTasks,
    x_github_event: str = Header(...),
    x_hub_signature_256: str = Header(...),
):
    # 1. Signature 검증 (HMAC SHA-256)
    # 2. PR 이벤트 필터링 (opened, synchronize, reopened)
    # 3. Background task로 리뷰 실행
```

#### C. PR 리뷰 서비스 (`c4/services/pr_review.py`)

```python
class PRReviewService:
    async def review_pr(self, pr_info: dict) -> ReviewResult:
        # 1. GitHub API로 diff 조회
        # 2. Claude API로 리뷰 생성
        # 3. GitHub에 코멘트 작성
```

### 5.4 보안 요구사항 (CRITICAL)

| 항목 | 설명 |
|------|------|
| **Webhook Secret** | HMAC SHA-256 서명 검증 필수 |
| **Private Key** | 환경변수로 관리, git 커밋 금지 |
| **Rate Limit** | GitHub API 5000 req/hour |
| **Input Validation** | diff 크기 제한 (50KB) |

### 5.5 GitHub App 권한

| Permission | Level | 용도 |
|------------|-------|------|
| `contents` | read | PR diff 조회 |
| `pull_requests` | read & write | 리뷰 코멘트 작성 |
| `metadata` | read | 기본 |

**Subscribe Events:** `pull_request`

### 5.6 환경 변수

```bash
# GitHub App
GITHUB_APP_ID=123456
GITHUB_APP_PRIVATE_KEY_PATH=/path/to/private-key.pem
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# Claude API (기존)
ANTHROPIC_API_KEY=sk-ant-...
```

### 5.7 구현 순서

| 순서 | 파일 | 작업 |
|------|------|------|
| 1 | `c4/config/github_app.py` | 설정 모델 |
| 2 | `c4/integrations/github_app.py` | App 인증 + API |
| 3 | `c4/api/routes/webhooks.py` | Webhook 핸들러 |
| 4 | `c4/services/pr_review.py` | 리뷰 서비스 |
| 5 | `c4/api/server.py` | 라우터 등록 |
| 6 | `tests/` | 테스트 코드 |

### 5.8 검증 방법

```bash
# 1. Unit Test
uv run pytest tests/unit/test_webhook_signature.py
uv run pytest tests/unit/test_pr_review.py

# 2. Integration Test
uv run pytest tests/integration/test_github_app.py

# 3. E2E Test
# - GitHub App 설치
# - 테스트 레포에 PR 생성
# - 리뷰 코멘트 확인
```

---

## 6. 우선순위 로드맵 (업데이트)

### Phase 1: GitHub App (최우선, 1-2주)

```
1. GitHub App 구현 (위 5절 참조)
   - Webhook 엔드포인트
   - PR 리뷰 서비스
   - Claude API 연동
```

### Phase 2: Discord Bot (2-3주)

```
3. Template Library (Agency + Solo)
   - 프로젝트 유형별 템플릿
   - 커스텀 템플릿 저장

4. Enhanced RBAC (Enterprise)
   - 역할 기반 권한
   - 팀 관리

5. Audit Log Export (Enterprise)
   - 감사 로그 다운로드
   - 컴플라이언스 보고서
```

### Phase 3: 차별화 (5-6개월)

```
6. Client Portal (Agency)
   - 클라이언트 전용 뷰
   - 진행 상황 공유
   - White-label 옵션

7. Time Tracking (Agency)
   - 태스크별 시간 추적
   - 빌링 리포트

8. SSO/SAML (Enterprise)
   - Okta, Azure AD 연동
```

---

## 5. 가격 전략 제안

### 티어별 구성

| 티어 | 월 가격 | 타겟 | 주요 기능 |
|------|--------|------|----------|
| **Free** | $0 | Solo/OSS | 1 Worker, BYOK, 기본 기능 |
| **Pro** | $49 | Solo+ | 2 Worker, 우선 지원 |
| **Team** | $199 | Startup | 5 Worker, GitHub 통합, 대시보드 |
| **Agency** | $499 | Agency | 10 Worker, Multi-Project, 템플릿 |
| **Enterprise** | Custom | Enterprise | 무제한, SSO, 감사, SLA |

### 추가 옵션

```
- 추가 Worker: $20/worker/월
- Private LLM 연동: $100/월
- White-label: $200/월
- 전용 지원: $500/월
```

---

## 7. 확장 가능한 통합 아키텍처 설계 (Phase 2 - 핵심)

> GitHub App, Discord, Dooray 등 다양한 외부 서비스를 유연하게 지원하는 멀티테넌트 통합 시스템

### 7.1 현재 상태 분석

**완료된 것:**
- ✅ GitHub App 기본 구현 (서버 레벨 환경변수)
- ✅ Supabase RLS 기반 workspace 격리
- ✅ Stripe 과금 시스템 (Free/Pro/Team/Enterprise)
- ✅ JWT/API Key 인증

**필요한 것:**
- 🔴 사용자/팀별 통합 설정 저장
- 🔴 OAuth 콜백 → 사용자 계정 연동
- 🔴 통합 유형별 추상화 레이어
- 🔴 Webhook 라우팅 (installation_id → workspace)

---

### 7.2 데이터베이스 스키마

```sql
-- 1. 통합 프로바이더 정의 (시스템 테이블)
CREATE TABLE integration_providers (
    id TEXT PRIMARY KEY,  -- 'github', 'discord', 'dooray', 'slack'
    name TEXT NOT NULL,
    category TEXT NOT NULL,  -- 'source_control', 'messaging', 'collaboration'
    oauth_url TEXT,
    webhook_path TEXT,
    icon_url TEXT,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. 워크스페이스별 통합 연결 (핵심 테이블)
CREATE TABLE workspace_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL REFERENCES integration_providers(id),

    -- 프로바이더별 식별자
    external_id TEXT NOT NULL,  -- GitHub: installation_id, Discord: guild_id, Dooray: project_id
    external_name TEXT,         -- repo명, 서버명, 프로젝트명

    -- 인증 정보 (암호화 저장)
    credentials JSONB,  -- access_token, refresh_token, webhook_secret 등

    -- 설정
    settings JSONB DEFAULT '{}',  -- 알림 채널, 자동 리뷰 on/off 등

    -- 상태
    status TEXT DEFAULT 'active',  -- active, suspended, revoked
    connected_by UUID REFERENCES users(id),
    connected_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,

    UNIQUE(workspace_id, provider_id, external_id)
);

-- 3. 인덱스 (Webhook 라우팅용)
CREATE INDEX idx_integrations_provider_external
ON workspace_integrations(provider_id, external_id);

CREATE INDEX idx_integrations_workspace
ON workspace_integrations(workspace_id, status);

-- 4. RLS 정책
ALTER TABLE workspace_integrations ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Users can view own workspace integrations"
ON workspace_integrations FOR SELECT
USING (workspace_id IN (
    SELECT workspace_id FROM workspace_members WHERE user_id = auth.uid()
));

CREATE POLICY "Admins can manage workspace integrations"
ON workspace_integrations FOR ALL
USING (workspace_id IN (
    SELECT workspace_id FROM workspace_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));
```

---

### 7.3 통합 추상화 레이어

```
c4/integrations/
├── base.py                 # 추상 베이스 클래스
├── registry.py             # 프로바이더 레지스트리
├── github/
│   ├── __init__.py
│   ├── client.py           # GitHub API 클라이언트 (기존)
│   ├── app.py              # GitHub App 인증 (기존)
│   ├── oauth.py            # OAuth 플로우
│   └── webhooks.py         # Webhook 핸들러
├── discord/
│   ├── __init__.py
│   ├── client.py           # Discord API 클라이언트
│   ├── oauth.py            # OAuth 플로우
│   └── bot.py              # Bot 명령 핸들러
├── dooray/
│   ├── __init__.py
│   ├── client.py           # Dooray API 클라이언트
│   ├── oauth.py            # OAuth 플로우
│   └── webhooks.py         # Webhook 핸들러
└── slack/
    └── ...                 # 미래 확장
```

#### A. 베이스 인터페이스 (`c4/integrations/base.py`)

```python
from abc import ABC, abstractmethod
from enum import Enum
from pydantic import BaseModel

class IntegrationCategory(str, Enum):
    SOURCE_CONTROL = "source_control"  # GitHub, GitLab
    MESSAGING = "messaging"            # Discord, Slack
    COLLABORATION = "collaboration"    # Dooray, Notion
    CI_CD = "ci_cd"                   # Jenkins, CircleCI

class IntegrationCapability(str, Enum):
    PR_REVIEW = "pr_review"
    NOTIFICATIONS = "notifications"
    COMMANDS = "commands"
    WEBHOOKS = "webhooks"
    OAUTH = "oauth"

class IntegrationProvider(ABC):
    """모든 통합 프로바이더의 베이스 클래스"""

    @property
    @abstractmethod
    def id(self) -> str:
        """프로바이더 ID (예: 'github', 'discord')"""
        pass

    @property
    @abstractmethod
    def name(self) -> str:
        """표시 이름"""
        pass

    @property
    @abstractmethod
    def category(self) -> IntegrationCategory:
        pass

    @property
    @abstractmethod
    def capabilities(self) -> list[IntegrationCapability]:
        pass

    @abstractmethod
    async def connect(self, workspace_id: str, auth_code: str) -> dict:
        """OAuth 인증 완료 후 연결"""
        pass

    @abstractmethod
    async def disconnect(self, integration_id: str) -> bool:
        """연결 해제"""
        pass

    @abstractmethod
    async def send_notification(
        self,
        integration_id: str,
        message: str,
        **kwargs
    ) -> bool:
        """알림 전송"""
        pass

    @abstractmethod
    async def handle_webhook(self, payload: bytes, headers: dict) -> dict:
        """Webhook 처리"""
        pass
```

#### B. 프로바이더 레지스트리 (`c4/integrations/registry.py`)

```python
from typing import Type

class IntegrationRegistry:
    """통합 프로바이더 레지스트리 (싱글톤)"""

    _providers: dict[str, Type[IntegrationProvider]] = {}

    @classmethod
    def register(cls, provider_class: Type[IntegrationProvider]):
        """프로바이더 등록 데코레이터"""
        instance = provider_class()
        cls._providers[instance.id] = provider_class
        return provider_class

    @classmethod
    def get(cls, provider_id: str) -> IntegrationProvider | None:
        """프로바이더 인스턴스 반환"""
        provider_class = cls._providers.get(provider_id)
        return provider_class() if provider_class else None

    @classmethod
    def list_all(cls) -> list[dict]:
        """모든 프로바이더 목록"""
        return [
            {
                "id": p().id,
                "name": p().name,
                "category": p().category.value,
                "capabilities": [c.value for c in p().capabilities],
            }
            for p in cls._providers.values()
        ]

# 사용 예시
@IntegrationRegistry.register
class GitHubProvider(IntegrationProvider):
    @property
    def id(self) -> str:
        return "github"
    # ...
```

---

### 7.4 API 엔드포인트

```
c4/api/routes/
├── integrations.py         # 통합 관리 API
└── webhooks.py             # Webhook 수신 (기존 확장)
```

#### A. 통합 관리 API (`c4/api/routes/integrations.py`)

```python
@router.get("/integrations/providers")
async def list_providers():
    """사용 가능한 통합 프로바이더 목록"""
    return IntegrationRegistry.list_all()

@router.get("/workspaces/{workspace_id}/integrations")
async def list_workspace_integrations(
    workspace_id: str,
    current_user: User = Depends(get_current_user),
):
    """워크스페이스의 연결된 통합 목록"""
    # RLS가 자동으로 권한 체크
    return await db.fetch_all(
        "SELECT * FROM workspace_integrations WHERE workspace_id = $1",
        workspace_id
    )

@router.get("/integrations/{provider_id}/oauth/url")
async def get_oauth_url(
    provider_id: str,
    workspace_id: str,
    current_user: User = Depends(get_current_user),
):
    """OAuth 인증 URL 생성"""
    provider = IntegrationRegistry.get(provider_id)
    if not provider:
        raise HTTPException(404, "Provider not found")

    # state에 workspace_id 포함 (CSRF + 라우팅)
    state = encode_state(workspace_id=workspace_id, user_id=current_user.id)
    return {"url": provider.get_oauth_url(state)}

@router.get("/integrations/{provider_id}/oauth/callback")
async def oauth_callback(
    provider_id: str,
    code: str,
    state: str,
):
    """OAuth 콜백 처리"""
    # 1. state 디코딩 → workspace_id, user_id 추출
    state_data = decode_state(state)

    # 2. 프로바이더로 토큰 교환
    provider = IntegrationRegistry.get(provider_id)
    connection = await provider.connect(
        workspace_id=state_data["workspace_id"],
        auth_code=code,
    )

    # 3. DB에 저장
    await db.execute("""
        INSERT INTO workspace_integrations
        (workspace_id, provider_id, external_id, external_name, credentials, connected_by)
        VALUES ($1, $2, $3, $4, $5, $6)
    """, state_data["workspace_id"], provider_id,
        connection["external_id"], connection["name"],
        encrypt(connection["credentials"]), state_data["user_id"])

    # 4. 대시보드로 리다이렉트
    return RedirectResponse(f"/dashboard/integrations?success={provider_id}")

@router.delete("/workspaces/{workspace_id}/integrations/{integration_id}")
async def disconnect_integration(
    workspace_id: str,
    integration_id: str,
    current_user: User = Depends(get_current_user),
):
    """통합 연결 해제"""
    # ...
```

#### B. Webhook 라우팅 (`c4/api/routes/webhooks.py` 확장)

```python
@router.post("/webhooks/{provider_id}")
async def handle_webhook(
    provider_id: str,
    request: Request,
    background_tasks: BackgroundTasks,
):
    """통합 Webhook 엔드포인트"""
    payload = await request.body()
    headers = dict(request.headers)

    # 1. 프로바이더 가져오기
    provider = IntegrationRegistry.get(provider_id)
    if not provider:
        raise HTTPException(404, "Provider not found")

    # 2. 서명 검증 + 파싱
    event = await provider.handle_webhook(payload, headers)

    # 3. external_id로 워크스페이스 찾기
    integration = await db.fetch_one("""
        SELECT * FROM workspace_integrations
        WHERE provider_id = $1 AND external_id = $2 AND status = 'active'
    """, provider_id, event["external_id"])

    if not integration:
        logger.warning(f"No integration found for {provider_id}:{event['external_id']}")
        return {"status": "ignored"}

    # 4. 워크스페이스 컨텍스트로 이벤트 처리
    background_tasks.add_task(
        process_integration_event,
        workspace_id=integration["workspace_id"],
        provider_id=provider_id,
        event=event,
    )

    return {"status": "accepted"}
```

---

### 7.5 OAuth 플로우

```
┌─────────┐     1. "GitHub 연결"      ┌─────────────┐
│  User   │ ──────────────────────→ │  C4 Cloud   │
│ Browser │                          │  Dashboard  │
└────┬────┘                          └──────┬──────┘
     │                                      │
     │ 2. Redirect to GitHub OAuth          │
     │ ←─────────────────────────────────────
     │    state={workspace_id, user_id}
     │
     ▼
┌─────────────┐
│   GitHub    │  3. User authorizes
│   OAuth     │
└──────┬──────┘
       │
       │ 4. Redirect to callback
       │    code=xxx, state=yyy
       ▼
┌─────────────────────────────────────────────┐
│  C4 Cloud: /integrations/github/oauth/callback
│                                             │
│  5. Exchange code → access_token            │
│  6. Get installation_id                     │
│  7. Save to workspace_integrations          │
│  8. Redirect to dashboard                   │
└─────────────────────────────────────────────┘
```

---

### 7.6 프로바이더별 구현 요약

| 프로바이더 | Category | 연결 방식 | 주요 기능 |
|-----------|----------|----------|----------|
| **GitHub** | source_control | GitHub App OAuth | PR 리뷰, 코멘트, 라벨 |
| **Discord** | messaging | Bot OAuth2 | 알림, 명령어, 승인 |
| **Dooray** | collaboration | OAuth2 | 알림, 이슈 연동 |
| **Slack** | messaging | Bot OAuth | 알림, 명령어 |

---

### 7.7 구현 파일 목록

| 순서 | 파일 | 작업 |
|------|------|------|
| 1 | `c4/integrations/base.py` | 추상 베이스 클래스 |
| 2 | `c4/integrations/registry.py` | 프로바이더 레지스트리 |
| 3 | `c4/integrations/github/oauth.py` | GitHub OAuth 플로우 |
| 4 | `c4/api/routes/integrations.py` | 통합 관리 API |
| 5 | `c4/api/routes/webhooks.py` | Webhook 라우팅 확장 |
| 6 | `migrations/xxx_integrations.sql` | DB 스키마 |
| 7 | `c4/integrations/discord/` | Discord 프로바이더 |
| 8 | `c4/integrations/dooray/` | Dooray 프로바이더 |

---

### 7.8 검증 방법

```bash
# 1. 스키마 마이그레이션
uv run alembic upgrade head

# 2. 단위 테스트
uv run pytest tests/unit/test_integration_registry.py
uv run pytest tests/unit/test_oauth_flow.py

# 3. 통합 테스트
uv run pytest tests/integration/test_github_oauth.py

# 4. E2E 테스트
# - C4 대시보드에서 "GitHub 연결" 클릭
# - GitHub 인증
# - 워크스페이스에 연결 확인
# - Webhook 테스트 (테스트 레포에 PR 생성)
```

---

## 8. 완료 현황

### ✅ Phase 1 완료 (GitHub App)
- GitHub App 설정 모델
- GitHub App 인증 + API
- Webhook 핸들러
- PR 리뷰 서비스
- 라우터 등록 + 테스트

### ✅ Phase 2 완료 (통합 프레임워크)
- 통합 베이스 클래스 (`c4/integrations/base.py`)
- 프로바이더 레지스트리 (`c4/integrations/registry.py`)
- GitHub 프로바이더 (`c4/integrations/github_provider.py`)
- Discord 프로바이더 (`c4/integrations/discord_provider.py`)
- DB 스키마 마이그레이션
- 통합 관리 API (`c4/api/routes/integrations.py`)
- 테스트 코드 (68 tests passing)

### 🟡 Phase 3 진행중 (팀 관리) - API 100%, UI 미완료
**✅ 백엔드 완료:**
- Teams 서비스 (`c4/services/teams.py`) - 900+ lines, 전체 구현
- Teams API 라우트 (`c4/api/routes/teams.py`) - 450+ lines
- DB 스키마 (`infra/supabase/migrations/00005_team_enhancements.sql`)
- CLI 서버 라우터 등록 (`c4/api/server.py`)
- 단위 테스트 (`tests/unit/teams/test_teams_service.py`)

**✅ API 연동 완료 (2025-01-24):**
- Cloud API에 Teams Router 등록 (`c4/api/app.py`) - T-3001 ✅
- E2E 테스트 작성 (`tests/e2e/test_teams_api.py`) - T-3002 ✅ (28 tests)
- 프론트엔드 API 클라이언트 (`web/lib/teams.ts`) - T-3003 ✅

**🔴 남은 작업:**
- Teams 관리 UI 컴포넌트 (T-3004)

### 🟡 Phase 4 진행중 (타임트래킹/감사로그) - API 100%, UI 미완료
**✅ 백엔드 완료:**
- ActivityCollector (`c4/services/activity.py`) - 전체 구현
- AuditLogger (`c4/services/audit.py`) - 전체 구현
- Reports API (`c4/api/routes/reports.py`) - 사용량/활동/감사로그
- DB 스키마 (`infra/supabase/migrations/00003_activity_audit_logs.sql`)
- 단위 테스트 (passing)
- Teams/Integrations/SSO 라우트에 이미 통합됨

**✅ API 연동 완료 (2025-01-24):**
- 프론트엔드 Reports API 클라이언트 (`web/lib/reports.ts`) - T-4002 ✅
- 통합 테스트 작성 (`tests/integration/test_activity_audit.py`) - T-4001 ✅

**🔴 남은 작업:**
- Reports/Analytics UI 컴포넌트 (T-4003)

### ✅ Phase 5 완료 (SSO/SAML)
- SSO 데이터베이스 마이그레이션 (`infra/supabase/migrations/00003_sso.sql`)
- SSO 모델 (`c4/services/sso/models.py`)
- SSO 베이스 인터페이스 (`c4/services/sso/base.py`)
- Google OIDC 프로바이더 (`c4/services/sso/providers/google.py`)
- Microsoft OIDC 프로바이더 (`c4/services/sso/providers/microsoft.py`)
- SSO 서비스 (`c4/services/sso/service.py`)
- SSO API 라우트 (`c4/api/routes/sso.py`)
- 테스트 코드 (69 tests passing)

---

## 8.1 C4 태스크 목록 (Phase 3 & 4 마무리)

### Phase 3: 팀 관리 마무리

#### T-3001: Cloud API에 Teams Router 등록
- **scope**: `c4/api/app.py`
- **dod**:
  1. `c4/api/app.py`에 teams_router import 추가
  2. `/api/teams` prefix로 라우터 등록
  3. `/api/invites` prefix로 invite_router 등록
  4. Cloud API 서버 시작 시 에러 없음
  5. `GET /api/teams` 엔드포인트 200 응답 확인
- **dependencies**: 없음
- **priority**: 1

#### T-3002: Teams API E2E 테스트 작성
- **scope**: `tests/e2e/test_teams_api.py`
- **dod**:
  1. 팀 생성 → 멤버 초대 → 초대 수락 전체 플로우 테스트
  2. 권한 검증 테스트 (owner/admin/member/viewer 각 역할)
  3. 팀 설정 변경 테스트
  4. 팀 삭제 테스트
  5. 모든 테스트 `uv run pytest tests/e2e/test_teams_api.py` 통과
- **dependencies**: T-3001
- **priority**: 2

#### T-3003: 프론트엔드 Teams API 클라이언트
- **scope**: `web/lib/api/teams.ts`
- **dod**:
  1. TeamsAPI 클래스 구현 (list, create, get, update, delete)
  2. MembersAPI 구현 (list, invite, updateRole, remove)
  3. TypeScript 타입 정의 (Team, TeamMember, TeamRole, TeamInvite)
  4. 에러 핸들링 + toast 알림
  5. `pnpm build` 에러 없음
- **dependencies**: T-3001
- **priority**: 2

#### T-3004: Teams 관리 UI 컴포넌트 구현
- **scope**: `web/app/(dashboard)/teams/`
- **dod**:
  1. 팀 목록 페이지 (`/teams`) - 카드 그리드
  2. 팀 상세 페이지 (`/teams/[teamId]`) - 개요 + 통계
  3. 멤버 관리 페이지 (`/teams/[teamId]/members`) - 목록 + 역할 변경
  4. 팀 설정 페이지 (`/teams/[teamId]/settings`) - 이름/slug 변경
  5. 초대 모달 컴포넌트 (이메일 + 역할 선택)
  6. 반응형 디자인 (모바일/태블릿/데스크톱)
  7. `pnpm build` 에러 없음
- **dependencies**: T-3003
- **priority**: 3

### Phase 4: 리포트/분석 마무리

#### ✅ T-4001: Activity/Audit 통합 테스트 작성 (완료 2025-01-24)
- **scope**: `tests/integration/test_activity_audit.py`
- **dod**:
  1. ActivityCollector 통합 테스트 (실제 Supabase DB)
  2. AuditLogger 통합 테스트 (실제 Supabase DB)
  3. Reports API 통합 테스트 (/usage, /activities, /audit)
  4. 데이터 일관성 검증 (생성 → 조회 → 내보내기)
  5. 모든 테스트 `uv run pytest tests/integration/test_activity_audit.py` 통과
- **dependencies**: 없음
- **priority**: 2
- **status**: ✅ 완료 - 22개 테스트 케이스 작성, DB 마이그레이션 적용 후 실행 가능

#### T-4002: 프론트엔드 Reports API 클라이언트
- **scope**: `web/lib/api/reports.ts`
- **dod**:
  1. ReportsAPI 클래스 구현
  2. getUsageReport(teamId, startDate, endDate) 함수
  3. getActivities(teamId, filters, pagination) 함수
  4. getAuditLogs(teamId, filters, pagination) 함수
  5. exportAuditLogs(teamId, format: 'csv'|'json') 함수
  6. TypeScript 타입 정의 (UsageReport, ActivityLog, AuditLog)
  7. `pnpm build` 에러 없음
- **dependencies**: 없음
- **priority**: 2

#### T-4003: Reports/Analytics UI 컴포넌트 구현
- **scope**: `web/app/(dashboard)/teams/[teamId]/reports/`
- **dod**:
  1. 사용량 대시보드 (`/teams/[teamId]/reports`) - 주요 지표 카드
  2. 활동 로그 타임라인 컴포넌트 - 날짜별 그룹핑
  3. 감사 로그 테이블 - 필터링/페이지네이션/정렬
  4. CSV/JSON 내보내기 버튼 (다운로드)
  5. 차트 시각화 (recharts 사용) - 일별/주별 트렌드
  6. 반응형 디자인
  7. `pnpm build` 에러 없음
- **dependencies**: T-4002
- **priority**: 3

### 체크포인트

#### CP-PHASE3: Phase 3 팀 관리 완료 검증
- **required_tasks**: [T-3001, T-3002, T-3003, T-3004]
- **required_validations**: [lint, unit, e2e]
- **description**: 팀 관리 기능 전체 완료 - API + UI 동작 확인

#### CP-PHASE4: Phase 4 리포트/분석 완료 검증
- **required_tasks**: [T-4001, T-4002, T-4003]
- **required_validations**: [lint, unit, integration]
- **description**: 리포트/분석 기능 전체 완료 - 사용량/활동/감사 확인

---

## 9. Phase 3: 팀 관리 + 클라이언트 포털 (참조용)

> **목표**: 에이전시/스타트업을 위한 멀티테넌트 팀 시스템

### 9.1 현재 상태 분석 (2025-01-24 업데이트)

**✅ 백엔드 완료:**
- JWT + API Key 인증 (`c4/api/auth.py`)
- 기본 사용자 모델 (user_id, email, is_api_key_user)
- Stripe 과금 시스템 (`c4/billing/`)
- 워크스페이스 관리 (`c4/workspace/`)
- **Teams 서비스 (`c4/services/teams.py`)** - 900+ lines
- **Teams API (`c4/api/routes/teams.py`)** - 450+ lines
- **DB 스키마** - teams, team_members, team_branding 등

**🔴 프론트엔드 필요:**
- Next.js 프론트엔드 (`web/`) - Teams UI 추가 필요
- Teams API 클라이언트
- 팀 관리 페이지들

### 9.2 데이터베이스 스키마

```sql
-- 1. 팀 테이블
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,  -- URL-friendly identifier
    owner_id UUID NOT NULL REFERENCES auth.users(id),

    -- 과금 연결
    stripe_customer_id TEXT,
    plan TEXT DEFAULT 'free',  -- free, pro, team, agency, enterprise

    -- 설정
    settings JSONB DEFAULT '{}',

    -- 메타데이터
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. 팀 멤버 테이블
CREATE TABLE team_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES auth.users(id),

    -- 역할 (RBAC)
    role TEXT NOT NULL DEFAULT 'member',
    -- owner: 모든 권한 + 팀 삭제
    -- admin: 멤버 관리 + 설정 변경
    -- member: 워크스페이스 접근 + 태스크 실행
    -- viewer: 읽기 전용

    -- 초대 상태
    invited_by UUID REFERENCES auth.users(id),
    invited_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(team_id, user_id)
);

-- 3. 워크스페이스에 팀 연결 추가
ALTER TABLE workspaces
ADD COLUMN team_id UUID REFERENCES teams(id);

-- 4. 인덱스
CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE INDEX idx_team_members_team ON team_members(team_id);
CREATE INDEX idx_teams_owner ON teams(owner_id);
CREATE INDEX idx_workspaces_team ON workspaces(team_id);

-- 5. RLS 정책
ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;

-- 팀 접근: 멤버만
CREATE POLICY "Team members can view team"
ON teams FOR SELECT
USING (id IN (
    SELECT team_id FROM team_members WHERE user_id = auth.uid()
));

-- 팀 수정: owner/admin만
CREATE POLICY "Team admins can update team"
ON teams FOR UPDATE
USING (id IN (
    SELECT team_id FROM team_members
    WHERE user_id = auth.uid() AND role IN ('owner', 'admin')
));

-- 멤버 조회: 같은 팀
CREATE POLICY "Members can view team members"
ON team_members FOR SELECT
USING (team_id IN (
    SELECT team_id FROM team_members WHERE user_id = auth.uid()
));
```

### 9.3 백엔드 구현

#### A. 팀 모델 (`c4/teams/models.py`)

```python
from enum import Enum
from pydantic import BaseModel
from datetime import datetime

class TeamRole(str, Enum):
    OWNER = "owner"
    ADMIN = "admin"
    MEMBER = "member"
    VIEWER = "viewer"

class TeamPlan(str, Enum):
    FREE = "free"
    PRO = "pro"
    TEAM = "team"
    AGENCY = "agency"
    ENTERPRISE = "enterprise"

class Team(BaseModel):
    id: str
    name: str
    slug: str
    owner_id: str
    plan: TeamPlan = TeamPlan.FREE
    settings: dict = {}
    created_at: datetime

class TeamMember(BaseModel):
    id: str
    team_id: str
    user_id: str
    role: TeamRole
    email: str | None
    accepted_at: datetime | None
    created_at: datetime

class TeamInvite(BaseModel):
    email: str
    role: TeamRole = TeamRole.MEMBER
```

#### B. 팀 서비스 (`c4/teams/service.py`)

```python
class TeamService:
    async def create_team(self, owner_id: str, name: str) -> Team:
        """팀 생성 + owner 멤버 자동 추가"""

    async def get_user_teams(self, user_id: str) -> list[Team]:
        """사용자가 속한 팀 목록"""

    async def get_team_members(self, team_id: str) -> list[TeamMember]:
        """팀 멤버 목록"""

    async def invite_member(
        self, team_id: str, email: str, role: TeamRole, invited_by: str
    ) -> TeamMember:
        """이메일로 멤버 초대"""

    async def accept_invite(self, user_id: str, team_id: str) -> bool:
        """초대 수락"""

    async def update_member_role(
        self, team_id: str, member_id: str, new_role: TeamRole
    ) -> TeamMember:
        """멤버 역할 변경"""

    async def remove_member(self, team_id: str, member_id: str) -> bool:
        """멤버 제거"""

    async def check_permission(
        self, user_id: str, team_id: str, action: str
    ) -> bool:
        """권한 확인"""
```

#### C. 팀 API (`c4/api/routes/teams.py`)

```python
@router.post("/teams")
async def create_team(
    name: str,
    current_user: User = Depends(get_current_user),
) -> Team:
    """새 팀 생성"""

@router.get("/teams")
async def list_teams(
    current_user: User = Depends(get_current_user),
) -> list[Team]:
    """내 팀 목록"""

@router.get("/teams/{team_id}")
async def get_team(
    team_id: str,
    current_user: User = Depends(get_current_user),
) -> Team:
    """팀 상세 정보"""

@router.get("/teams/{team_id}/members")
async def list_members(
    team_id: str,
    current_user: User = Depends(get_current_user),
) -> list[TeamMember]:
    """팀 멤버 목록"""

@router.post("/teams/{team_id}/members")
async def invite_member(
    team_id: str,
    invite: TeamInvite,
    current_user: User = Depends(get_current_user),
) -> TeamMember:
    """멤버 초대 (admin 이상)"""

@router.patch("/teams/{team_id}/members/{member_id}")
async def update_member(
    team_id: str,
    member_id: str,
    role: TeamRole,
    current_user: User = Depends(get_current_user),
) -> TeamMember:
    """역할 변경 (admin 이상)"""

@router.delete("/teams/{team_id}/members/{member_id}")
async def remove_member(
    team_id: str,
    member_id: str,
    current_user: User = Depends(get_current_user),
) -> dict:
    """멤버 제거 (admin 이상)"""
```

### 9.4 프론트엔드 구현

#### 페이지 구조

```
web/src/app/
├── (auth)/
│   ├── login/page.tsx
│   └── signup/page.tsx
├── dashboard/
│   ├── page.tsx                    # 팀 대시보드 (메인)
│   ├── layout.tsx                  # 팀 네비게이션
│   └── [teamId]/
│       ├── page.tsx                # 팀 워크스페이스 목록
│       ├── members/page.tsx        # 팀 멤버 관리
│       ├── settings/page.tsx       # 팀 설정
│       ├── billing/page.tsx        # 과금 현황
│       └── integrations/page.tsx   # 연동 설정
└── invite/[token]/page.tsx         # 초대 수락 페이지
```

#### 주요 컴포넌트

```typescript
// components/teams/TeamSwitcher.tsx
// 헤더에서 팀 전환

// components/teams/MemberList.tsx
// 멤버 목록 + 역할 변경

// components/teams/InviteModal.tsx
// 멤버 초대 모달

// components/dashboard/WorkspaceGrid.tsx
// 팀 워크스페이스 카드 그리드

// components/dashboard/ActivityFeed.tsx
// 최근 활동 피드
```

### 9.5 구현 순서

| 순서 | 파일 | 작업 |
|------|------|------|
| 1 | `migrations/xxx_teams.sql` | 팀/멤버 스키마 |
| 2 | `c4/teams/models.py` | 팀 모델 |
| 3 | `c4/teams/service.py` | 팀 서비스 로직 |
| 4 | `c4/api/routes/teams.py` | 팀 API |
| 5 | `c4/api/app.py` | 라우터 등록 |
| 6 | `web/src/app/dashboard/` | 대시보드 UI |
| 7 | `web/src/components/teams/` | 팀 컴포넌트 |
| 8 | `tests/unit/teams/` | 테스트 |

### 9.6 검증 방법

```bash
# 1. 마이그레이션
uv run alembic upgrade head

# 2. 단위 테스트
uv run pytest tests/unit/teams/

# 3. API 테스트
uv run pytest tests/integration/test_teams_api.py

# 4. E2E 테스트
# - 팀 생성
# - 멤버 초대 (이메일 발송)
# - 초대 수락
# - 역할 변경
# - 워크스페이스 접근 확인
```

---

## 10. Phase 4: 타임 트래킹 + 감사 로그

> **목표**: 에이전시 빌링과 Enterprise 컴플라이언스

### 10.1 타임 트래킹 시스템

#### 데이터 모델

```sql
-- 활동 로그 (자동 수집)
CREATE TABLE activity_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id),
    user_id UUID REFERENCES auth.users(id),  -- NULL for system/worker
    workspace_id UUID REFERENCES workspaces(id),

    -- 활동 정보
    activity_type TEXT NOT NULL,
    -- task_started, task_completed, pr_created, review_submitted,
    -- command_executed, checkpoint_approved, etc.

    -- 상세
    resource_type TEXT,  -- task, pr, workspace, etc.
    resource_id TEXT,
    metadata JSONB DEFAULT '{}',

    -- 시간
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    duration_seconds INTEGER GENERATED ALWAYS AS (
        EXTRACT(EPOCH FROM (ended_at - started_at))
    ) STORED,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 시간 집계 뷰 (빌링용)
CREATE VIEW team_usage_summary AS
SELECT
    team_id,
    DATE_TRUNC('day', started_at) as date,
    activity_type,
    COUNT(*) as count,
    SUM(duration_seconds) as total_seconds
FROM activity_logs
GROUP BY team_id, DATE_TRUNC('day', started_at), activity_type;
```

#### 활동 수집기

```python
# c4/tracking/collector.py
class ActivityCollector:
    async def log_activity(
        self,
        team_id: str,
        activity_type: str,
        *,
        user_id: str | None = None,
        workspace_id: str | None = None,
        resource_type: str | None = None,
        resource_id: str | None = None,
        metadata: dict | None = None,
        started_at: datetime | None = None,
        ended_at: datetime | None = None,
    ) -> None:
        """활동 기록"""

    @contextmanager
    async def track_activity(
        self,
        team_id: str,
        activity_type: str,
        **kwargs
    ):
        """활동 시간 자동 측정"""
        started_at = datetime.now(UTC)
        try:
            yield
        finally:
            ended_at = datetime.now(UTC)
            await self.log_activity(
                team_id, activity_type,
                started_at=started_at,
                ended_at=ended_at,
                **kwargs
            )
```

### 10.2 감사 로그 시스템

#### 데이터 모델

```sql
-- 감사 로그 (보안 이벤트)
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id),

    -- 행위자
    actor_type TEXT NOT NULL,  -- user, api_key, system, worker
    actor_id TEXT NOT NULL,
    actor_email TEXT,

    -- 행위
    action TEXT NOT NULL,
    -- team.created, member.invited, member.role_changed,
    -- workspace.created, workspace.deleted,
    -- integration.connected, integration.disconnected,
    -- settings.updated, etc.

    -- 대상
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,

    -- 변경 내용
    old_value JSONB,
    new_value JSONB,

    -- 컨텍스트
    ip_address INET,
    user_agent TEXT,
    request_id TEXT,

    -- 시간
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- 불변성 보장
    hash TEXT GENERATED ALWAYS AS (
        encode(sha256(
            (id || actor_id || action || resource_id || created_at::text)::bytea
        ), 'hex')
    ) STORED
);

-- 인덱스
CREATE INDEX idx_audit_logs_team ON audit_logs(team_id, created_at DESC);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
```

#### 감사 로거

```python
# c4/audit/logger.py
class AuditLogger:
    async def log(
        self,
        team_id: str,
        action: str,
        resource_type: str,
        resource_id: str,
        *,
        actor: User | None = None,
        old_value: dict | None = None,
        new_value: dict | None = None,
        request: Request | None = None,
    ) -> None:
        """감사 이벤트 기록"""

    async def get_logs(
        self,
        team_id: str,
        *,
        actor_id: str | None = None,
        action: str | None = None,
        resource_type: str | None = None,
        start_date: datetime | None = None,
        end_date: datetime | None = None,
        limit: int = 100,
    ) -> list[AuditLog]:
        """감사 로그 조회"""

    async def export_logs(
        self,
        team_id: str,
        format: str = "csv",  # csv, json
        **filters
    ) -> bytes:
        """감사 로그 내보내기 (컴플라이언스)"""
```

### 10.3 리포트 API

```python
# c4/api/routes/reports.py

@router.get("/teams/{team_id}/usage")
async def get_usage_report(
    team_id: str,
    start_date: date,
    end_date: date,
    current_user: User = Depends(get_current_user),
) -> UsageReport:
    """사용량 리포트 (빌링용)"""

@router.get("/teams/{team_id}/audit")
async def get_audit_logs(
    team_id: str,
    filters: AuditFilter = Depends(),
    current_user: User = Depends(get_current_user),
) -> list[AuditLog]:
    """감사 로그 조회"""

@router.get("/teams/{team_id}/audit/export")
async def export_audit_logs(
    team_id: str,
    format: str = "csv",
    filters: AuditFilter = Depends(),
    current_user: User = Depends(get_current_user),
) -> FileResponse:
    """감사 로그 내보내기"""
```

---

## 11. Phase 5: SSO/SAML (Enterprise)

> **목표**: 대기업 인증 시스템 통합

### 11.1 지원 프로바이더

| 프로바이더 | 프로토콜 | 우선순위 |
|-----------|----------|----------|
| **Google Workspace** | OAuth2/OIDC | P0 |
| **Microsoft Entra ID** | OAuth2/OIDC + SAML | P0 |
| **Okta** | SAML 2.0 | P1 |
| **OneLogin** | SAML 2.0 | P2 |
| **Custom SAML** | SAML 2.0 | P2 |

### 11.2 데이터베이스 스키마

```sql
-- SSO 설정 (팀별)
CREATE TABLE team_sso_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    -- 프로바이더
    provider TEXT NOT NULL,  -- google, microsoft, okta, saml

    -- OIDC 설정
    client_id TEXT,
    client_secret_encrypted TEXT,
    issuer_url TEXT,

    -- SAML 설정
    entity_id TEXT,
    sso_url TEXT,
    certificate TEXT,

    -- 옵션
    auto_provision BOOLEAN DEFAULT true,  -- JIT 프로비저닝
    default_role TEXT DEFAULT 'member',
    allowed_domains TEXT[],  -- 허용 이메일 도메인

    -- 상태
    enabled BOOLEAN DEFAULT false,
    verified BOOLEAN DEFAULT false,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(team_id)
);

-- SSO 세션
CREATE TABLE sso_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth.users(id),
    team_id UUID NOT NULL REFERENCES teams(id),

    provider TEXT NOT NULL,
    provider_user_id TEXT,

    -- SAML assertion 저장 (감사용)
    assertion_hash TEXT,

    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 11.3 SSO 서비스

```python
# c4/sso/service.py
class SSOService:
    async def initiate_login(
        self, team_slug: str, redirect_uri: str
    ) -> str:
        """SSO 로그인 시작 → redirect URL 반환"""

    async def handle_callback(
        self, team_slug: str, code: str | None, saml_response: str | None
    ) -> tuple[User, str]:
        """콜백 처리 → (user, jwt_token)"""

    async def provision_user(
        self, team_id: str, provider_user: ProviderUser
    ) -> User:
        """JIT 사용자 프로비저닝"""

    async def validate_session(
        self, session_id: str
    ) -> User | None:
        """SSO 세션 검증"""

# c4/sso/providers/
# ├── base.py          # 추상 베이스
# ├── google.py        # Google Workspace
# ├── microsoft.py     # Microsoft Entra
# ├── okta.py          # Okta SAML
# └── generic_saml.py  # 일반 SAML
```

### 11.4 SSO 로그인 플로우

```
1. 사용자: /login?team=acme 접근
2. C4: team SSO 설정 확인
3. C4: SSO 프로바이더로 리다이렉트
4. 프로바이더: 인증 + 콜백
5. C4: 사용자 프로비저닝 (없으면 생성)
6. C4: JWT 발급 + 대시보드 리다이렉트
```

### 11.5 구현 순서

| 순서 | 작업 |
|------|------|
| 1 | DB 스키마 + 마이그레이션 |
| 2 | SSO 베이스 인터페이스 |
| 3 | Google OIDC 프로바이더 |
| 4 | Microsoft OIDC 프로바이더 |
| 5 | SSO 설정 API |
| 6 | SSO 로그인 페이지 |
| 7 | SAML 프로바이더 (Okta) |
| 8 | 테스트 + 문서 |

---

## 12. Phase 6: 추가 기능 (선택적)

### 12.1 템플릿 라이브러리

```
목적: 프로젝트 유형별 빠른 시작

기능:
- 공식 템플릿 (ecommerce, healthcare, fintech)
- 팀 커스텀 템플릿
- 템플릿 마켓플레이스

구현:
- templates/ 디렉토리 구조
- 템플릿 메타데이터 (checkpoints, validations)
- 템플릿 적용 CLI (/c4-init --template=ecommerce)
```

### 12.2 화이트라벨

```
목적: 에이전시가 자체 브랜딩으로 제공

기능:
- 커스텀 도메인 (client.agency.com)
- 로고/색상 커스터마이징
- 이메일 템플릿 브랜딩

구현:
- team_branding 테이블
- 동적 테마 시스템
- 이메일 템플릿 변수
```

### 12.3 클라이언트 포털 (에이전시용)

```
목적: 에이전시 고객에게 진행상황 공유

기능:
- 읽기 전용 대시보드
- 진행률 + 마일스톤
- 코멘트/피드백 시스템

구현:
- client_portals 테이블
- 공유 링크 생성
- 제한된 API 접근
```

---

## 13. Phase 7: Self-Documenting Platform (자가 문서화 플랫폼)

> **목표**: C4 Cloud가 Serena처럼 코드 분석 MCP 역할 + Context7처럼 자가 문서화 + TDD Gap Analyzer로 스펙 vs 구현 차이 분석

### 13.1 핵심 아이디어

```
┌─────────────────────────────────────────────────────────────────┐
│                    C4 Self-Documenting Platform                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐       │
│  │ Serena-Like   │  │ Context7-Like │  │ TDD Gap       │       │
│  │ Code Analysis │  │ Self-Doc      │  │ Analyzer      │       │
│  ├───────────────┤  ├───────────────┤  ├───────────────┤       │
│  │ - AST Parsing │  │ - API Docs    │  │ - EARS Specs  │       │
│  │ - Symbol Find │  │ - Usage Guide │  │ - Impl Status │       │
│  │ - Dependency  │  │ - Examples    │  │ - Gap Report  │       │
│  │ - References  │  │ - Changelog   │  │ - Auto Tests  │       │
│  └───────────────┘  └───────────────┘  └───────────────┘       │
│           │                 │                  │                │
│           └─────────────────┴──────────────────┘                │
│                             │                                   │
│                    ┌────────┴────────┐                         │
│                    │  MCP Endpoints  │                         │
│                    └─────────────────┘                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 13.2 DDD (Documentation-Driven Development) 워크플로우

```
1. EARS 스펙 작성 (.c4/specs/)
       ↓
2. C4가 스펙 기반 테스트 자동 생성
       ↓
3. 테스트가 실패하면 → 구현 필요 (RED)
       ↓
4. 구현 완료 → 테스트 통과 (GREEN)
       ↓
5. Gap Analyzer가 스펙 vs 구현 비교
       ↓
6. 완료된 기능 → 자동 문서화 (Context7 스타일)
       ↓
7. 외부 개발자가 C4 API 문서 참조 가능
```

### 13.3 데이터베이스 스키마

```sql
-- 코드 심볼 (AST 분석 결과)
CREATE TABLE code_symbols (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

    -- 심볼 정보
    name TEXT NOT NULL,
    kind TEXT NOT NULL,  -- class, function, method, variable, type
    file_path TEXT NOT NULL,
    line_start INTEGER,
    line_end INTEGER,

    -- 부모/자식 관계
    parent_id UUID REFERENCES code_symbols(id),

    -- 메타데이터
    signature TEXT,          -- 함수 시그니처
    docstring TEXT,          -- 문서 문자열
    return_type TEXT,
    parameters JSONB,        -- [{name, type, default}]
    decorators TEXT[],       -- @router.get, @dataclass 등

    -- 의존성
    imports TEXT[],          -- 이 심볼이 import하는 것들
    references TEXT[],       -- 이 심볼을 참조하는 파일들

    -- 분석 시점
    commit_sha TEXT,
    analyzed_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(workspace_id, file_path, name, kind)
);

-- 스펙-구현 매핑
CREATE TABLE spec_implementation_map (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

    -- 스펙 정보 (EARS 패턴)
    spec_id TEXT NOT NULL,           -- REQ-001
    spec_pattern TEXT NOT NULL,      -- event-driven, state-driven 등
    spec_text TEXT NOT NULL,         -- EARS 형식 요구사항 문장
    feature TEXT NOT NULL,           -- user-auth, dashboard 등

    -- 구현 상태
    status TEXT DEFAULT 'pending',   -- pending, partial, complete, verified

    -- 연결된 심볼들
    implementing_symbols UUID[],     -- code_symbols.id 배열

    -- 테스트 연결
    test_file_path TEXT,
    test_function_name TEXT,
    test_status TEXT,                -- none, failing, passing

    -- 갭 분석
    gap_analysis JSONB,              -- {missing: [], partial: [], notes: ""}
    last_analyzed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 자동 생성된 테스트
CREATE TABLE auto_generated_tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

    -- 소스
    spec_id TEXT NOT NULL,
    ears_pattern TEXT NOT NULL,

    -- 생성된 테스트
    test_code TEXT NOT NULL,
    test_file_path TEXT NOT NULL,
    test_function_name TEXT NOT NULL,

    -- 상태
    status TEXT DEFAULT 'generated',  -- generated, approved, applied, rejected
    applied_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 문서 스냅샷 (Context7 스타일)
CREATE TABLE documentation_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,

    -- 문서 정보
    doc_type TEXT NOT NULL,          -- api, guide, changelog, reference
    title TEXT NOT NULL,
    content TEXT NOT NULL,           -- Markdown

    -- 버전
    version TEXT,
    commit_sha TEXT,

    -- 메타데이터
    symbols_covered UUID[],          -- 관련 code_symbols
    specs_covered TEXT[],            -- 관련 spec_ids

    -- 접근성
    is_public BOOLEAN DEFAULT false, -- 외부 공개 여부

    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 인덱스
CREATE INDEX idx_code_symbols_workspace ON code_symbols(workspace_id);
CREATE INDEX idx_code_symbols_file ON code_symbols(workspace_id, file_path);
CREATE INDEX idx_code_symbols_kind ON code_symbols(workspace_id, kind);
CREATE INDEX idx_spec_map_workspace ON spec_implementation_map(workspace_id);
CREATE INDEX idx_spec_map_status ON spec_implementation_map(workspace_id, status);
CREATE INDEX idx_doc_snapshots_type ON documentation_snapshots(workspace_id, doc_type);
```

### 13.4 MCP 엔드포인트 (Serena-Like)

```python
# c4/mcp/code_analysis.py

class CodeAnalysisMCP:
    """Serena 스타일 코드 분석 MCP 엔드포인트"""

    @mcp_tool("analyze_codebase")
    async def analyze_codebase(
        self,
        workspace_id: str,
        paths: list[str] = None,  # None이면 전체
        languages: list[str] = ["python", "typescript"],
    ) -> dict:
        """
        코드베이스 분석 및 심볼 추출

        Returns:
            {
                "symbols_count": 1234,
                "files_analyzed": 56,
                "languages": {"python": 40, "typescript": 16},
                "top_level_symbols": [...],
            }
        """

    @mcp_tool("find_symbol")
    async def find_symbol(
        self,
        workspace_id: str,
        name: str,
        kind: str = None,  # class, function, method
        include_body: bool = False,
    ) -> list[Symbol]:
        """심볼 검색 (Serena의 find_symbol과 동일)"""

    @mcp_tool("find_references")
    async def find_references(
        self,
        workspace_id: str,
        symbol_id: str,
    ) -> list[Reference]:
        """심볼 참조 찾기"""

    @mcp_tool("get_symbol_tree")
    async def get_symbol_tree(
        self,
        workspace_id: str,
        file_path: str,
        depth: int = 2,
    ) -> dict:
        """파일의 심볼 트리 구조 반환"""
```

### 13.5 Context7-Like 문서화

```python
# c4/mcp/documentation.py

class DocumentationMCP:
    """Context7 스타일 자가 문서화 MCP"""

    @mcp_tool("generate_api_docs")
    async def generate_api_docs(
        self,
        workspace_id: str,
        output_format: str = "markdown",  # markdown, openapi
    ) -> str:
        """API 문서 자동 생성"""

    @mcp_tool("query_docs")
    async def query_docs(
        self,
        workspace_id: str,
        query: str,
        doc_type: str = None,
    ) -> list[DocResult]:
        """문서 검색 (Context7의 query-docs와 동일)"""

    @mcp_tool("get_usage_examples")
    async def get_usage_examples(
        self,
        workspace_id: str,
        symbol_name: str,
    ) -> list[CodeExample]:
        """심볼 사용 예시 추출"""

    @mcp_tool("generate_changelog")
    async def generate_changelog(
        self,
        workspace_id: str,
        from_commit: str,
        to_commit: str = "HEAD",
    ) -> str:
        """커밋 기반 변경 로그 생성"""
```

### 13.6 TDD Gap Analyzer

```python
# c4/mcp/gap_analyzer.py

class GapAnalyzerMCP:
    """EARS 스펙 vs 구현 차이 분석"""

    @mcp_tool("analyze_spec_gaps")
    async def analyze_spec_gaps(
        self,
        workspace_id: str,
        feature: str = None,  # None이면 전체
    ) -> GapReport:
        """
        스펙과 구현 사이의 갭 분석

        Returns:
            GapReport {
                total_specs: 25,
                implemented: 18,
                partial: 4,
                missing: 3,
                coverage_percent: 72.0,
                gaps: [
                    {
                        spec_id: "REQ-015",
                        spec_text: "When user clicks login...",
                        status: "partial",
                        missing_parts: ["error handling", "rate limiting"],
                        suggested_tests: [...]
                    }
                ]
            }
        """

    @mcp_tool("generate_tests_from_spec")
    async def generate_tests_from_spec(
        self,
        workspace_id: str,
        spec_id: str,
    ) -> GeneratedTest:
        """EARS 스펙에서 테스트 코드 자동 생성"""

    @mcp_tool("link_impl_to_spec")
    async def link_impl_to_spec(
        self,
        workspace_id: str,
        spec_id: str,
        symbol_ids: list[str],
    ) -> bool:
        """구현 심볼을 스펙에 연결"""

    @mcp_tool("verify_spec_completion")
    async def verify_spec_completion(
        self,
        workspace_id: str,
        spec_id: str,
    ) -> VerificationResult:
        """스펙 완료 여부 검증 (테스트 실행 포함)"""
```

### 13.7 Gap 분석 알고리즘

```python
def analyze_gaps(specs: list[Spec], symbols: list[Symbol]) -> GapReport:
    """
    EARS 스펙과 코드 심볼 비교하여 갭 분석

    분석 기준:
    1. 키워드 매칭: 스펙의 동사/명사 → 함수/클래스 이름
    2. 테스트 존재 여부: 스펙 ID가 테스트에 언급되는지
    3. 패턴 매칭:
       - event-driven → 이벤트 핸들러 존재?
       - state-driven → 상태 관리 로직 존재?
       - unwanted → 에러 핸들링 존재?
    """

    for spec in specs:
        # 1. 관련 심볼 찾기
        related_symbols = find_related_symbols(spec, symbols)

        # 2. 테스트 매핑
        tests = find_tests_for_spec(spec)

        # 3. 완성도 계산
        if not related_symbols:
            status = "missing"
        elif all_tests_passing(tests):
            status = "complete"
        else:
            status = "partial"

        # 4. 부족한 부분 식별
        missing_parts = identify_missing_parts(spec, related_symbols)
```

### 13.8 EARS 패턴별 테스트 생성 템플릿

```python
EARS_TEST_TEMPLATES = {
    "event-driven": '''
def test_{spec_id}_{action}():
    """
    EARS: {spec_text}
    """
    # Arrange: 이벤트 발생 조건 설정
    {setup_code}

    # Act: 이벤트 트리거
    result = {trigger_code}

    # Assert: 시스템 응답 검증
    assert {assertion}
''',

    "state-driven": '''
def test_{spec_id}_{state}():
    """
    EARS: {spec_text}
    """
    # Arrange: 특정 상태로 진입
    system = setup_state("{state}")

    # Act: 상태에서의 동작
    result = system.{action}()

    # Assert: 상태별 동작 검증
    assert {assertion}
''',

    "unwanted": '''
def test_{spec_id}_prevents_{condition}():
    """
    EARS: {spec_text}
    """
    # Arrange: 비정상 상황 설정
    {setup_code}

    # Act & Assert: 예외 처리 검증
    with pytest.raises({exception}):
        {action_code}
    # 또는 에러 응답 검증
    assert result.error == {expected_error}
''',
}
```

### 13.9 구현 태스크

#### T-7001: 코드 심볼 분석 엔진
- **scope**: `c4/services/code_analysis/`
- **dod**:
  1. Python AST 파서 구현 (ast 모듈 사용)
  2. TypeScript AST 파서 구현 (tree-sitter 사용)
  3. 클래스/함수/메서드/변수 추출
  4. 의존성 그래프 생성
  5. code_symbols 테이블에 저장
- **dependencies**: 없음
- **priority**: 1

#### T-7002: 스펙-구현 매핑 서비스
- **scope**: `c4/services/spec_mapper/`
- **dod**:
  1. EARS 스펙 파싱 (.c4/specs/*.yaml)
  2. 키워드 기반 심볼 매칭
  3. 수동 링크 지원 (API)
  4. spec_implementation_map 테이블 관리
- **dependencies**: T-7001
- **priority**: 2

#### T-7003: Gap Analyzer 엔진
- **scope**: `c4/services/gap_analyzer/`
- **dod**:
  1. 스펙 vs 구현 비교 알고리즘
  2. 커버리지 계산 (%, 상태별)
  3. 미구현/부분구현 식별
  4. 갭 리포트 생성 (JSON/Markdown)
- **dependencies**: T-7002
- **priority**: 2

#### T-7004: 테스트 자동 생성
- **scope**: `c4/services/test_generator/`
- **dod**:
  1. EARS 패턴별 테스트 템플릿
  2. pytest/vitest 코드 생성
  3. 생성된 테스트 저장 (auto_generated_tests)
  4. 사용자 승인 워크플로우
- **dependencies**: T-7002
- **priority**: 3

#### T-7005: MCP 코드 분석 엔드포인트
- **scope**: `c4/mcp/code_analysis.py`
- **dod**:
  1. analyze_codebase 도구
  2. find_symbol 도구
  3. find_references 도구
  4. get_symbol_tree 도구
  5. MCP 서버 등록
- **dependencies**: T-7001
- **priority**: 2

#### T-7006: MCP 문서화 엔드포인트
- **scope**: `c4/mcp/documentation.py`
- **dod**:
  1. generate_api_docs 도구
  2. query_docs 도구
  3. get_usage_examples 도구
  4. generate_changelog 도구
- **dependencies**: T-7001, T-7005
- **priority**: 3

#### T-7007: MCP Gap Analyzer 엔드포인트
- **scope**: `c4/mcp/gap_analyzer.py`
- **dod**:
  1. analyze_spec_gaps 도구
  2. generate_tests_from_spec 도구
  3. link_impl_to_spec 도구
  4. verify_spec_completion 도구
- **dependencies**: T-7003, T-7004
- **priority**: 3

#### T-7008: 문서 스냅샷 및 공개 API
- **scope**: `c4/api/routes/docs.py`, `c4/services/documentation/`
- **dod**:
  1. 문서 스냅샷 생성/저장
  2. 공개 문서 API (Context7 스타일)
  3. 외부 개발자용 C4 API 문서
  4. 버전별 문서 관리
- **dependencies**: T-7006
- **priority**: 4

### 13.10 체크포인트

#### CP-PHASE7-CORE: 핵심 분석 기능 완료
- **required_tasks**: [T-7001, T-7002, T-7003, T-7005]
- **required_validations**: [lint, unit]
- **description**: 코드 분석 + 스펙 매핑 + 갭 분석 기본 동작

#### CP-PHASE7-FULL: 전체 자가 문서화 완료
- **required_tasks**: [T-7001, T-7002, T-7003, T-7004, T-7005, T-7006, T-7007, T-7008]
- **required_validations**: [lint, unit, integration]
- **description**: MCP 엔드포인트 + 테스트 생성 + 문서화 전체 동작

---

## 14. 전체 로드맵 요약

| Phase | 기능 | 대상 | 예상 기간 |
|-------|------|------|----------|
| **1** ✅ | GitHub App | All | 완료 |
| **2** ✅ | 통합 프레임워크 + Discord | All | 완료 |
| **3** 🟡 | 팀 관리 + 클라이언트 포털 | Startup, Agency | API 완료, UI 진행중 |
| **4** 🟡 | 타임트래킹 + 감사 로그 | Agency, Enterprise | API 완료, UI 진행중 |
| **5** ✅ | SSO/SAML | Enterprise | 완료 (UI 제외) |
| **6** | 템플릿 + 화이트라벨 | Agency | 선택적 |
| **7** | Self-Documenting Platform | All (핵심) | 3-4주 |

---

## 14. 검증 체크리스트

### Phase 3 완료 조건
- [ ] 팀 생성/수정/삭제 API
- [ ] 멤버 초대/역할 변경/제거
- [ ] RLS 기반 팀 격리
- [ ] 대시보드 UI
- [ ] 테스트 커버리지 80%+

### Phase 4 완료 조건
- [ ] 활동 자동 수집
- [ ] 감사 로그 기록
- [ ] 사용량 리포트 API
- [ ] 감사 로그 내보내기

### Phase 5 완료 조건
- [x] Google SSO 동작
- [x] Microsoft SSO 동작
- [x] JIT 프로비저닝
- [ ] SSO 설정 UI (프론트엔드 - 별도 작업 필요)

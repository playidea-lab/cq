# C4 Plan Mode (Enhanced)

기획 문서를 해석하고, 구조화된 인터뷰를 통해 C4 태스크를 생성합니다.

## Instructions

### Phase 0: 상태 확인

```
status = mcp__c4__c4_status()
```

1. 현재 상태 확인
2. 상태에 따른 처리:
   - `INIT`: 계획 진행 가능
   - `PLAN`: "이미 계획 모드입니다. 계속 진행하시겠습니까?" 질문
   - `EXECUTE/CHECKPOINT`: "실행 중입니다. 계획을 수정하면 현재 진행이 초기화됩니다. 계속하시겠습니까?" 경고
   - `COMPLETE`: "완료된 프로젝트입니다. 새 계획을 시작하시겠습니까?" 질문

---

### Phase 1: 기획 문서 스캔

프로젝트 루트와 `docs/` 폴더에서 기획 문서를 찾습니다.

**스캔 대상:**
- `*.md` (PRD, requirements, spec 등)
- `docs/*.md`
- `docs/**/*.md`

**실행:**
```
1. ls docs/ 또는 프로젝트 루트 확인
2. 기획 문서로 보이는 파일 식별:
   - PRD, prd, 기획, requirements, spec, plan 등 키워드 포함
   - 크기가 1KB 이상인 .md 파일
3. 발견된 문서 목록 출력
```

**출력 예시:**
```
📄 기획 문서 발견:
  - docs/PRD.md (11KB) - 제품 요구사항 문서
  - docs/API_SPEC.md (5KB) - API 스펙
```

---

### Phase 2: 문서 해석

각 기획 문서를 읽고 핵심 정보를 추출합니다.

**추출 항목:**
1. **프로젝트 개요**: 이름, 목표, 배경
2. **핵심 기능**: 주요 기능 목록
3. **기술 스택**: 언어, 프레임워크, 라이브러리
4. **개발 로드맵**: Phase/단계별 계획
5. **아키텍처**: 컴포넌트 구조, 데이터 흐름

**추출 시 주의:**
- `- [ ]` 체크리스트 → 잠재적 태스크
- `Phase N:` 또는 `단계 N:` → 체크포인트 후보
- 기술명 언급 → 기술 스택

**출력 예시:**
```
📋 문서 분석 결과:

프로젝트: AirSign - 허공 손끝 서명 인식 시스템

핵심 기능:
  1. 실시간 손끝 추적 (MediaPipe Hands)
  2. 펜업/펜다운 인식 (속도 기반)
  3. 필압 시뮬레이션
  4. 캘리그래피 렌더링
  5. 서명 검증 (DTW → Siamese)

기술 스택:
  - MediaPipe Hands
  - Canvas 2D
  - JavaScript

개발 로드맵:
  - Phase 1: 프로토타입 (8개 항목)
  - Phase 2: 검증 시스템 (5개 항목)
  - Phase 3: SDK 배포 (5개 항목)
  - Phase 4: 고도화 (4개 항목)

잠재적 태스크: 22개 식별됨
```

---

### Phase 2.5: Discovery - 도메인 감지 및 요구사항 수집

**목표**: 프로젝트 도메인을 감지하고, EARS 패턴 기반의 구조화된 요구사항을 수집합니다.

#### 2.5.1 도메인 자동 감지

프로젝트 구조를 분석하여 도메인을 추론합니다.

**감지 규칙:**
| 도메인 | 감지 조건 |
|--------|----------|
| `web-frontend` | `package.json` + (react\|vue\|angular\|svelte) |
| `web-backend` | `package.json` + (express\|fastify\|nest) 또는 `pyproject.toml` + (fastapi\|flask\|django) |
| `ml-dl` | `pyproject.toml` + (torch\|tensorflow\|jax) 또는 `*.ipynb` 존재 |
| `mobile-app` | `pubspec.yaml` (Flutter) 또는 `react-native` |
| `infra` | `*.tf` (Terraform) 또는 `docker-compose.yml` |
| `library` | `setup.py` 또는 `pyproject.toml` (build-system 섹션) |

**도메인 확인 질문:**
```python
AskUserQuestion(questions=[
    {
        "question": f"프로젝트 도메인이 [{detected_domain}]으로 감지되었습니다. 맞나요?",
        "header": "도메인",
        "options": [
            {"label": f"{detected_domain} (감지됨)", "description": "자동 감지된 도메인"},
            {"label": "웹 프론트엔드", "description": "React, Vue 등 프론트엔드"},
            {"label": "웹 백엔드", "description": "FastAPI, Express 등 API 서버"},
            {"label": "ML/DL", "description": "PyTorch, TensorFlow 등 머신러닝"},
            {"label": "모바일 앱", "description": "Flutter, React Native 등"},
            {"label": "인프라", "description": "Terraform, Docker 등 DevOps"}
        ],
        "multiSelect": True  # 복수 선택 시 fullstack 처리
    }
])
```

---

#### 2.5.2 EARS 패턴 기반 요구사항 수집

**EARS (Easy Approach to Requirements Syntax)** 패턴을 사용하여 구조화된 요구사항을 수집합니다.

##### EARS 5가지 패턴

| 패턴 | 형식 | 예시 |
|------|------|------|
| **Ubiquitous** | "시스템은 ~해야 한다" | "시스템은 사용자 데이터를 암호화해야 한다" |
| **Event-Driven** | "~할 때, 시스템은 ~해야 한다" | "사용자가 로그인 폼을 제출할 때, 시스템은 자격 증명을 검증해야 한다" |
| **State-Driven** | "~하는 동안, 시스템은 ~해야 한다" | "데이터 로딩 중, 시스템은 스피너를 표시해야 한다" |
| **Optional** | "~기능이 있으면, 시스템은 ~해야 한다" | "다크 모드가 활성화되면, 시스템은 다크 테마를 사용해야 한다" |
| **Unwanted** | "~하면, 시스템은 ~해야 한다" | "자격 증명이 잘못되면, 시스템은 에러 메시지를 표시해야 한다" |

##### 요구사항 인터뷰 플로우

**1. 핵심 기능 식별:**
```python
# 사용자가 명시한 기능 = 반드시 상세화
# AI가 중요하다고 판단한 기능 = 질문으로 확인
AskUserQuestion(questions=[
    {
        "question": "구현할 핵심 기능을 선택해주세요",
        "header": "핵심기능",
        "options": [
            {"label": "인증/로그인", "description": "사용자 인증 및 세션 관리"},
            {"label": "대시보드", "description": "데이터 시각화 및 모니터링"},
            {"label": "CRUD 기능", "description": "데이터 생성/조회/수정/삭제"},
            {"label": "파일 업로드", "description": "파일 저장 및 관리"}
        ],
        "multiSelect": True
    }
])
```

**2. 기능별 상세화 (EARS 패턴 적용):**

각 선택된 기능에 대해 후속 질문으로 상세화합니다.

```
사용자: "로그인 기능 필요해요. 실패하면 에러 보여주고."
    ↓
AI 분석: Event-Driven + Unwanted 패턴 식별
    ↓
EARS 변환:
  REQ-001 (event-driven): "사용자가 로그인 폼을 제출할 때, 시스템은 자격 증명을 검증해야 한다"
  REQ-002 (unwanted): "자격 증명이 잘못되면, 시스템은 에러 메시지를 표시해야 한다"
```

**3. 엣지 케이스 확인:**
```python
# AI가 판단하여 중요한 엣지 케이스 질문
AskUserQuestion(questions=[
    {
        "question": "비밀번호 오류 시 처리는 어떻게 할까요?",
        "header": "보안정책",
        "options": [
            {"label": "5회 실패 시 계정 잠금 (권장)", "description": "보안 강화"},
            {"label": "제한 없음", "description": "단순 처리"},
            {"label": "CAPTCHA 표시", "description": "봇 차단"}
        ],
        "multiSelect": False
    }
])
```

---

#### 2.5.3 도메인별 확장 질문 템플릿

##### 웹 프론트엔드 (`web-frontend`)
```python
AskUserQuestion(questions=[
    {
        "question": "어떤 UI 기능이 필요한가요?",
        "header": "UI기능",
        "options": [
            {"label": "인증/로그인", "description": "로그인, 회원가입, 세션"},
            {"label": "대시보드", "description": "차트, 통계, 모니터링"},
            {"label": "폼/입력", "description": "데이터 입력 및 검증"},
            {"label": "리스트/테이블", "description": "데이터 목록 표시"}
        ],
        "multiSelect": True
    },
    {
        "question": "어떤 인터랙션이 필요한가요?",
        "header": "인터랙션",
        "options": [
            {"label": "없음", "description": "기본 클릭/입력만"},
            {"label": "드래그앤드롭", "description": "항목 재배열"},
            {"label": "실시간 업데이트", "description": "WebSocket, SSE"},
            {"label": "제스처", "description": "스와이프, 핀치 등"}
        ],
        "multiSelect": True
    }
])
```

##### 웹 백엔드 (`web-backend`)
```python
AskUserQuestion(questions=[
    {
        "question": "어떤 API 기능이 필요한가요?",
        "header": "API기능",
        "options": [
            {"label": "REST CRUD", "description": "기본 리소스 관리"},
            {"label": "인증 API", "description": "JWT, OAuth"},
            {"label": "파일 업로드", "description": "S3, 로컬 저장"},
            {"label": "실시간 통신", "description": "WebSocket, SSE"}
        ],
        "multiSelect": True
    },
    {
        "question": "데이터베이스 유형을 선택해주세요",
        "header": "데이터베이스",
        "options": [
            {"label": "PostgreSQL (권장)", "description": "관계형, 안정적"},
            {"label": "SQLite", "description": "단순, 파일 기반"},
            {"label": "MongoDB", "description": "NoSQL, 유연한 스키마"}
        ],
        "multiSelect": False
    }
])
```

##### ML/DL (`ml-dl`)
```python
AskUserQuestion(questions=[
    {
        "question": "어떤 ML 태스크인가요?",
        "header": "ML태스크",
        "options": [
            {"label": "분류 (Classification)", "description": "이미지, 텍스트 분류"},
            {"label": "회귀 (Regression)", "description": "수치 예측"},
            {"label": "생성 (Generative)", "description": "이미지, 텍스트 생성"},
            {"label": "추천 (Recommendation)", "description": "개인화 추천"}
        ],
        "multiSelect": False
    },
    {
        "question": "실험 관리 도구를 선택해주세요",
        "header": "실험관리",
        "options": [
            {"label": "MLflow (권장)", "description": "실험 추적, 모델 관리"},
            {"label": "Weights & Biases", "description": "시각화, 협업"},
            {"label": "없음", "description": "수동 관리"}
        ],
        "multiSelect": False
    }
])
```

---

#### 2.5.4 명세 저장 (MCP 도구 사용)

인터뷰 완료 후, EARS 패턴으로 변환된 요구사항을 저장합니다.

```python
# 명세 저장 예시
mcp__c4__c4_save_spec(
    feature="user-auth",
    domain="web-frontend",
    description="사용자 인증 기능",
    requirements=[
        {
            "id": "REQ-001",
            "pattern": "event-driven",
            "text": "When user submits login form, the system shall validate credentials"
        },
        {
            "id": "REQ-002",
            "pattern": "unwanted",
            "text": "If credentials are invalid, the system shall display error message"
        },
        {
            "id": "REQ-003",
            "pattern": "unwanted",
            "text": "If login fails 5 times, the system shall lock account for 30 minutes"
        }
    ]
)
```

**저장 위치:** `.c4/specs/{feature}/requirements.yaml`

```yaml
# .c4/specs/user-auth/requirements.yaml
feature: user-auth
domain: web-frontend
description: 사용자 인증 기능
requirements:
  - id: REQ-001
    pattern: event-driven
    text: "When user submits login form, the system shall validate credentials"
  - id: REQ-002
    pattern: unwanted
    text: "If credentials are invalid, the system shall display error message"
```

---

#### 2.5.5 Discovery 완료

모든 기능의 명세가 수집되면 DESIGN 단계로 전환합니다.

```python
# 저장된 명세 확인
specs = mcp__c4__c4_list_specs()
print(f"📋 {specs['count']}개 기능 명세 저장됨:")
for feature in specs['features']:
    print(f"  - {feature['feature']} ({feature['domain']})")

# Discovery 완료 → DESIGN 상태로 전환
result = mcp__c4__c4_discovery_complete()
if result['success']:
    print(f"✅ Discovery 완료! {result['previous_status']} → {result['new_status']}")
```

---

### Phase 2.6: Design - 아키텍처 설계 및 결정

**목표**: 수집된 요구사항을 바탕으로 아키텍처를 설계하고, 주요 기술적 결정을 기록합니다.

#### 2.6.1 아키텍처 옵션 제시

각 핵심 기능에 대해 아키텍처 옵션을 제시합니다.

**옵션 구성 요소:**
- `id`: 옵션 식별자 (option-a, option-b 등)
- `name`: 옵션 이름
- `description`: 상세 설명
- `complexity`: 복잡도 (low, medium, high)
- `pros`: 장점 목록
- `cons`: 단점 목록
- `recommended`: 권장 여부

```python
# 예: 인증 기능 아키텍처 옵션
AskUserQuestion(questions=[
    {
        "question": "인증 시스템의 아키텍처를 선택해주세요",
        "header": "인증아키텍처",
        "options": [
            {"label": "Session-based (권장)", "description": "서버 세션 + 쿠키, 단순하고 안전"},
            {"label": "JWT Token", "description": "Stateless, 확장성 좋음, 토큰 관리 필요"},
            {"label": "OAuth 2.0", "description": "소셜 로그인, 복잡도 높음"}
        ],
        "multiSelect": False
    }
])
```

**도메인별 아키텍처 옵션 템플릿:**

##### 웹 프론트엔드 (`web-frontend`)
```python
# 상태 관리 아키텍처
AskUserQuestion(questions=[
    {
        "question": "상태 관리 패턴을 선택해주세요",
        "header": "상태관리",
        "options": [
            {"label": "Context API (권장)", "description": "React 기본, 소규모 프로젝트"},
            {"label": "Redux", "description": "대규모, 복잡한 상태"},
            {"label": "Zustand", "description": "경량, 간단한 API"}
        ],
        "multiSelect": False
    },
    {
        "question": "폴더 구조를 선택해주세요",
        "header": "폴더구조",
        "options": [
            {"label": "Feature-based (권장)", "description": "기능별 그룹화"},
            {"label": "Type-based", "description": "컴포넌트/훅/유틸 분리"},
            {"label": "Atomic Design", "description": "atoms/molecules/organisms"}
        ],
        "multiSelect": False
    }
])
```

##### 웹 백엔드 (`web-backend`)
```python
# API 아키텍처
AskUserQuestion(questions=[
    {
        "question": "API 아키텍처를 선택해주세요",
        "header": "API아키텍처",
        "options": [
            {"label": "REST (권장)", "description": "표준적, 캐싱 용이"},
            {"label": "GraphQL", "description": "유연한 쿼리, 클라이언트 주도"},
            {"label": "gRPC", "description": "고성능, 마이크로서비스"}
        ],
        "multiSelect": False
    },
    {
        "question": "데이터베이스 패턴을 선택해주세요",
        "header": "DB패턴",
        "options": [
            {"label": "Repository Pattern (권장)", "description": "추상화, 테스트 용이"},
            {"label": "Active Record", "description": "단순, ORM 직접 사용"},
            {"label": "CQRS", "description": "읽기/쓰기 분리, 복잡"}
        ],
        "multiSelect": False
    }
])
```

##### ML/DL (`ml-dl`)
```python
# ML 파이프라인 아키텍처
AskUserQuestion(questions=[
    {
        "question": "학습 파이프라인 구조를 선택해주세요",
        "header": "파이프라인",
        "options": [
            {"label": "단일 스크립트 (권장)", "description": "단순, 프로토타입"},
            {"label": "Hydra + Config", "description": "설정 분리, 실험 관리"},
            {"label": "Lightning", "description": "구조화, 보일러플레이트 감소"}
        ],
        "multiSelect": False
    },
    {
        "question": "서빙 아키텍처를 선택해주세요",
        "header": "서빙",
        "options": [
            {"label": "FastAPI (권장)", "description": "단순, 빠른 배포"},
            {"label": "TorchServe", "description": "PyTorch 네이티브"},
            {"label": "Triton", "description": "고성능, GPU 최적화"}
        ],
        "multiSelect": False
    }
])
```

---

#### 2.6.2 컴포넌트 설계

선택된 아키텍처를 기반으로 주요 컴포넌트를 정의합니다.

**컴포넌트 정보:**
- `name`: 컴포넌트 이름
- `type`: 유형 (frontend, backend, service, database 등)
- `description`: 역할 설명
- `responsibilities`: 책임 목록
- `dependencies`: 의존성
- `interfaces`: 제공하는 인터페이스

```python
# 컴포넌트 정의 예시
components = [
    {
        "name": "AuthService",
        "type": "service",
        "description": "사용자 인증 및 세션 관리",
        "responsibilities": [
            "사용자 로그인/로그아웃 처리",
            "세션 토큰 발급 및 검증",
            "비밀번호 해싱 및 검증"
        ],
        "dependencies": ["UserRepository", "SessionStore"],
        "interfaces": ["login()", "logout()", "validateSession()"]
    },
    {
        "name": "UserRepository",
        "type": "repository",
        "description": "사용자 데이터 접근 계층",
        "responsibilities": [
            "사용자 CRUD 작업",
            "이메일로 사용자 조회"
        ],
        "dependencies": ["Database"],
        "interfaces": ["findByEmail()", "create()", "update()"]
    }
]
```

---

#### 2.6.3 데이터 흐름 및 Mermaid 다이어그램 생성

컴포넌트 간 데이터 흐름을 정의하고 시각화합니다.

**데이터 흐름 정의:**
```python
data_flows = [
    {
        "from_component": "Client",
        "to_component": "AuthController",
        "action": "POST /api/login",
        "data": "email, password"
    },
    {
        "from_component": "AuthController",
        "to_component": "AuthService",
        "action": "authenticate()",
        "data": "credentials"
    },
    {
        "from_component": "AuthService",
        "to_component": "UserRepository",
        "action": "findByEmail()",
        "data": "email"
    }
]
```

**Mermaid 다이어그램 생성:**
```
sequenceDiagram
    participant C as Client
    participant AC as AuthController
    participant AS as AuthService
    participant UR as UserRepository
    participant DB as Database

    C->>AC: POST /api/login (email, password)
    AC->>AS: authenticate(credentials)
    AS->>UR: findByEmail(email)
    UR->>DB: SELECT * FROM users
    DB-->>UR: User data
    UR-->>AS: User entity
    AS->>AS: validatePassword()
    AS->>AS: createSession()
    AS-->>AC: SessionToken
    AC-->>C: 200 OK + Set-Cookie
```

---

#### 2.6.4 설계 결정 (Design Decisions) 기록

중요한 기술적 결정을 기록합니다.

**결정 기록 요소:**
- `id`: 결정 식별자 (DEC-001 등)
- `question`: 결정해야 할 질문
- `decision`: 결정 내용
- `rationale`: 결정 이유
- `alternatives_considered`: 고려한 대안들

```python
# 설계 결정 예시
decisions = [
    {
        "id": "DEC-001",
        "question": "어떤 인증 방식을 사용할 것인가?",
        "decision": "Session-based 인증",
        "rationale": "단순한 구현, 서버 측 세션 관리로 보안 강화, 프로젝트 규모에 적합",
        "alternatives_considered": ["JWT Token", "OAuth 2.0"]
    },
    {
        "id": "DEC-002",
        "question": "비밀번호 해싱 알고리즘은?",
        "decision": "bcrypt (cost factor 12)",
        "rationale": "업계 표준, 충분한 보안성, 적절한 성능",
        "alternatives_considered": ["Argon2", "scrypt"]
    }
]
```

---

#### 2.6.5 설계 저장 (MCP 도구 사용)

설계를 저장합니다.

```python
# 설계 저장
mcp__c4__c4_save_design(
    feature="user-auth",
    domain="web-backend",
    description="사용자 인증 시스템 설계",
    options=[
        {
            "id": "option-a",
            "name": "Session-based Auth",
            "description": "서버 세션 + HTTP-only 쿠키",
            "complexity": "low",
            "pros": ["단순한 구현", "서버 측 세션 관리", "CSRF 보호 용이"],
            "cons": ["서버 메모리 사용", "수평 확장 시 세션 공유 필요"],
            "recommended": True
        },
        {
            "id": "option-b",
            "name": "JWT Token Auth",
            "description": "Stateless JWT 토큰",
            "complexity": "medium",
            "pros": ["Stateless", "수평 확장 용이"],
            "cons": ["토큰 만료 관리", "토큰 크기"]
        }
    ],
    selected_option="option-a",
    components=[
        {
            "name": "AuthService",
            "type": "service",
            "description": "인증 비즈니스 로직",
            "responsibilities": ["로그인/로그아웃", "세션 관리"],
            "dependencies": ["UserRepository", "SessionStore"]
        }
    ],
    decisions=[
        {
            "id": "DEC-001",
            "question": "인증 방식?",
            "decision": "Session-based",
            "rationale": "프로젝트 규모에 적합, 단순한 구현"
        }
    ],
    mermaid_diagram="""sequenceDiagram
    Client->>AuthController: POST /login
    AuthController->>AuthService: authenticate()
    AuthService->>UserRepository: findByEmail()
    UserRepository-->>AuthService: User
    AuthService-->>AuthController: SessionToken
    AuthController-->>Client: Set-Cookie"""
)
```

**저장 위치:** `.c4/specs/{feature}/design.yaml`, `.c4/specs/{feature}/design.md`

---

#### 2.6.6 설계 확인 및 승인

사용자에게 설계를 확인받습니다.

```python
# 저장된 설계 조회
designs = mcp__c4__c4_list_designs()
print(f"📐 {designs['count']}개 기능 설계 완료:")
for d in designs['designs']:
    status = "✅ 선택됨" if d.get('selected_option') else "⚠️ 미선택"
    print(f"  - {d['feature']} ({d['domain']}) - {status}")

# 설계 확인 질문
AskUserQuestion(questions=[
    {
        "question": "위 설계로 진행할까요?",
        "header": "설계확인",
        "options": [
            {"label": "진행 (권장)", "description": "태스크 생성으로 이동"},
            {"label": "수정 필요", "description": "설계 재검토"},
            {"label": "다시 시작", "description": "Discovery부터 재시작"}
        ],
        "multiSelect": False
    }
])
```

---

#### 2.6.7 Design 완료

설계 승인 후 PLAN 상태로 전환합니다.

```python
# Design 완료 → PLAN 상태로 전환
result = mcp__c4__c4_design_complete()
if result['success']:
    print(f"✅ Design 완료! {result['previous_status']} → {result['new_status']}")
    print(f"📐 승인된 설계: {result['designs_count']}개")
else:
    print(f"❌ 오류: {result['error']}")
    # 일반적인 오류:
    # - "No design specifications found": 설계가 저장되지 않음
    # - "without selected option": 아키텍처 옵션이 선택되지 않음
```

---

### Phase 3: 구조화된 인터뷰 (개발 환경)

문서에서 확인되지 않은 **개발 환경** 정보를 **AskUserQuestion** 도구로 질문합니다.

**중요**: 각 카테고리별로 AskUserQuestion을 호출하여 선택지를 제공합니다.

#### 3.1 개발 환경 질문

```python
AskUserQuestion(questions=[
    {
        "question": "프로젝트에서 어떤 언어를 사용할까요?",
        "header": "언어",
        "options": [
            {"label": "TypeScript (권장)", "description": "타입 안정성, IDE 지원 우수"},
            {"label": "Vanilla JavaScript", "description": "단순하고 빠른 시작"},
            {"label": "Python", "description": "백엔드/ML 프로젝트"}
        ],
        "multiSelect": False
    },
    {
        "question": "빌드 도구를 선택해주세요",
        "header": "빌드",
        "options": [
            {"label": "Vite (권장)", "description": "빠른 개발, HMR 지원"},
            {"label": "없음", "description": "단순 프로젝트, CDN 사용"},
            {"label": "Webpack", "description": "복잡한 설정 필요 시"}
        ],
        "multiSelect": False
    },
    {
        "question": "패키지 매니저를 선택해주세요",
        "header": "패키지",
        "options": [
            {"label": "pnpm (권장)", "description": "빠르고 디스크 효율적"},
            {"label": "npm", "description": "기본 패키지 매니저"},
            {"label": "uv (Python)", "description": "Python 프로젝트용"}
        ],
        "multiSelect": False
    }
])
```

#### 3.2 테스트 전략 질문

```python
AskUserQuestion(questions=[
    {
        "question": "유닛 테스트 프레임워크를 선택해주세요",
        "header": "유닛테스트",
        "options": [
            {"label": "Vitest (권장)", "description": "Vite와 호환, 빠름"},
            {"label": "Jest", "description": "React 프로젝트 표준"},
            {"label": "pytest (Python)", "description": "Python 프로젝트용"},
            {"label": "필요 없음", "description": "테스트 스킵"}
        ],
        "multiSelect": False
    },
    {
        "question": "E2E 테스트 프레임워크를 선택해주세요",
        "header": "E2E",
        "options": [
            {"label": "필요 없음", "description": "유닛 테스트만"},
            {"label": "Playwright (권장)", "description": "빠르고 안정적"},
            {"label": "Cypress", "description": "직관적 UI"}
        ],
        "multiSelect": False
    }
])
```

#### 3.3 품질 기준 질문

```python
AskUserQuestion(questions=[
    {
        "question": "코드 품질 도구를 선택해주세요 (복수 선택 가능)",
        "header": "품질도구",
        "options": [
            {"label": "ESLint + Prettier (권장)", "description": "린팅 + 포매팅"},
            {"label": "ESLint만", "description": "린팅만"},
            {"label": "Ruff (Python)", "description": "Python 린터/포매터"},
            {"label": "없음", "description": "품질 도구 스킵"}
        ],
        "multiSelect": True
    }
])
```

#### 3.4 C4 워크플로우 질문

```python
AskUserQuestion(questions=[
    {
        "question": "체크포인트를 어디에 설정할까요?",
        "header": "체크포인트",
        "options": [
            {"label": "Phase별 (권장)", "description": "각 Phase 완료 시 Supervisor 리뷰"},
            {"label": "기능별", "description": "주요 기능마다 리뷰"},
            {"label": "없음", "description": "마지막에 한번만 리뷰"}
        ],
        "multiSelect": False
    },
    {
        "question": "태스크 크기를 어떻게 설정할까요?",
        "header": "태스크크기",
        "options": [
            {"label": "PRD 그대로 (권장)", "description": "문서의 체크리스트 항목 그대로"},
            {"label": "더 작게", "description": "세부 단계로 분할"},
            {"label": "더 크게", "description": "관련 항목 합침"}
        ],
        "multiSelect": False
    },
    {
        "question": "자동 실행 범위는?",
        "header": "실행범위",
        "options": [
            {"label": "Phase 1만", "description": "프로토타입까지만 자동"},
            {"label": "Phase 1~2", "description": "기본 기능까지"},
            {"label": "전체", "description": "모든 Phase 자동 실행"}
        ],
        "multiSelect": False
    }
])

---

### Phase 4: 태스크 생성

인터뷰 결과를 반영하여 C4 태스크를 생성합니다.

**생성 규칙:**
1. PRD의 체크리스트 항목 → 개별 태스크
2. `scope`는 영향받는 파일/디렉토리
3. **`dod`는 반드시 구체적이고 검증 가능하게 작성** (아래 DoD 작성 원칙 참조)
4. `dependencies`는 선후관계 고려
5. `validations`는 인터뷰에서 결정된 도구

---

#### ⚠️ DoD (Definition of Done) 작성 원칙 - 필수!

**모든 태스크는 명확한 DoD가 있어야 합니다.** DoD가 불분명하면 Worker가 완료 여부를 판단할 수 없습니다.

**좋은 DoD의 조건:**
1. **검증 가능**: "~가 동작한다", "~를 반환한다", "~테스트가 통과한다"
2. **구체적**: 모호한 표현 금지 ("개선한다", "최적화한다" ❌)
3. **독립적**: 이 태스크만으로 확인 가능 (다른 태스크 의존 ❌)

**DoD 작성 예시:**

| ❌ 나쁜 DoD | ✅ 좋은 DoD |
|------------|------------|
| "로그인 기능 구현" | "이메일/비밀번호로 로그인 시 JWT 토큰 반환, 잘못된 비밀번호 시 401 에러" |
| "API 최적화" | "GET /users 응답시간 500ms → 100ms 이하, 기존 테스트 통과" |
| "버그 수정" | "null 입력 시 에러 대신 빈 배열 반환, 관련 테스트 추가" |
| "UI 개선" | "버튼 클릭 시 로딩 스피너 표시, 완료 시 성공 메시지" |
| "코드 정리" | "미사용 함수 3개 삭제, lint 에러 0개" |

**DoD 체크리스트:**
- [ ] Worker가 읽고 바로 구현 가능한가?
- [ ] 완료 여부를 객관적으로 판단 가능한가?
- [ ] 테스트나 수동 확인으로 검증 가능한가?

---

**태스크 생성 (MCP 도구 사용):**
```javascript
mcp__c4__c4_add_todo({
  task_id: "T-001",
  title: "MediaPipe Hands 연동",
  scope: "src/HandTracker.js",
  dod: "1) HandTracker 클래스 구현, 2) startTracking() 호출 시 웹캠 스트림 시작, 3) onFrame 콜백에서 검지 손끝(landmark 8) 좌표 {x, y, z} 반환, 4) stopTracking() 호출 시 리소스 해제"
})
```

**또는 CLI 사용:**
```bash
uv run --directory $C4_INSTALL_DIR c4 add-task \
  --task-id "T-001" \
  --title "MediaPipe Hands 연동" \
  --scope "src/HandTracker.js" \
  --dod "1) HandTracker 클래스 구현, 2) startTracking() 호출 시 웹캠 스트림 시작, 3) onFrame 콜백에서 검지 손끝(landmark 8) 좌표 {x, y, z} 반환, 4) stopTracking() 호출 시 리소스 해제"
```

**체크포인트 설정:**
`.c4/config.yaml`에 추가:
```yaml
checkpoints:
  - id: CP-001
    description: "Phase 1 프로토타입 완료"
    required_tasks: [T-001, T-002, ..., T-008]
    required_validations: [lint, unit]
```

**검증 명령 설정 (인터뷰 결과 반영):**
```yaml
validation:
  commands:
    lint: "npm run lint"      # 또는 "pnpm lint", "uv run ruff check ."
    unit: "npm test"          # 또는 "pnpm test", "uv run pytest"
    e2e: "npm run e2e"        # 선택한 경우만
  required: [lint, unit]      # 필수 검증
```

---

### Phase 5: 계획 확정

생성된 계획을 요약하고 확인합니다.

**출력:**
```
✅ C4 계획 생성 완료

📊 요약:
  - 태스크: 22개 (Phase 1: 8개, Phase 2: 5개, ...)
  - 체크포인트: 4개
  - 예상 검증: lint, unit

📋 Phase 1 태스크:
  T-001: MediaPipe Hands 연동
  T-002: 손끝 좌표 추출
  T-003: 속도 기반 펜업/펜다운
  ...

🚀 다음 단계:
  /c4-run    - 실행 시작
  /c4-status - 상태 확인
```

**확인:**
- "진행" → `/c4-run` 안내
- "수정" → 어떤 부분 수정할지 질문
- "취소" → 태스크 삭제 후 재시작

---

## 빠른 참조

```
/c4-plan              # 전체 워크플로우 시작
/c4-plan --scan       # 문서 스캔만
/c4-plan --interview  # 인터뷰만 다시
```

## 관련 명령어

- `/c4-add-task` - 개별 태스크 추가
- `/c4-run` - 실행 시작
- `/c4-status` - 상태 확인

# C4 Plan Mode (Enhanced)

프로젝트 현황을 파악하고, 사용자 선택에 따라 계획을 수립/수정합니다.

---

## ⚠️ 핵심 원칙: 계획 승인 전 코드 작성 금지

> **절대 규칙**: 사용자가 계획을 명시적으로 승인하기 전까지 코드를 작성하지 않습니다.

```
❌ 금지:
- 계획 설명 중 코드 작성 시작
- "일단 해보면서 조정하자" 식의 접근
- 사용자 확인 없이 태스크 실행

✅ 필수:
- 계획 요약 → 사용자 확인 → "진행" 선택 → 코드 작성
- 불명확한 요구사항 → 질문으로 확인 → 합의 후 진행
- 변경 요청 시 → 계획 수정 → 재확인 → 진행
```

**위반 시**: Worker가 코드를 작성해도 리뷰에서 거부됩니다.

## Instructions

### Phase 0: 현황 파악 (Context Display)

**먼저 전체 현황을 수집하고 출력합니다.**

#### 0.1 데이터 수집

```python
# 1. 상태 확인
status = mcp__c4__c4_status()

# 2. 기존 Specs 확인
specs = mcp__c4__c4_list_specs()

# 3. 기존 Designs 확인
designs = mcp__c4__c4_list_designs()

# 4. 기획 문서 스캔
# Glob으로 docs/**/*.md 검색
# PRD, requirements, spec, plan 키워드 포함 파일 식별

# 5. Lighthouse 현황
lighthouses = mcp__c4__c4_lighthouse(action="list")
```

#### 0.2 현황 출력 (Enhanced)

**프로젝트 개요와 상세 컨텍스트를 풍부하게 출력합니다.**

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  {project_name} - {project_description}                                      ║
║  "{프로젝트 한줄 설명}"                                                       ║
╚══════════════════════════════════════════════════════════════════════════════╝

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🎯 프로젝트 개요
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  프로젝트:   {project_id}
  설명:       {README.md에서 추출한 프로젝트 설명}
  도메인:     {domain}
  라이선스:   {라이선스 정보}

  핵심 기능:
  ┌────────────────────────────────────────────────────────────────────────┐
  │ • {기능1}    {설명1}                                                   │
  │ • {기능2}    {설명2}                                                   │
  │ • ...                                                                  │
  └────────────────────────────────────────────────────────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📊 현재 상태
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  워크플로우:  INIT → DISCOVERY → DESIGN → PLAN → [EXECUTE] ↔ CHECKPOINT → COMPLETE
                                                      ↑
                                                  현재 위치 표시

  상태:        {status_icon} {status} ({execution_mode})
  Supervisor:  {supervisor 상태}
  워커:        {workers_count}개 ({활성/유휴/연결끊김 상세})

  ┌─────────────────────────────────────────────────────────────────┐
  │  진행률:  {프로그레스바}  {percentage}%                         │
  │           완료 {done} / 전체 {total} 태스크                     │
  │                                                                 │
  │  ✅ 완료: {done}    🔄 진행중: {in_progress}    ⏳ 대기: {pending}    ❌ 블록: {blocked}
  └─────────────────────────────────────────────────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📈 태스크 의존성 그래프
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[T-xxx] {시리즈 이름} - {시리즈 설명}
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│  ✅ T-xxx ────→ 🔄 T-xxx ────→ ✅ T-xxx ────→ ⏳ T-xxx                      │
│   {title}        {title}        {title}        {title}                      │
│   (완료)         (진행중)        (완료)         (대기)                       │
│                     │                                                       │
│                     ↓                                                       │
│                 ⏳ T-xxx                                                     │
│                  {title}                                                    │
│                  (의존: T-xxx)                                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘

  범례: ✅ 완료({done})  🔄 진행중({in_progress})  ⏳ 대기({pending})  ❌ 블록({blocked})

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📋 기존 Specifications ({specs_count}개)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

┌─────────────────────────────────────────────────────────────────────────────┐
│ 📦 {feature_name}                                                           │
│    Domain: {domain}                                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ 🎯 비전: {description}                                                       │
│                                                                             │
│ 📝 요구사항 ({requirements_count}개, EARS 패턴):                             │
│    ┌──────────────┬────────────────────────────────────────────────────┐   │
│    │ ubiquitous   │ {요약된 ubiquitous 요구사항들}                     │   │
│    │ event-driven │ {요약된 event-driven 요구사항들}                   │   │
│    │ state-driven │ {요약된 state-driven 요구사항들}                   │   │
│    │ optional     │ {요약된 optional 요구사항들}                       │   │
│    └──────────────┴────────────────────────────────────────────────────┘   │
│                                                                             │
│ ✅ 상태: {spec_status}                                                       │
└─────────────────────────────────────────────────────────────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📐 기존 Designs ({designs_count}개)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

┌─────────────────────────────────────────────────────────────────────────────┐
│ 🏗️ {feature_name}                                                           │
│    Domain: {domain}                                                         │
│    선택된 아키텍처: ✅ {selected_option}                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│ 🔍 아키텍처 옵션:                                                            │
│    ✅ {selected}: {description}  [선택됨]                                   │
│    ⚪ {other}: {description}                                                │
│                                                                             │
│ 🧩 컴포넌트 ({components_count}개):                                          │
│    {컴포넌트 간략 목록 및 관계도}                                            │
│                                                                             │
│ 📌 핵심 설계 결정 ({decisions_count}개):                                     │
│    {주요 결정사항 요약}                                                      │
│                                                                             │
│ ⚡ NFR: {성능/메모리/확장성 요약}                                            │
│                                                                             │
│ ✅ 상태: {design_status}                                                     │
└─────────────────────────────────────────────────────────────────────────────┘

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🏗️ Lighthouse (Contract-First TDD)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ┌─────────────────────────────────────────────────────────────────────────────┐
  │ Stubs: {stubs}개  Implemented: {impl}개  Deprecated: {depr}개              │
  │                                                                             │
  │ 📋 Active Stubs:                                                            │
  │   🔴 {name1} — {description1}     → T-LH-{name1}-0                         │
  │   🔴 {name2} — {description2}     → T-LH-{name2}-0                         │
  │                                                                             │
  │ ✅ Recently Implemented:                                                    │
  │   🟢 {name3} — {description3}                                               │
  └─────────────────────────────────────────────────────────────────────────────┘

데이터 수집: `lighthouses = mcp__c4__c4_lighthouse(action="list")`
Lighthouse가 없으면 이 섹션은 "(등록된 Lighthouse 없음)" 한 줄로 표시.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📄 기획 문서 (docs/)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  📂 docs/specs/ - 핵심 스펙 문서
     {파일 목록 및 간략 설명}

  📂 docs/{category}/ - {카테고리 설명}
     {파일 목록}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🛠️ 기술 스택
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  언어:       {language}
  패키지:     {package_manager}
  데이터베이스: {database}
  IDE 통합:   {platforms}
  검증:       {validation_tools}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

**정보 추출 소스:**
- 프로젝트 개요: `README.md` 파싱
- 기술 스택: `pyproject.toml`, `package.json` 등에서 추출
- 핵심 기능: README의 "주요 기능" 또는 "Features" 섹션
- 라이선스: `LICENSE` 또는 `LICENSE.md`

#### 0.3 의존성 그래프 렌더링 로직

```python
def render_dependency_graph(tasks, pending_ids):
    """
    태스크 의존성을 시각적으로 표현합니다.

    1. 대기 중인 태스크와 관련된 의존성 체인만 표시
    2. 루트 태스크(의존성 없음)부터 시작
    3. 상태별 아이콘: ✅완료 🔄진행 ⏳대기 ❌블록
    """
    # pending_ids에 있는 태스크들의 의존성 역추적
    # 트리 형태로 렌더링
    # 최근 완료된 태스크도 컨텍스트로 포함
```

---

### Phase 0.5: 행동 선택 (Action Selection)

현황을 확인한 후 사용자가 다음 행동을 선택합니다.

```python
AskUserQuestion(questions=[
    {
        "question": "무엇을 하시겠습니까?",
        "header": "작업선택",
        "options": [
            {"label": "새 기능 계획", "description": "Discovery → Design → Lighthouse → Tasks 전체 플로우"},
            {"label": "기존 계획 검토/수정", "description": "저장된 Spec/Design 상세 보기 및 수정"},
            {"label": "Lighthouse 관리", "description": "도구 계약 등록/조회/promote/삭제"},
            {"label": "태스크만 추가", "description": "기존 설계 기반 태스크 생성"},
            {"label": "현황만 확인", "description": "출력 후 종료"}
        ],
        "multiSelect": False
    }
])
```

#### 0.4 선택에 따른 분기

| 선택 | 다음 단계 |
|------|----------|
| **새 기능 계획** | → Phase 1 (문서 스캔)으로 이동 |
| **기존 계획 검토/수정** | → Phase R (검토/수정)로 이동 |
| **Lighthouse 관리** | → Phase L (Lighthouse 관리)로 이동 |
| **태스크만 추가** | → Phase 4 (태스크 생성)로 직행 |
| **현황만 확인** | → "현황 파악 완료" 메시지 후 종료 |

---

### Phase R: 기존 계획 검토/수정 (Review/Modify)

기존에 저장된 Spec 또는 Design을 검토하고 수정합니다.

#### R.1 대상 선택

```python
# specs와 designs 목록에서 선택
options = []
for spec in specs['features']:
    options.append({
        "label": f"{spec['feature']} (Spec)",
        "description": f"{spec['domain']} - {spec['requirements_count']} requirements"
    })
for design in designs['designs']:
    options.append({
        "label": f"{design['feature']} (Design)",
        "description": f"Option: {design['selected_option']}, {design['components_count']} components"
    })

AskUserQuestion(questions=[
    {
        "question": "어떤 것을 검토하시겠습니까?",
        "header": "대상선택",
        "options": options,
        "multiSelect": False
    }
])
```

#### R.2 상세 출력

선택한 Spec 또는 Design의 전체 내용을 출력합니다.

```python
# Spec 선택 시
spec = mcp__c4__c4_get_spec(feature=selected_feature)
print(f"""
📐 Spec: {spec['feature']}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

도메인: {spec['domain']}
설명: {spec['description']}

요구사항:
{각 requirement를 EARS 패턴과 함께 출력}
""")

# Design 선택 시
design = mcp__c4__c4_get_design(feature=selected_feature)
print(f"""
🏗️ Design: {design['feature']}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

선택된 옵션: {design['selected_option']}

컴포넌트:
{각 component 정보}

설계 결정:
{각 decision 정보}

다이어그램:
{mermaid_diagram if exists}
""")
```

#### R.3 수정 여부

```python
AskUserQuestion(questions=[
    {
        "question": "수정하시겠습니까?",
        "header": "수정여부",
        "options": [
            {"label": "요구사항 추가/수정", "description": "Spec의 requirements 변경"},
            {"label": "컴포넌트 변경", "description": "Design의 components 수정"},
            {"label": "아키텍처 결정 변경", "description": "Design의 decisions 수정"},
            {"label": "수정 없음", "description": "검토만 하고 종료"}
        ],
        "multiSelect": False
    }
])
```

#### R.4 수정 진행

선택에 따라 대화형으로 수정을 진행합니다.

- **요구사항 추가/수정**: EARS 패턴 인터뷰 → `c4_save_spec()` 호출
- **컴포넌트 변경**: 컴포넌트 정보 질문 → `c4_save_design()` 호출
- **아키텍처 결정 변경**: 결정 사항 질문 → `c4_save_design()` 호출

```python
# 수정 완료 후 저장
if is_spec:
    mcp__c4__c4_save_spec(
        feature=feature,
        domain=domain,
        requirements=updated_requirements,
        description=description
    )
else:
    mcp__c4__c4_save_design(
        feature=feature,
        domain=domain,
        components=updated_components,
        decisions=updated_decisions,
        ...
    )

print("✅ 수정이 저장되었습니다.")
```

---

### Phase 1: 기획 문서 스캔

> **진입 조건**: Phase 0.5에서 "새 기능 계획" 선택 시

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

#### 2.5.5 검증 요구사항 수집 (Verification Requirements)

**목표**: 대화 중 사용자가 언급한 검증 요구사항이나 도메인별 필수 검증을 수집합니다.

##### 검증 요구사항이 필요한 경우

1. **사용자 명시적 요청**:
   - "성능 검증 필요해요"
   - "API 응답 시간 500ms 이하인지 확인해줘"
   - "로그인 플로우 E2E 테스트 해줘"
   - "브라우저에서 실제로 동작하는지 확인"

2. **도메인별 기본 검증**:
   | 도메인 | 기본 검증 |
   |--------|----------|
   | `web-frontend` | browser (E2E), visual (스크린샷) |
   | `web-backend` | http (API 호출), cli (서버 시작) |
   | `ml-dl` | cli (추론), metrics (정확도) |
   | `infra` | cli (terraform), dryrun (apply 시뮬레이션) |

3. **대화 중 암시적 발견**:
   - "API가 빨라야 해요" → performance 검증
   - "화면이 예쁘게 나와야 해요" → visual 검증
   - "모델 정확도가 95% 이상이어야 해요" → metrics 검증

##### 검증 요구사항 질문 (선택적)

```python
# 도메인별 검증 옵션 제시
AskUserQuestion(questions=[
    {
        "question": "추가적인 검증이 필요한가요?",
        "header": "검증방식",
        "options": [
            {"label": "기본 검증만 (권장)", "description": f"도메인 기본: {domain_defaults}"},
            {"label": "브라우저 E2E 테스트", "description": "실제 브라우저에서 시나리오 테스트"},
            {"label": "API 성능 테스트", "description": "응답 시간, 상태 코드 검증"},
            {"label": "시각적 회귀 테스트", "description": "스크린샷 비교"}
        ],
        "multiSelect": True
    }
])
```

##### 검증 요구사항 저장 (MCP 도구 사용)

```python
# 사용자가 E2E 테스트를 요청한 경우
mcp__c4__c4_add_verification(
    feature="user-auth",
    verification_type="browser",
    name="Login Flow E2E",
    reason="사용자 요청: 로그인 플로우 E2E 테스트",
    priority=1,  # 1=critical, 2=normal, 3=optional
    config={
        "url": "http://localhost:3000",
        "steps": [
            {"action": "goto", "url": "/login"},
            {"action": "type", "selector": "#email", "value": "test@example.com"},
            {"action": "type", "selector": "#password", "value": "password123"},
            {"action": "click", "selector": "#submit"},
            {"action": "wait", "selector": ".dashboard"}
        ]
    }
)

# API 성능 검증 요청
mcp__c4__c4_add_verification(
    feature="api-users",
    verification_type="http",
    name="User API Response Time",
    reason="사용자 요청: API 응답 500ms 이하",
    config={
        "url": "/api/users",
        "method": "GET",
        "max_response_time": 500,
        "expected_status": 200
    }
)

# ML 메트릭 검증
mcp__c4__c4_add_verification(
    feature="model-training",
    verification_type="metrics",
    name="Model Accuracy Check",
    reason="사용자 요구사항: 정확도 95% 이상",
    config={
        "thresholds": {
            "accuracy": {"min": 0.95},
            "loss": {"max": 0.1}
        }
    }
)
```

##### 검증 타입 참조

| Type | 용도 | 필수 config |
|------|------|------------|
| `http` | API 엔드포인트 검증 | `url`, `method`, `expected_status` |
| `browser` | E2E 브라우저 테스트 | `url`, `steps` (action 배열) |
| `cli` | CLI 명령 실행 검증 | `command`, `expected_output` 또는 `expected_exit_code` |
| `metrics` | ML/DL 메트릭 검증 | `thresholds` (metric → {min, max, eq}) |
| `visual` | 스크린샷 비교 | `baseline`, `current` |
| `dryrun` | 인프라 dry-run | `command`, `success_patterns`, `failure_patterns` |

##### 저장된 검증 확인

```python
# 저장된 검증 확인
verifications = mcp__c4__c4_get_feature_verifications(feature="user-auth")
print(f"📋 {verifications['feature']} 검증 요구사항:")
for v in verifications['verifications']:
    status = "✅" if v['enabled'] else "⏸️"
    print(f"  {status} [{v['type']}] {v['name']} (P{v['priority']})")
    print(f"      이유: {v['reason']}")
```

---

#### 2.5.6 Discovery 완료

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

### Phase 2.7: Contract-First — Lighthouse 도구 계약 정의

> **진입 조건**: Design 완료 후 (Phase 2.6.7), Task 생성 전
> **핵심 원칙**: "먼저 인터페이스를 정의하고, 그 다음에 구현한다" (TDD 방식)

#### 2.7.1 설계에서 도구 계약 추출

Design 단계에서 정의된 컴포넌트/인터페이스를 분석하여
**MCP 도구로 노출할 계약(contract)**을 식별합니다.

**식별 기준:**
| 유형 | 예시 | Lighthouse 후보 |
|------|------|----------------|
| 새 MCP 도구 | `c4_xyz` 도구 추가 | 반드시 등록 |
| 새 API 엔드포인트 | REST/gRPC/WS | 도구 래퍼로 등록 |
| 새 서비스 인터페이스 | `FooService.bar()` | 외부 노출 시 등록 |
| 내부 헬퍼/유틸 | `parseX()`, `validate()` | 등록 불필요 |

**원칙**: 새 기능 계획 시 **반드시 Lighthouse 도구 계약을 정의**한다 (TDD의 "Red" 단계).
내부 리팩토링/버그 수정처럼 새 도구가 없는 경우에만 스킵 가능하며, 이 경우 명시적 사유를 남긴다.

```python
# 설계 분석 → 도구 계약 목록 생성
AskUserQuestion(questions=[
    {
        "question": "정의할 MCP 도구 계약을 확인하세요 (새 기능엔 필수)",
        "header": "도구계약",
        "options": [
            # 설계에서 자동 추출된 후보들
            {"label": "{tool_name_1}", "description": "{description_1}"},
            {"label": "{tool_name_2}", "description": "{description_2}"},
            {"label": "추가 정의", "description": "직접 도구 이름과 스펙 입력"}
        ],
        "multiSelect": True
    }
])
# 참고: "스킵"은 옵션 목록에 없음 — 새 기능엔 Lighthouse 필수.
# 리팩토링/버그수정 시에만 사용자가 직접 "해당 없음"을 입력.
```

#### 2.7.2 도구별 계약 정의 (Lighthouse Register)

각 선택된 도구에 대해 **입력 스키마 + API 스펙**을 정의하고 Lighthouse stub으로 등록합니다.

```python
# 도구마다 계약 정의
for tool in selected_tools:
    # 1. 입력 스키마 정의 (Design의 컴포넌트 인터페이스에서 추출)
    input_schema = {
        "type": "object",
        "properties": {
            "param1": {"type": "string", "description": "..."},
            "param2": {"type": "integer", "description": "..."}
        },
        "required": ["param1"]
    }

    # 2. API 스펙 정의 (계약)
    spec = """
    ## {tool_name}
    ### 입력
    - param1 (string, required): ...
    - param2 (integer, optional): ...
    ### 출력
    - success (bool): 성공 여부
    - data: ...
    ### 에러
    - "param1 is required": param1 누락
    - "not found": 리소스 없음
    ### 동작
    1. ...
    2. ...
    """

    # 3. Lighthouse stub 등록 (auto_task=true → T-LH-{name}-0 자동 생성)
    mcp__c4__c4_lighthouse(
        action="register",
        name=tool_name,
        description=tool_description,
        input_schema=json.dumps(input_schema),
        spec=spec,
        auto_task=True
    )
```

#### 2.7.3 계약 검증 (Stub 호출 테스트)

등록된 Lighthouse stub을 직접 호출하여 **계약이 올바르게 정의되었는지** 확인합니다.

```python
# Stub 호출 → 계약 정보 반환 확인
result = mcp__c4__{tool_name}(param1="test")
# result: {"lighthouse": true, "status": "stub", "spec": "...", ...}

# 모든 stub 호출 성공 확인
print(f"✅ {len(stubs)}개 Lighthouse stub 등록 완료")
print(f"   자동 생성된 태스크: {[f'T-LH-{name}-0' for name in stubs]}")
```

#### 2.7.4 Lighthouse 요약

```
🏗️ Contract-First TDD — Lighthouse 등록 완료
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  🔴 c4_xyz         — "XYZ 도구 설명"    → T-LH-c4_xyz-0
  🔴 c4_abc         — "ABC 도구 설명"    → T-LH-c4_abc-0

  다음: Phase 3 (환경 인터뷰) → Phase 4 (태스크 생성)

  💡 T-LH 태스크 완료 시 해당 Lighthouse가 자동으로 promote됩니다.
```

---

### Phase L: Lighthouse 관리

> **진입 조건**: Phase 0.5에서 "Lighthouse 관리" 선택 시

#### L.1 액션 선택

```python
AskUserQuestion(questions=[
    {
        "question": "Lighthouse 작업을 선택하세요",
        "header": "LH액션",
        "options": [
            {"label": "새 도구 계약 등록", "description": "Lighthouse stub 등록 + 태스크 생성"},
            {"label": "도구 목록 조회", "description": "등록된 Lighthouse 전체 보기"},
            {"label": "수동 Promote", "description": "구현 완료된 stub을 implemented로 전환"},
            {"label": "도구 제거", "description": "Lighthouse deprecated 처리"}
        ],
        "multiSelect": False
    }
])
```

각 선택에 따라 `c4_lighthouse` MCP 도구의 해당 action(register/list/promote/remove) 호출.

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
6. **DDD-CLEANCODE 구조화된 명세 추가** (아래 Worker Packet 섹션 참조)

---

#### ⚠️ Worker Packet (DDD-CLEANCODE 구조화된 명세) - 권장!

**모든 태스크는 가능하면 Worker Packet 형태로 명세를 포함해야 합니다.**

Worker Packet은 다음 요소로 구성됩니다:

| 요소 | 필수 | 설명 |
|------|------|------|
| **Goal** | ✅ | 완료 조건 + 범위 외 명시 |
| **ContractSpec** | ✅ | API 명세 + 테스트 명세 |
| **LighthouseRef** | ⚠️ | Lighthouse stub 이름 (있으면 계약 기반 구현) |
| **BoundaryMap** | ⚠️ | DDD 레이어 제약 |
| **CodePlacement** | ⚠️ | 생성/수정 파일 목록 |
| **QualityGates** | ⚠️ | 검증 명령어 |
| **Checkpoints** | ⚠️ | CP1/CP2/CP3 마일스톤 |

##### 4.1 ContractSpec 입력 (API + 테스트)

각 태스크에 대해 API 명세와 필수 테스트를 정의합니다.

```python
AskUserQuestion(questions=[
    {
        "question": "이 태스크의 주요 API는 무엇인가요?",
        "header": "API명세",
        "options": [
            {"label": "서비스 메서드", "description": "예: UserService.register()"},
            {"label": "REST 엔드포인트", "description": "예: POST /api/users"},
            {"label": "CLI 명령", "description": "예: c4 add-task"},
            {"label": "없음 (리팩토링/버그픽스)", "description": "공개 API 변경 없음"}
        ],
        "multiSelect": False
    }
])
```

**API 상세 정보 수집:**
```python
# API 명세 구조화
api_spec = {
    "name": "UserService.register",
    "input": "email: str, password: str",
    "output": "User | None",
    "errors": ["DuplicateEmail", "WeakPassword"],
    "side_effects": "DB에 user 레코드 생성"
}
```

**필수 테스트 정의 (최소 요구사항):**
```python
AskUserQuestion(questions=[
    {
        "question": "이 API의 테스트 케이스를 정의해주세요",
        "header": "테스트명세",
        "options": [
            {"label": "기본 3종 (권장)", "description": "성공 1 + 실패 1 + 경계 1"},
            {"label": "상세 테스트", "description": "직접 테스트 케이스 정의"},
            {"label": "기존 테스트 활용", "description": "테스트 파일 지정"}
        ],
        "multiSelect": False
    }
])

# 테스트 명세 구조화
required_tests = {
    "success": ["test_register_creates_user"],
    "failure": ["test_register_duplicate_email", "test_register_weak_password"],
    "boundary": ["test_register_max_email_length"]
}
```

##### 4.2 BoundaryMap 입력 (DDD 레이어 제약)

클린 아키텍처 경계를 정의합니다.

```python
AskUserQuestion(questions=[
    {
        "question": "이 코드가 속하는 도메인과 레이어는?",
        "header": "경계정의",
        "options": [
            {"label": "domain 레이어 (권장)", "description": "비즈니스 로직, 외부 의존성 없음"},
            {"label": "app 레이어", "description": "유스케이스, domain 참조 가능"},
            {"label": "infra 레이어", "description": "DB/외부 API, 모든 레이어 참조 가능"},
            {"label": "api 레이어", "description": "컨트롤러/라우터"}
        ],
        "multiSelect": False
    },
    {
        "question": "허용되지 않는 import가 있나요?",
        "header": "금지import",
        "options": [
            {"label": "도메인 기본 (권장)", "description": "sqlalchemy, httpx, fastapi 금지"},
            {"label": "없음", "description": "제한 없음"},
            {"label": "커스텀", "description": "직접 지정"}
        ],
        "multiSelect": False
    }
])

# BoundaryMap 구조화
boundary_map = {
    "target_domain": "auth",
    "target_layer": "app",
    "allowed_imports": ["stdlib", "pydantic", "domain.user"],
    "forbidden_imports": ["sqlalchemy", "httpx", "fastapi"],
    "public_export": "src/api/v1/users.py"
}
```

##### 4.3 CodePlacement 입력 (파일 위치)

생성/수정할 파일을 명시합니다.

```python
# 코드 배치 구조화
code_placement = {
    "create": ["src/auth/service.py", "src/auth/domain/user.py"],
    "modify": ["src/api/v1/users.py"],
    "tests": ["tests/unit/auth/test_service.py"]
}
```

##### 4.4 QualityGates 입력 (검증 명령)

태스크별 검증 명령을 정의합니다.

```python
# 품질 게이트 구조화
quality_gates = [
    {"name": "lint", "command": "uv run ruff check .", "required": True},
    {"name": "unit", "command": "uv run pytest tests/unit/auth/ -v", "required": True},
    {"name": "forbidden_imports", "command": "uv run python scripts/check_imports.py src/auth/", "required": True}
]
```

##### 4.5 Work Breakdown 검증

태스크 생성 전에 크기가 적절한지 검증합니다.

**기준 (DDD-CLEANCODE 가이드):**
| 항목 | 최대 | 초과 시 |
|------|------|---------|
| 소요 시간 | 2일 | 분할 필수 |
| 공개 API | 3개 | 분할 권장 |
| 테스트 | 9개 | 분할 권장 |
| 수정 파일 | 5개 | 분할 권장 |
| 도메인 | 1개 | **분할 필수** |

```python
# 분할 필요 여부 검사
if api_count > 3 or file_count > 5 or domain_count > 1:
    AskUserQuestion(questions=[
        {
            "question": f"태스크가 너무 큽니다 (API {api_count}개, 파일 {file_count}개). 분할할까요?",
            "header": "태스크분할",
            "options": [
                {"label": "분할 (권장)", "description": "작은 태스크 여러 개로 나누기"},
                {"label": "유지", "description": "큰 태스크로 진행"},
            ],
            "multiSelect": False
        }
    ])
```

##### 4.6 Worker Packet 저장

모든 명세를 포함한 완전한 태스크를 생성합니다.

```python
# 완전한 Worker Packet으로 태스크 생성
mcp__c4__c4_add_todo({
    "task_id": "T-001",
    "title": "Implement user registration",
    "scope": "src/auth/",
    "dod": """
Goal: POST /v1/users 호출 시 User가 생성된다

ContractSpec:
  API: UserService.register(email: str, password: str) -> User | None
  Errors: DuplicateEmail, WeakPassword
  Tests:
    - success: test_register_creates_user
    - failure: test_register_duplicate_email
    - boundary: test_register_max_email_length

BoundaryMap:
  Domain: auth / Layer: app
  Forbidden: sqlalchemy, httpx

CodePlacement:
  Create: src/auth/service.py
  Modify: src/api/v1/users.py
  Tests: tests/unit/auth/test_service.py

Checkpoints:
  CP1: 파일 배치 + 테스트 골격
  CP2: register() 성공 테스트 통과
  CP3: 실패/경계 테스트 추가
"""
})
```

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

`c4_add_todo`에 T- 접두사 태스크를 추가하면 **R-XXX 리뷰 태스크가 자동 생성**됩니다:

```javascript
// T-001-0 추가 → R-001-0 자동 생성 (review_required 기본값 true)
mcp__c4__c4_add_todo({
  task_id: "T-001-0",
  title: "MediaPipe Hands 연동",
  scope: "src/HandTracker.js",
  dod: "1) HandTracker 클래스 구현, 2) startTracking() 시 웹캠 시작, 3) onFrame에서 좌표 반환, 4) stopTracking() 시 해제"
})
// → 결과: { task_id: "T-001-0", review_task_id: "R-001-0" }

// review_required=false면 R 태스크 미생성 (인프라/설정 태스크 등)
mcp__c4__c4_add_todo({
  task_id: "T-002-0",
  title: "CI/CD 설정",
  dod: "GitHub Actions 워크플로우 추가",
  review_required: false
})
```

**CP 태스크도 Plan 단계에서 명시적으로 생성:**
```javascript
// CP 태스크: 여러 R 태스크 완료 후 실행
mcp__c4__c4_add_todo({
  task_id: "CP-001",
  title: "Phase 1 checkpoint",
  dod: "Phase 1 구현+리뷰 완료 확인",
  dependencies: ["R-001-0", "R-002-0"],
  review_required: false
})
```

**의존성 트리:**
```
T-001-0 → R-001-0 ─┐
T-002-0 → R-002-0 ─┤→ CP-001 → T-003-0 → R-003-0
```

**리뷰에서 변경 요청 시:**
```
R-001-0 REQUEST_CHANGES → T-001-1 (수정) → R-001-1 (재리뷰)
                          (max_revision 초과 시 blocked)
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
/c4-plan    현황 파악 → 행동 선택 → 적절한 플로우 진행
```

### 플로우 요약

```
/c4-plan 실행
    ↓
Phase 0: 현황 출력 (상태, 태스크, specs, designs, lighthouses, docs)
    ↓
Phase 0.5: 행동 선택
    ├→ "새 기능 계획"      → Phase 1~2.7~5 (Discovery → Design → Lighthouse → Tasks)
    ├→ "기존 계획 검토/수정" → Phase R (상세 보기 → 수정)
    ├→ "Lighthouse 관리"   → Phase L (등록/조회/promote/삭제)
    ├→ "태스크만 추가"      → Phase 4~5 (Tasks 직행)
    └→ "현황만 확인"       → 종료
```

### 태스크 의존성 그래프 범례

```
✅ 완료    🔄 진행중    ⏳ 대기    ❌ 블록

예시:
✅ T-001 ─┬→ ✅ T-002 ─→ ⏳ T-003
          └→ 🔄 T-004
```

---

## 계획 검증 체크리스트

계획 확정 전 다음 항목을 확인합니다:

### 필수 검증 (모두 통과해야 진행 가능)

- [ ] **요구사항 명확성**: 모든 요구사항이 EARS 패턴으로 작성되었는가?
- [ ] **DoD 구체성**: 모든 태스크의 DoD가 검증 가능한가?
- [ ] **의존성 유효성**: 순환 의존성이 없는가?
- [ ] **범위 정의**: 각 태스크의 scope가 명확한가?
- [ ] **사용자 승인**: 계획을 사용자가 명시적으로 승인했는가?

### 권장 검증 (품질 향상)

- [ ] **아키텍처 결정**: 주요 기술 결정이 문서화되었는가?
- [ ] **검증 전략**: lint, unit, e2e 중 필요한 검증이 정의되었는가?
- [ ] **체크포인트**: 적절한 위치에 체크포인트가 설정되었는가?
- [ ] **위험 식별**: 기술적 위험이 식별되고 대응 계획이 있는가?

### EARS 패턴 참조

| 패턴 | 키워드 | 예시 |
|------|--------|------|
| **Ubiquitous** | "시스템은 ~해야 한다" | 시스템은 모든 입력을 검증해야 한다 |
| **Event-Driven** | "~할 때" | 사용자가 제출할 때, 시스템은 저장해야 한다 |
| **State-Driven** | "~하는 동안" | 로딩 중, 시스템은 스피너를 표시해야 한다 |
| **Optional** | "~기능이 있으면" | 다크모드가 활성화되면, 테마를 변경해야 한다 |
| **Unwanted** | "~하면 (에러 상황)" | 인증 실패 시, 에러 메시지를 표시해야 한다 |

> 참고: [EARS 논문](https://ieeexplore.ieee.org/document/5328509) - Easy Approach to Requirements Syntax

---

## 관련 명령어

- `/c4-add-task` - 개별 태스크 추가
- `/c4-run` - 실행 시작
- `/c4-status` - 상태 확인

---
description: |
  Create structured implementation plans for C4 projects. Scans project state,
  specs, designs, docs, and Lighthouse contracts, then guides through Discovery
  (EARS requirements), Design (architecture decisions), Lighthouse contracts
  (contract-first TDD), and task breakdown with DoD. Use when the user wants to
  plan features, review existing plans, manage Lighthouse contracts, or create
  implementation tasks. Triggers: "계획 수립", "기능 설계", "태스크 분해",
  "plan this feature", "create implementation plan", "design this",
  "break down requirements", "set up tasks".
---

# C4 Plan Mode

Structured project planning: Discovery, Design, Lighthouse, and Task creation.

## Critical Rules

### MUST NOT write code before user explicitly approves the plan

```
Forbidden:
- Writing code during plan explanation
- "Let's try and adjust" approach
- Executing tasks without user confirmation

Required:
- Plan summary -> user confirmation -> "proceed" -> code writing
- Unclear requirements -> ask questions -> agree -> proceed
- Change request -> modify plan -> reconfirm -> proceed
```

Violation: Workers' code will be rejected in review.

### Main session plans only; Workers implement

```
Forbidden:
- Adding tasks then "I'll do it myself" -> writing code
- Editing files after c4_add_todo()
- Main session doing implementation without Workers

Required:
- Create tasks -> /c4-run to spawn Workers -> Workers execute
- Main monitors, reviews, makes decisions
- Implementation always through Workers
```

---

## Flags (--from-pi / --auto-run)

`/pi`에서 호출 시 args에 플래그가 포함될 수 있다.

```python
args = "$ARGUMENTS"  # e.g. "--from-pi my-feature --auto-run"
FROM_PI = "--from-pi" in args
AUTO_RUN = "--auto-run" in args

if FROM_PI:
    # slug 추출: "--from-pi {slug}" 뒤 첫 토큰
    parts = args.split("--from-pi", 1)[1].strip().split()
    PI_SLUG = parts[0] if parts else ""
    idea_path = f".c4/ideas/{PI_SLUG}.md"
    print(f"🔗 /pi 연결 모드: idea.md = {idea_path}")
    if AUTO_RUN:
        print("🚀 자동 실행 모드: 태스크 생성 후 /c4-run → /c4-finish 자동 진행")
```

---

## Phase 0: Context Display

### 0.0 Config 읽기 (필수 — 가장 먼저 실행)

```python
cfg = c4_config_get(section="all")

# Critique Loop 설정 읽기
critique_cfg = cfg.get("planning", {}).get("critique_loop", {})
CRITIQUE_ENABLED = critique_cfg.get("enabled", True)
CRITIQUE_MAX_ROUNDS = critique_cfg.get("max_rounds", 3)
CRITIQUE_MODE = critique_cfg.get("mode", "auto")
# mode: "auto"        → 루프 자동 실행 (기본값)
# mode: "interactive" → 라운드마다 사용자 확인 후 진행
# mode: "skip"        → Phase 4.5 완전 건너뜀 (enabled=false와 동일)

# 루프 비활성화 판정
LOOP_ACTIVE = CRITIQUE_ENABLED and CRITIQUE_MODE != "skip"

# 설정 요약 출력
if LOOP_ACTIVE:
    print(f"📋 Plan Refine: ON (mode={CRITIQUE_MODE}, max={CRITIQUE_MAX_ROUNDS} rounds)")
    print("   변경: .c4/config.yaml planning.critique_loop.mode = skip 으로 비활성화")
else:
    print(f"📋 Plan Refine: OFF (enabled={CRITIQUE_ENABLED}, mode={CRITIQUE_MODE})")
```

### 0.1 Data Collection

Call these MCP tools to gather current state:

```
1. c4_status()           — project state, tasks, progress
2. c4_list_specs()       — saved specifications
3. c4_list_designs()     — saved designs
4. c4_lighthouse(list)   — tool contracts (stubs/implemented)
5. Glob docs/**/*.md     — planning documents
6. c4_knowledge_search(query="{feature description}")  — past patterns/insights/experiments
7. c4_pattern_suggest(context="{domain}")              — recurring patterns from past work
```

**Knowledge 조회 목적**: 과거 유사 기능의 실패/성공 패턴, 아키텍처 결정의 이유,
반복된 이슈 패턴을 참조하여 같은 실수를 방지하고 검증된 접근 방식을 재활용합니다.
결과가 없으면 건너뜁니다.

### 0.2 Rich Status Output

Display comprehensive project context. For detailed ASCII templates, see `references/output-format.md`.

Output sections (all required):
1. **Project Overview** — name, description, domain, key features (from README.md)
2. **Current State** — workflow position, status, supervisor, workers, progress bar
3. **Task Dependency Graph** — visual tree with status icons
4. **Specifications** — EARS requirements summary per feature
5. **Designs** — architecture options, components, decisions per feature
6. **Lighthouse** — stub count, implemented count, active stubs list
7. **Planning Documents** — docs/ file listing
8. **Tech Stack** — language, package manager, database, validation tools

Information sources: README.md, pyproject.toml/package.json, LICENSE, c4_status output.

### 0.3 Dependency Graph Rendering

Show only dependency chains related to pending tasks. Start from root tasks (no deps).
Status icons: completed, in_progress, pending, blocked.

---

## Phase 0.5: Action Selection

After displaying status, ask the user what to do:

```python
AskUserQuestion(questions=[{
    "question": "What would you like to do?",
    "header": "Action",
    "options": [
        {"label": "Plan new feature", "description": "Discovery -> Design -> Lighthouse -> Tasks full flow"},
        {"label": "Review/modify existing plan", "description": "View and edit saved Spec/Design"},
        {"label": "Lighthouse management", "description": "Register/list/promote/remove tool contracts"},
        {"label": "Add tasks only", "description": "Create tasks from existing design"},
        {"label": "View status only", "description": "Done after display"}
    ],
    "multiSelect": False
}])
```

| Selection | Next Phase |
|-----------|-----------|
| Plan new feature | Phase 1 (doc scan) |
| Review/modify | Phase R |
| Lighthouse | Phase L |
| Add tasks only | Phase 4 |
| View status only | End |

---

## Phase R: Review/Modify Existing Plans

### R.1 Target Selection

List all specs and designs. Ask user which to review.

```python
# Build options from specs['features'] and designs['designs']
AskUserQuestion(questions=[{
    "question": "Which item to review?",
    "header": "Target",
    "options": [
        # {"label": "{feature} (Spec)", "description": "{domain} - {N} requirements"}
        # {"label": "{feature} (Design)", "description": "Option: {selected}, {N} components"}
    ],
    "multiSelect": False
}])
```

### R.2 Detail Display

- **Spec**: c4_get_spec(feature=X) -> show domain, description, all requirements with EARS patterns
- **Design**: c4_get_design(feature=X) -> show selected option, components, decisions, mermaid diagram

### R.3 Modification

Ask if user wants to modify:
- Requirements add/edit -> EARS interview -> c4_save_spec()
- Component changes -> c4_save_design()
- Architecture decision changes -> c4_save_design()
- No changes -> exit

---

## Phase 1: Planning Document Scan

> Entry: "Plan new feature" selected in Phase 0.5

Scan project root and docs/ for planning documents.

**Targets**: `*.md` files containing PRD, requirements, spec, plan keywords. Files > 1KB.

**Output**: List found documents with size and type description.

---

## Phase 2: Document Interpretation

Read each planning document and extract:
1. **Project overview**: name, goal, background
2. **Core features**: feature list
3. **Tech stack**: languages, frameworks, libraries
4. **Roadmap**: phase/stage plans
5. **Architecture**: component structure, data flow

Extraction hints:
- `- [ ]` checklists -> potential tasks
- `Phase N:` or stage markers -> checkpoint candidates
- Technology names -> tech stack

---

## Phase 2.5: Discovery (EARS Requirements)

### 2.5.0 /pi 컨텍스트 감지 (FROM_PI 모드)

```python
if FROM_PI:
    # idea.md를 Discovery 소스로 사용 → Q&A 인터뷰 생략
    idea_content = c4_read_file(path=idea_path)

    print(f"""
💡 /pi idea.md 컨텍스트 로드: {idea_path}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
{idea_content[:800]}...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Discovery Q&A 인터뷰를 건너뜁니다 — idea.md에서 요구사항 추출 중
""")

    # idea.md의 "문제 정의", "가정", "리스크", "핵심 결정" 섹션을 파싱하여
    # EARS 패턴으로 자동 변환 후 c4_save_spec() 저장
    # → 2.5.3~2.5.5 실행 후 Phase 2.6으로 바로 진행

else:
    # 기존 EARS 인터뷰 진행 (2.5.1 이하)
    pass
```

> `FROM_PI = True`이면 2.5.1~2.5.2(인터뷰)를 건너뛰고 idea.md 내용으로 spec을 직접 작성한다.

### 2.5.1 Domain Auto-Detection

Analyze project structure to infer domain. For detection rules, see `references/domain-templates.md`.

Confirm with user:
```python
AskUserQuestion(questions=[{
    "question": f"Domain detected as [{detected}]. Correct?",
    "header": "Domain",
    "options": [
        {"label": f"{detected} (detected)", "description": "Auto-detected domain"},
        {"label": "Web Frontend", "description": "React, Vue, etc."},
        {"label": "Web Backend", "description": "FastAPI, Express, etc."},
        {"label": "ML/DL", "description": "PyTorch, TensorFlow, etc."}
    ],
    "multiSelect": True
}])
```

### 2.5.2 EARS Requirements Collection

Use EARS (Easy Approach to Requirements Syntax) patterns:

| Pattern | Format | Example |
|---------|--------|---------|
| **Ubiquitous** | "The system shall ~" | "The system shall encrypt user data" |
| **Event-Driven** | "When ~, the system shall ~" | "When user submits login, system shall validate" |
| **State-Driven** | "While ~, the system shall ~" | "While loading, system shall show spinner" |
| **Optional** | "If ~ is enabled, the system shall ~" | "If dark mode enabled, use dark theme" |
| **Unwanted** | "If ~ (error), the system shall ~" | "If invalid credentials, show error" |

**Interview flow**:
1. Identify core features (user-stated = must detail, AI-judged = confirm)
2. Detail each feature with EARS patterns
3. Confirm edge cases with follow-up questions

For domain-specific interview questions, see `references/domain-templates.md`.

### 2.5.3 Save Specification

```python
c4_save_spec(
    name="feature-name",
    content="""
feature: feature-name
domain: web-backend
description: Feature description
requirements:
  - id: REQ-001
    pattern: event-driven
    text: "When user submits form, system shall validate"
  - id: REQ-002
    pattern: unwanted
    text: "If validation fails, system shall show error"
"""
)
```

### 2.5.4 Verification Requirements

Collect verification needs from conversation and domain defaults:

| Domain | Default Verification |
|--------|---------------------|
| web-frontend | browser (E2E), visual |
| web-backend | http (API), cli (server) |
| ml-dl | cli (inference), metrics |
| infra | cli (terraform), dryrun |

Verification requirements go into task DoD (not separate tools).

Verification types: `unit`, `http`, `browser`, `cli`, `metrics`, `visual`, `dryrun`.

### 2.5.5 Discovery Complete

```python
specs = c4_list_specs()
# Show saved specs summary
c4_discovery_complete()  # Transitions to DESIGN state
```

---

## Phase 2.6: Design (Architecture Decisions)

### 2.6.1 Architecture Options

For each core feature, present architecture options with:
- id, name, description, complexity (low/medium/high)
- pros, cons, recommended flag

For domain-specific architecture templates, see `references/domain-templates.md`.

### 2.6.2 Component Design

Define components with: name, type, description, responsibilities, dependencies, interfaces.

### 2.6.3 Data Flow + Mermaid Diagram

Define data flows between components. Generate Mermaid sequence/flow diagrams.

### 2.6.4 Design Decisions

Record decisions with: id (DEC-XXX), question, decision, rationale, alternatives_considered.

### 2.6.5 Save Design

```python
c4_save_design(
    name="feature-name",
    content="""
feature: feature-name
domain: web-backend
description: Feature design

options:
  - id: option-a
    name: Option A Name
    description: "Description"
    complexity: low
    pros: [pro1, pro2]
    cons: [con1]
    recommended: true
  - id: option-b
    name: Option B Name
    description: "Description"
    complexity: medium

selected_option: option-a

components:
  - name: ServiceName
    type: service
    description: Business logic
    responsibilities: [resp1, resp2]
    dependencies: [Dep1, Dep2]

decisions:
  - id: DEC-001
    question: "Which approach?"
    decision: Option A
    rationale: "Fits project scale"

mermaid: |
  sequenceDiagram
    Client->>Controller: POST /api/action
    Controller->>Service: process()
    Service-->>Controller: Result
    Controller-->>Client: 200 OK
"""
)
```

### 2.6.6 Design Confirmation

Show all designs, ask user to confirm, modify, or restart from Discovery.

### 2.6.7 Design Complete

```python
c4_design_complete()  # Transitions to PLAN state
```

---

## Phase 2.7: Contract-First Lighthouse

> Entry: After Design complete, before Task creation.
> Principle: "Define interface first, implement second" (TDD approach).

### 2.7.1 Extract Tool Contracts from Design

Analyze design components/interfaces to identify MCP tools to expose.

| Type | Example | Lighthouse? |
|------|---------|-------------|
| New MCP tool | c4_xyz | MUST register |
| New API endpoint | REST/gRPC/WS | Register as tool wrapper |
| New service interface | FooService.bar() | Register if externally exposed |
| Internal helper | parseX(), validate() | NOT needed |

**Rule**: New features MUST define Lighthouse contracts. Skip only for refactoring/bugfix (state reason).

```python
AskUserQuestion(questions=[{
    "question": "Confirm MCP tool contracts to define (required for new features)",
    "header": "Contracts",
    "options": [
        # Auto-extracted from design
        {"label": "{tool_name_1}", "description": "{description}"},
        {"label": "{tool_name_2}", "description": "{description}"},
        {"label": "Add custom", "description": "Define tool name and spec manually"},
        {"label": "Skip (no new tools)", "description": "Refactoring/bugfix only"}
    ],
    "multiSelect": True
}])
```

### 2.7.2 Register Lighthouse Stubs

For each selected tool, define input schema + API spec, then register:

```python
c4_lighthouse(
    action="register",
    name=tool_name,
    description=tool_description,
    input_schema=json.dumps(input_schema),
    spec=spec_text,
    auto_task=True  # Creates T-LH-{name}-0 automatically
)
```

### 2.7.3 Verify Stubs

Call each registered stub to confirm contract is properly defined.

### 2.7.4 Summary

Display registered stubs with their auto-generated task IDs.

---

## Phase L: Lighthouse Management

> Entry: "Lighthouse management" selected in Phase 0.5

```python
AskUserQuestion(questions=[{
    "question": "Select Lighthouse action",
    "header": "LH Action",
    "options": [
        {"label": "Register new contract", "description": "Lighthouse stub + task creation"},
        {"label": "List all tools", "description": "View all registered Lighthouse entries"},
        {"label": "Manual promote", "description": "Mark implemented stub as complete"},
        {"label": "Remove tool", "description": "Deprecate Lighthouse entry"}
    ],
    "multiSelect": False
}])
```

Execute corresponding c4_lighthouse action (register/list/promote/remove).

---

## Phase 3: Development Environment Interview

Ask about development environment not found in documents.
For domain-specific questions, see `references/domain-templates.md`.

### 3.1 Core Environment

Ask about: language, build tool, package manager.

### 3.2 Test Strategy

Ask about: unit test framework, E2E framework.

### 3.3 Quality Standards

Ask about: linting/formatting tools (multi-select).

### 3.4 C4 Workflow

Ask about: checkpoint placement (per-phase/per-feature/none), task granularity, auto-execution scope.

---

## Phase 4: Task Draft (DB 커밋 전)

> ⚠️ 이 단계에서는 c4_add_todo를 호출하지 않습니다. 텍스트 초안만 작성합니다.
> DB 커밋은 Phase 4.5 Critique Loop 수렴 후 Phase 4.9에서만 수행합니다.

Create C4 tasks from interview results and design.
For Worker Packet format and DoD principles, see `references/worker-packet.md`.

### Core Rules

1. PRD checklist items -> individual tasks
2. `scope` = affected files/directories
3. `dod` MUST be specific and verifiable
4. `dependencies` respect execution order
5. Worker Packet format recommended for all tasks

### Worker Packet Elements

| Element | Required | Description |
|---------|----------|-------------|
| **Goal** | Yes | Completion criteria + out-of-scope |
| **Rationale** | Yes | Why this approach (design decision ref, past knowledge) |
| **ContractSpec** | Yes | API spec + test spec |
| **LighthouseRef** | If exists | Lighthouse stub name |
| **BoundaryMap** | Recommended | DDD layer constraints |
| **CodePlacement** | Recommended | Files to create/modify |
| **QualityGates** | Recommended | Validation commands |
| **Checkpoints** | Recommended | CP1/CP2/CP3 milestones |

### DoD Quality Rules

- **Verifiable**: "X works", "returns Y", "test passes"
- **Specific**: No vague terms ("improve", "optimize")
- **Independent**: Checkable without other tasks

#### Goal-Driven 변환 원칙 (Karpathy)

> "Don't tell it what to do — give it success criteria and watch it go."

| 명령형 (피할 것) | 선언형 목표로 변환 |
|----------------|-----------------|
| "X validation 추가해" | "잘못된 입력 테스트 작성 → 모두 통과시켜라" |
| "버그 수정해" | "버그를 재현하는 테스트 작성 → 통과시켜라" |
| "X 최적화해" | "현재 수치 측정 → 목표(N ms/N%) 달성 테스트 → 통과" |
| "리팩토링해" | "기존 테스트 통과 확인 → 리팩토링 → 여전히 통과" |

| Bad DoD | Good DoD |
|---------|----------|
| "Implement login" | "Email/password login returns JWT, wrong password returns 401" |
| "Optimize API" | "GET /users response < 100ms, existing tests pass" |
| "Fix bug" | "null input returns empty array, add regression test" |

#### QualityGates 표준 (CRITICAL)

DoD에 테스트 언급이 있으면 `QualityGates`에 **파일 존재 확인 명령**을 반드시 포함한다.

> ⚠️ `pnpm test` / `go test ./...` 단독 실행은 테스트 파일이 없어도 통과 → **false positive** 방지 필수.

```
QualityGates:
  # ❌ 잘못된 예 (테스트 파일 없어도 통과 가능)
  - pnpm test

  # ✅ 올바른 예 (파일 존재 먼저 확인)
  - test -f src/components/Foo.test.tsx        # 테스트 파일 존재 확인
  - pnpm test --run src/components/Foo.test.tsx # 해당 파일만 실행
  # 또는 Go:
  - test -f internal/foo/foo_test.go
  - go test ./internal/foo/...
```

| 언어 | 존재 확인 | 실행 |
|------|----------|------|
| TypeScript | `test -f path/to/Foo.test.tsx` | `pnpm test --run Foo.test.tsx` |
| Go | `test -f path/to/foo_test.go` | `go test ./path/to/...` |
| Python | `test -f tests/test_foo.py` | `uv run pytest tests/test_foo.py` |
| Rust | `grep -q '#\[test\]' src/foo.rs` | `cargo test foo` |

### Task Size Validation

| Metric | Max | If exceeded |
|--------|-----|-------------|
| Public APIs | 3 | Split recommended |
| Modified files | 5 | Split recommended |
| Domains | 1 | **Split required** |

If exceeded, ask user whether to split.

### DoD Starter Template

DoD 초안 작성 시 아래 구조를 시작점으로 사용한다:

```
1. Assumptions: 이 태스크가 전제하는 것 (명시적으로)
2. Success Criteria: 테스트/명령으로 검증 가능한 완료 상태
3. Failure Modes Top 3: 예상 실패 케이스 + 대응
4. QualityGates: 파일 존재 확인 + 테스트 실행
```

### Task Draft Format (텍스트만, DB 저장 금지)

```
[DRAFT] T-001-0: Task title
  scope: src/path/
  dod: |
    Goal: ...
    Rationale: (why this approach)
    ContractSpec:
      API: ...
      Tests: ...
      **Assumptions** (구현 전 선제 선언 — 틀리면 멈추고 확인):
        - [이 태스크가 전제하는 파일 경로, API 형식, 외부 의존성]
    CodePlacement:
      Modify: ...
  dependencies: []

[DRAFT] CP-001: Phase 1 checkpoint
  dod: Phase 1 implementation + review complete
  dependencies: [R-001-0, R-002-0]
  review_required: false
```

Dependency tree: `T-XXX -> R-XXX -> CP-XXX -> T-YYY -> R-YYY`

> ⚠️ **CP deps 규칙 (CRITICAL)**: CP 태스크의 `dependencies`에는 반드시 R- 태스크만 넣는다.
> T- 태스크를 CP에 직접 연결하는 것은 리뷰 레이어 우회이므로 금지.
> 예외: `review_required=False`인 경우 명시적 사유 필수 (체크포인트·docs·migration 전용 태스크만 허용).
>
> **올바른 구조**: `T-001 → R-001 → CP-001`
> **잘못된 구조**: `T-001 → CP-001` (R-001 없음 → CRITICAL)

---

## Phase 4.5: Plan Refine (Worker 기반 Pre-Mortem)

> Entry: Phase 4 draft 완료 직후. DB 커밋 전.
> Exit: 수렴 선언 → Phase 4.9 (DB 커밋) → Phase 5
> Purpose: 신선한 컨텍스트를 가진 Worker가 계획을 비판 — confirmation bias 제거.
>
> ⚠️ **인라인 자가 비판 금지**: 계획을 만든 세션이 직접 비판하면 confirmation bias 발생.
>    매 라운드마다 반드시 새 Worker(Task agent)를 스폰하여 격리된 시각으로 검토.

### 4.5.0 Config 분기

```python
# Phase 0.0에서 읽은 설정 사용
if not LOOP_ACTIVE:
    print("⏭️  Plan Refine 비활성화 (config: planning.critique_loop)")
    print("   활성화: .c4/config.yaml → planning.critique_loop.enabled: true")
    → Phase 4.9 (DB Commit) 직행

INTERACTIVE = (CRITIQUE_MODE == "interactive")
```

### 4.5.1 Loop 초기화

```
round = 0 (max: CRITIQUE_MAX_ROUNDS, 기본 3)
converged = false
current_draft = Phase 4에서 작성한 태스크 초안 (전체 텍스트)
```

### 4.5.2 Worker 스폰: 격리된 Plan Refiner

> c4-refine 패턴 적용 — 매 라운드마다 새 Worker를 스폰하여 fresh context 보장.

```python
# interactive 모드: 라운드 시작 전 확인
if INTERACTIVE:
    AskUserQuestion(question=f"Round {round+1}/{CRITIQUE_MAX_ROUNDS} critique를 실행할까요?")

critique_result = Task(
    subagent_type="general-purpose",
    description=f"Plan critique round {round+1}",
    prompt=f"""
당신은 시니어 소프트웨어 아키텍트입니다.
아래 C4 구현 계획 초안을 Pre-Mortem 방식으로 비판하세요.

**프레임**: "이 계획대로 실행했고 3개월 후 실패했다. 가장 큰 실패 원인은?"

## 계획 초안
{current_draft}

## 비판 렌즈 (각 태스크에 대해 검토)

| 렌즈 | 질문 | 심각도 |
|------|------|--------|
| DoD 측정 가능성 | "완료됐음을 어떻게 증명하는가?" — 구체적 명령어/테스트가 있는가? | CRITICAL |
| **R- 쌍 누락** | **T- 태스크에 대응하는 R- 태스크가 의존성 트리에 있는가? CP deps에 T-가 직접 들어있고 R-가 없으면 리뷰 레이어가 없는 것.** | **CRITICAL** |
| **test 파일 존재 확인** | **DoD에 "X.test.tsx N개 pass"가 있으면 QualityGate에 `test -f X.test.tsx` 명령이 포함되어 있는가? 없으면 false positive 위험.** | **CRITICAL** |
| 파일 충돌 | 두 태스크가 같은 파일을 동시에 수정하는가? | CRITICAL |
| 가정 목록 | 이 태스크가 전제하는 것은? (파일 경로, API 형식, 외부 설정) | HIGH |
| 의존성 누락 | 숨겨진 실행 순서 제약이 있는가? | HIGH |
| 범위 과잉 | 수정 파일 5개 초과? API 3개 초과? 도메인 2개 이상? | HIGH |
| 더 단순한 방법 | 50% 코드로 80% 결과를 내는 방법이 있는가? | MEDIUM |
| 외부 의존성 | 환경 변수, 외부 서비스가 전제되어 있는가? | MEDIUM |

## 출력 형식 (반드시 이 형식으로)

```
## Plan Critique Round {round+1}

### CRITICAL (즉시 수정 필요)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### HIGH (수정 권장)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### MEDIUM (선택적)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### 수렴 판정
CRITICAL: N건 / HIGH: N건 / MEDIUM: N건
판정: CONVERGED (0건) | NOT_CONVERGED (N건 미해결)
```
"""
)
```

### 4.5.3 수정 (Revise) — 메인 세션에서 적용

Worker critique 결과를 받아 메인 세션에서 draft 수정:

- **CRITICAL**: 즉시 draft 수정 (DoD 재작성, 태스크 분리/병합)
- **HIGH**: 수정 적용 또는 Rationale에 리스크 명시
- **MEDIUM**: 사용자에게 선택 제시 (INTERACTIVE 모드) 또는 자동 적용

### 4.5.4 수렴 판정

```python
critical_count = critique_result.count("CRITICAL") - 해결된 수
high_count = critique_result.count("HIGH") - 해결된 수

converged = (critical_count == 0 and high_count == 0)

if converged or round + 1 >= CRITIQUE_MAX_ROUNDS:
    → 수렴 선언 출력 → Phase 4.9
else:
    round += 1
    current_draft = 수정된 draft
    → Phase 4.5.2 (새 Worker 스폰)
```

**수렴 선언 출력**:
```
## 계획 수렴 ✅ (Round N/MAX)
CRITICAL: 0 / HIGH: 0 / MEDIUM: N (허용)
Worker 비판 횟수: N회 (각 라운드 fresh context)
→ DB 커밋 진행
```

**MAX round 미수렴 시**:
```
## 계획 미수렴 ⚠️ (MAX/MAX rounds)
미해결: CRITICAL N건, HIGH N건
```
→ AskUserQuestion: "(1) 현재 상태로 진행  (2) 수동 수정 후 재시작"

---

## Phase 4.9: DB Commit

> Entry: Phase 4.5 수렴 선언 직후
> 이 단계에서만 c4_add_todo를 호출합니다.

수렴 확정된 task draft를 순서대로 DB에 기록합니다.
Critique loop에서 발생한 수정 이력은 각 태스크 DoD의 Rationale 섹션에 포함합니다.

```python
# T-XXX creates R-XXX review task automatically (review_required default true)
c4_add_todo(
    task_id="T-001-0",
    title="Task title",
    scope="src/path/",
    dod="Goal: ...\n\nRationale: (why this approach; critique loop에서 수정된 경우 이유 포함)\n\nContractSpec:\n  API: ...\n  Tests: ...\n\nCodePlacement:\n  Create: ...\n  Modify: ..."
)

# CP tasks depend on R tasks
c4_add_todo(
    task_id="CP-001",
    title="Phase 1 checkpoint",
    dod="Phase 1 implementation + review complete",
    dependencies=["R-001-0", "R-002-0"],
    review_required=False
)
```

---

## Phase 5: Plan Confirmation

Summarize the generated plan and confirm with user.

**Output**:
- Task count per phase
- Checkpoint count
- Validation strategy
- Task list per phase
- Next steps (전체 실행 플로우 포함)

**Confirmation**:

```python
if AUTO_RUN:
    # /pi --auto-run 모드: 사용자 확인 없이 자동 진행
    print("""
✅ 태스크 생성 완료
🚀 /pi 자동 실행 모드 — /c4-run 시작합니다
   (plan → run → finish 전체 자동 진행)
""")
    Skill("c4-run")

else:
    # 일반 모드: 사용자 확인 후 안내
    # "Proceed" -> 아래 전체 플로우를 안내하고 `/c4-run` 시작 유도
    # "Modify"  -> ask which part
    # "Cancel"  -> delete tasks, restart
    pass
```

**전체 실행 플로우 (반드시 이 순서로 안내)**:
```
/c4-run          # Worker 스폰 → 태스크 실행 → (체크포인트 처리) → polish → finish 자동 실행
/c4-finish       # c4-run이 완료되지 못한 경우 수동으로 마무리
```

> ℹ️ **Checkpoint는 c4-run에 내장되어 있다.**
> - `run.checkpoint_mode: interactive`(기본): CP 도달 시 중단 + 사용자가 `/c4-checkpoint` 호출
> - `run.checkpoint_mode: auto`: Worker가 자동으로 4-lens 리뷰 후 계속 진행
>
> ℹ️ **Polish와 Finish도 c4-run 완료 시 자동 실행된다.**
> c4-run이 끊겼거나 수동으로 마무리가 필요할 때만 `/c4-finish`를 별도 실행.

### Validation Checklist (must pass before confirmation)

- [ ] **Requirement clarity**: All requirements in EARS patterns?
- [ ] **DoD specificity**: All task DoDs verifiable?
- [ ] **Dependency validity**: No circular dependencies?
- [ ] **Scope definition**: Each task scope clear?
- [ ] **User approval**: Plan explicitly approved?

Recommended:
- [ ] Architecture decisions documented?
- [ ] Validation strategy defined (lint, unit, e2e)?
- [ ] Checkpoints at appropriate positions?
- [ ] Technical risks identified with mitigation plans?

---

## Flow Summary

```
/c4-plan
    |
Phase 0: Status display + knowledge_search (state, tasks, specs, designs, lighthouses, docs)
    |
Phase 0.5: Action selection
    |-> "Plan new feature"     -> Phase 1~2.7~3~4~4.5~4.9~5
    |-> "Review/modify"        -> Phase R (detail view -> edit)
    |-> "Lighthouse"           -> Phase L (register/list/promote/remove)
    |-> "Add tasks only"       -> Phase 4~4.5~4.9~5
    |-> "View status only"     -> End

"Plan new feature" 상세:
    Phase 1  : 문서 스캔
    Phase 2  : 문서 해석
    Phase 2.5: Discovery (EARS 요구사항)
              → FROM_PI=True이면 idea.md에서 자동 추출 (Q&A 생략)
    Phase 2.6: Design (아키텍처 결정)
    Phase 2.7: Lighthouse (계약 정의)
    Phase 3  : 개발 환경 인터뷰
    Phase 4  : Task Draft (텍스트만, DB 저장 금지)
        |
    Phase 4.5: Plan Critique Loop ← ─ ─ ─ ─ ┐
        |  Pre-Mortem 분석 (역할 전환)        │
        |  수정 (Critical/High/Medium)         │
        |  수렴 판정: 미수렴이면 round++ ──── ┘
        |  수렴 (max 3 rounds)
        ↓
    Phase 4.9: DB Commit (c4_add_todo 일괄 호출)
        |
    Phase 5  : Plan Confirmation (사용자 최종 승인)
```

## Related Skills

- `/c4-add-task` — add individual task
- `/c4-run` — start execution
- `/c4-status` — check status
- `/c4-checkpoint` — review checkpoint

---
name: plan
description: |
  ★ Create structured implementation plans for C4 projects. Scans project state,
  specs, designs, docs, and Lighthouse contracts, then guides through Discovery
  (EARS requirements), Design (architecture decisions), Lighthouse contracts
  (contract-first TDD), and task breakdown with DoD. Use when the user wants to
  plan features, review existing plans, manage Lighthouse contracts, or create
  implementation tasks. Triggers: "계획 수립", "기능 설계", "태스크 분해",
  "plan this feature", "create implementation plan", "design this",
  "break down requirements", "set up tasks".
allowed-tools: Read, Glob, Grep, Agent, Skill, mcp__cq__*
---

# C4 Plan Mode

Structured project planning: Discovery, Design, Lighthouse, and Task creation.

## Critical Rules

### MUST NOT write code before user explicitly approves the plan

```
Forbidden: Writing code during plan, "Let's try and adjust", executing without confirmation
Required: Plan summary -> user "proceed" -> code. Unclear? Ask first.
```

### Main session plans only; Workers implement

```
Forbidden: Adding tasks then writing code yourself
Required: Create tasks -> /c4-run -> Workers execute. Main monitors/reviews.
```

---

## Flags (--from-pi / --auto-run)

```python
args = "$ARGUMENTS"
FROM_PI = "--from-pi" in args
AUTO_RUN = "--auto-run" in args
if FROM_PI:
    PI_SLUG = args.split("--from-pi", 1)[1].strip().split()[0]
    idea_path = f".c4/ideas/{PI_SLUG}.md"
```

---

## Phase 0: Context Display

### 0.0 Config 읽기 (필수)

```python
cfg = c4_config_get(section="all")
critique_cfg = cfg.get("planning", {}).get("critique_loop", {})
CRITIQUE_ENABLED = critique_cfg.get("enabled", True)
CRITIQUE_MAX_ROUNDS = critique_cfg.get("max_rounds", 3)
CRITIQUE_MODE = critique_cfg.get("mode", "auto")  # auto | interactive | skip
LOOP_ACTIVE = CRITIQUE_ENABLED and CRITIQUE_MODE != "skip"
```

### 0.1 Data Collection

```
1. c4_status()           — project state, tasks, progress
2. c4_list_specs()       — saved specifications
3. c4_list_designs()     — saved designs
4. c4_lighthouse(list)   — tool contracts
5. Glob docs/**/*.md     — planning documents
6. c4_knowledge_search(query="{feature}")  — past patterns
7. c4_pattern_suggest(context="{domain}")  — recurring patterns
```

### 0.2 Rich Status Output

Display project context. For ASCII templates, see `references/output-format.md`.

Sections: Project Overview, Current State, Task Dependency Graph, Specifications,
Designs, Lighthouse, Planning Documents, Tech Stack.

### 0.3 Dependency Graph Rendering

Show dependency chains for pending tasks. Status icons: completed, in_progress, pending, blocked.

---

## Phase 0.5: Action Selection

```python
if FROM_PI:
    → Phase 2.5  # 이미 "Plan new feature"가 확정 — 선택 불필요
```

```python
AskUserQuestion(questions=[{
    "question": "What would you like to do?",
    "header": "Action",
    "options": [
        {"label": "Plan new feature", "description": "Discovery -> Design -> Lighthouse -> Tasks"},
        {"label": "Review/modify existing plan", "description": "View and edit saved Spec/Design"},
        {"label": "Lighthouse management", "description": "Register/list/promote/remove contracts"},
        {"label": "Add tasks only", "description": "Create tasks from existing design"},
        {"label": "View status only", "description": "Done after display"}
    ]
}])
```

| Selection | Next Phase |
|-----------|-----------|
| Plan new feature | Phase 1 |
| Review/modify | Phase R — see `references/output-format.md` |
| Lighthouse | Phase L — `c4_lighthouse(action=register/list/promote/remove)` |
| Add tasks only | Phase 4 |
| View status only | End |

---

## Phase 1: Planning Document Scan

Scan root and docs/ for `*.md` files with PRD/requirements/spec/plan keywords (>1KB).

## Phase 2: Document Interpretation

Extract: project overview, core features, tech stack, roadmap, architecture.

---

## Phase 2.5: Discovery (EARS Requirements)

### FROM_PI 모드

`FROM_PI=True`: idea.md EARS 섹션을 직접 파싱 → `c4_save_spec()` → Phase 2.6. 인터뷰 생략.
`FROM_PI=False`: 아래 인터뷰 진행.

### 2.5.1 Domain Auto-Detection

Analyze project structure. For detection rules, see `references/domain-templates.md`.
Confirm with user (FROM_PI=False only).

### 2.5.2 EARS Requirements Collection (FROM_PI=False only)

| Pattern | Format |
|---------|--------|
| Ubiquitous | "The system shall ~" |
| Event-Driven | "When ~, the system shall ~" |
| State-Driven | "While ~, the system shall ~" |
| Optional | "If ~ is enabled, the system shall ~" |
| Unwanted | "If ~ (error), the system shall ~" |

For domain-specific questions, see `references/domain-templates.md`.

### 2.5.3 Save Specification

```python
c4_save_spec(name=feature_slug, content="feature/domain/requirements/non_functional/out_of_scope YAML")
```

### 2.5.4 Verification Requirements

| Domain | Default Verification |
|--------|---------------------|
| web-frontend | browser (E2E), visual |
| web-backend | http (API), cli (server) |
| ml-dl | cli (inference), metrics |
| infra | cli (terraform), dryrun |

### 2.5.5 Discovery Complete

```python
c4_discovery_complete()  # Transitions to DESIGN state
```

---

## Phase 2.6: Design (Architecture Decisions)

For domain-specific templates, see `references/domain-templates.md`.

1. **Architecture Options** — id, name, complexity, pros/cons, recommended
2. **Component Design** — name, type, responsibilities, dependencies, interfaces
3. **Data Flow + Mermaid** — sequence/flow diagrams
4. **Design Decisions** — id (DEC-XXX), question, decision, rationale, alternatives

### Save Design

```python
c4_save_design(name="feature", content="options/selected/components/decisions/mermaid YAML")
```

### Design Complete

```python
c4_design_complete()  # Transitions to PLAN state
```

---

## Phase 2.65: Conflict Gate

소프트 게이트 — 워커/스펙/지식 충돌 감지. 상세: `references/conflict-gate.md`

충돌 없으면 조용히 통과. 충돌 있어도 사용자가 무시하고 진행 가능.

---

## Phase 2.7: Contract-First Lighthouse

> "Define interface first, implement second" (TDD approach).

| Type | Lighthouse? |
|------|-------------|
| New MCP tool | MUST register |
| New API endpoint | Register as tool wrapper |
| New service interface | Register if externally exposed |
| Internal helper | NOT needed |

```python
c4_lighthouse(action="register", name=tool_name, description=desc,
              input_schema=json.dumps(schema), spec=spec_text, auto_task=True)
```

---

## Phase 3: Development Environment Interview

For domain-specific questions, see `references/domain-templates.md`.

Ask about: language/build/package manager, test frameworks, linting tools,
checkpoint placement, task granularity.

---

## Phase 4: Task Draft (DB 커밋 전)

> ⚠️ c4_add_todo 호출 금지. 텍스트 초안만. DB 커밋은 Phase 4.9에서만.

For Worker Packet format and DoD principles, see `references/worker-packet.md`.

### Core Rules

1. PRD checklist → individual tasks, `scope` = affected files
2. `dod` MUST be specific and verifiable, `dependencies` respect order
3. Worker Packet format recommended

### DoD Quality Rules

- **Verifiable**: "X works", "returns Y", "test passes"
- **Specific**: No vague terms ("improve", "optimize")
- **Goal-Driven**: 명령형 → 선언형 변환 (테스트 작성 → 통과)

### QualityGates 표준 (CRITICAL)

> `go test ./...` 단독은 테스트 파일 없어도 통과 → false positive 방지 필수.

```
- test -f internal/foo/foo_test.go     # 파일 존재 확인
- go test ./internal/foo/...           # 해당 파일만 실행
```

### Task Size: APIs ≤3, Files ≤5, Domains = 1. 초과 시 분할 권고.

### Task Draft Format

```
[DRAFT] T-001-0: Task title
  scope: src/path/
  dod: |
    Goal: ...
    Rationale: ...
    Assumptions: [전제사항]
    ContractSpec: API + Tests
    QualityGates: test -f ... && go test ...
  dependencies: []
```

> **CP deps 규칙**: CP dependencies에 R- 태스크만. T- 직접 연결 금지 (리뷰 우회).
> 올바른: `T-001 → R-001 → CP-001`. 잘못된: `T-001 → CP-001`.

---

## Phase 4.5: Plan Critique Loop

```python
# 소규모 태스크 자동 skip
if len(draft_tasks) <= 3:
    print(f"⏭️  태스크 {len(draft_tasks)}개 — critique skip (소규모 작업)")
    → Phase 4.9
```

Worker 기반 Pre-Mortem 분석. 상세: `references/critique-loop.md`

매 라운드마다 fresh Worker 스폰 → 비판 → 수정 → 수렴 판정 (max 3 rounds).
`planning.critique_loop.mode: skip`으로 비활성화 가능.

---

## Phase 4.9: DB Commit

수렴 확정된 draft를 `c4_add_todo()`로 일괄 기록.
알림: `c4_notify(message='계획 확정', event='plan.created')`

### 설계 결정 지식 저장

Plan 단계에서 도출된 핵심 설계 결정을 지식 베이스에 기록한다.
다음 세션에서 "왜 이렇게 설계했는지" 검색 가능하게 한다.

```python
# Design 단계에서 결정된 핵심 사항만 기록 (전체 spec/design은 파일에 이미 있음)
if design_decisions:  # DEC-XXX 목록
    summary = "\n".join(f"- {d.id}: {d.question} → {d.decision} ({d.rationale})"
                        for d in design_decisions[:5])
    c4_knowledge_record(
        title=f"{feature_slug} — 설계 결정",
        content=f"## 설계 결정\n{summary}\n\n## 선택한 아키텍처\n{selected_option}",
        domain=domain,
        tags=[feature_slug, "design-decision"]
    )
```

---

## Phase 4.95: Spec Scenarios (동작정의서)

> 태스크 확정 후, 실행 전에 "이 기능이 뭘 하는가"를 시나리오로 정의한다.
> 기획자/개발자/QA가 구현 전에 동작을 합의하는 계약서 역할.

### 조건

```python
task_count = len(created_tasks)  # Phase 4.9에서 생성된 태스크 수
if task_count <= 3:
    print("⏭️  소규모 작업 — 시나리오 생성 skip")
    → Phase 5
```

### 시나리오 생성

기존 `docs/specs/{slug}.md`(Phase 2.5에서 저장된 EARS spec)에 시나리오 섹션을 추가한다.

```python
spec_path = f"docs/specs/{feature_slug}.md"
spec_content = c4_read_file(path=spec_path)

# idea.md가 있으면 시나리오 힌트로 활용
if FROM_PI:
    idea_content = c4_read_file(path=idea_path)
```

EARS 요구사항과 태스크 DoD를 분석하여 시나리오를 도출한다.
각 시나리오는 **WHEN-THEN-VERIFY 3계층** 형식:

```markdown
## 동작 시나리오

### S1: [시나리오 이름]
- WHEN: [사용자 행동 또는 시스템 이벤트]
- THEN: [기대 결과 — 사람이 읽는 자연어]
- VERIFY:
  - [기계가 검증 가능한 조건 1]
  - [기계가 검증 가능한 조건 2]

### S2: [에러 케이스]
- WHEN: [비정상 입력 또는 장애 상황]
- THEN: [에러 처리 — 사용자에게 보이는 것]
- VERIFY:
  - [에러 코드/메시지 조건]
```

**시나리오 도출 원칙**:
- EARS의 Event-Driven → 정상 시나리오 (S1~)
- EARS의 Unwanted → 에러 시나리오
- 태스크 DoD의 Success Criteria → VERIFY 조건
- 경계값, 동시성, 권한 → 추가 시나리오

### 에디터 열기

```python
if AUTO_RUN:
    → Phase 5  # 에디터 대기 불필요
```

```python
# spec 파일에 시나리오 섹션 추가 후 에디터에서 열기
import shutil
if shutil.which("code"):
    Bash(f"code '{spec_path}'")
elif shutil.which("open"):
    Bash(f"open '{spec_path}'")

print(f"""
📋 동작정의서 열렸습니다: {spec_path}
   시나리오를 검토하고 수정해주세요.
   WHEN-THEN은 사람용, VERIFY는 기계용입니다.
""")
```

### 테스트 매핑 테이블 초안

시나리오 생성 시 빈 테스트 매핑 테이블도 함께 추가한다:

```markdown
## 테스트 매핑
| 시나리오 | 테스트 | 상태 |
|---------|--------|------|
| S1 | (구현 후 자동 매핑) | ⏳ |
| S2 | (구현 후 자동 매핑) | ⏳ |
```

이 테이블은 `/finish`에서 테스트 실행 결과로 자동 갱신된다.

### 수정 감지 + 연쇄 갱신

에디터에서 사람이 파일을 수정하고 돌아오면 변경을 감지하여 처리한다.

**단방향 연쇄 원칙**:
```
idea 변경 → spec 재생성 → 태스크 DoD 재생성
spec 변경 → 태스크 DoD 갱신 (idea는 안 바뀜)
```

```python
# idea.md 수정 감지 (FROM_PI 모드)
if FROM_PI:
    idea_before = idea_content  # 에디터 열기 전 저장
    # ... 에디터 열기 + 사람 검토 ...
    idea_after = c4_read_file(path=idea_path)
    if idea_before != idea_after:
        print("📝 idea.md 수정 감지 — EARS + 시나리오 재생성")
        # EARS 재파싱 → c4_save_spec() 갱신
        # 시나리오 재생성 → spec.md 갱신
        # spec.md 다시 에디터 열기

# spec.md 수정 감지
spec_before = spec_content  # 에디터 열기 전 저장
# ... 에디터 열기 + 사람 검토 ...
spec_after = c4_read_file(path=spec_path)
if spec_before != spec_after:
    print("📝 spec.md 수정 감지 — 태스크 DoD 갱신")
    # 변경된 시나리오를 분석하여 관련 태스크 DoD에 반영
    # (이미 Phase 4.9에서 생성된 태스크가 있으면 dod 업데이트)

# 수정 없이 닫음 = 승인
if idea_before == idea_after and spec_before == spec_after:
    print("✅ idea + spec 승인 — 구현 진행")
```

---

## Phase 5: Plan Confirmation

```python
if AUTO_RUN:
    Skill("run")  # 자동 진행
else:
    # Proceed / Modify / Cancel 선택
```

전체 실행 플로우: `/run` → (checkpoint) → polish → finish 자동.

### Validation Checklist

- [ ] Requirements in EARS patterns?
- [ ] DoDs verifiable?
- [ ] No circular dependencies?
- [ ] Scope clear?
- [ ] User approved?

## Related Skills

- `/add-task` — add individual task
- `/run` — start execution
- `/status` — check status
- `/checkpoint` — review checkpoint

---
name: c4-plan
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

Worker 기반 Pre-Mortem 분석. 상세: `references/critique-loop.md`

매 라운드마다 fresh Worker 스폰 → 비판 → 수정 → 수렴 판정 (max 3 rounds).
`planning.critique_loop.mode: skip`으로 비활성화 가능.

---

## Phase 4.9: DB Commit

수렴 확정된 draft를 `c4_add_todo()`로 일괄 기록.
알림: `c4_notify(message='계획 확정', event='plan.created')`

---

## Phase 5: Plan Confirmation

```python
if AUTO_RUN:
    Skill("c4-run")  # 자동 진행
else:
    # Proceed / Modify / Cancel 선택
```

전체 실행 플로우: `/c4-run` → (checkpoint) → polish → finish 자동.

### Validation Checklist

- [ ] Requirements in EARS patterns?
- [ ] DoDs verifiable?
- [ ] No circular dependencies?
- [ ] Scope clear?
- [ ] User approved?

## Related Skills

- `/c4-add-task` — add individual task
- `/c4-run` — start execution
- `/c4-status` — check status
- `/c4-checkpoint` — review checkpoint

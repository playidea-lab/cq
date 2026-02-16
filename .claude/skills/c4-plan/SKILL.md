---
description: |
  Create structured implementation plans for C4 projects. Scans project state,
  specs, designs, docs, and Lighthouse contracts, then guides through Discovery
  (EARS requirements), Design (architecture decisions), Lighthouse contracts
  (contract-first TDD), and task breakdown with DoD. Use when the user wants to
  plan features, review existing plans, manage Lighthouse contracts, or create
  implementation tasks. Triggers on: "plan this feature", "create implementation
  plan", "design this", "break down requirements", "set up tasks".
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

## Phase 0: Context Display

### 0.1 Data Collection

Call these MCP tools to gather current state:

```
1. c4_status()           — project state, tasks, progress
2. c4_list_specs()       — saved specifications
3. c4_list_designs()     — saved designs
4. c4_lighthouse(list)   — tool contracts (stubs/implemented)
5. Glob docs/**/*.md     — planning documents
```

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

## Phase 4: Task Creation

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

| Bad DoD | Good DoD |
|---------|----------|
| "Implement login" | "Email/password login returns JWT, wrong password returns 401" |
| "Optimize API" | "GET /users response < 100ms, existing tests pass" |
| "Fix bug" | "null input returns empty array, add regression test" |

### Task Size Validation

| Metric | Max | If exceeded |
|--------|-----|-------------|
| Public APIs | 3 | Split recommended |
| Modified files | 5 | Split recommended |
| Domains | 1 | **Split required** |

If exceeded, ask user whether to split.

### Task Creation

```python
# T-XXX creates R-XXX review task automatically (review_required default true)
c4_add_todo(
    task_id="T-001-0",
    title="Task title",
    scope="src/path/",
    dod="Goal: ...\n\nContractSpec:\n  API: ...\n  Tests: ...\n\nCodePlacement:\n  Create: ...\n  Modify: ..."
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

Dependency tree: `T-XXX -> R-XXX -> CP-XXX -> T-YYY -> R-YYY`

---

## Phase 5: Plan Confirmation

Summarize the generated plan and confirm with user.

**Output**:
- Task count per phase
- Checkpoint count
- Validation strategy
- Task list per phase
- Next steps (/c4-run, /c4-status)

**Confirmation**:
- "Proceed" -> guide to /c4-run
- "Modify" -> ask which part
- "Cancel" -> delete tasks, restart

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
Phase 0: Status display (state, tasks, specs, designs, lighthouses, docs)
    |
Phase 0.5: Action selection
    |-> "Plan new feature"     -> Phase 1~2.7~5 (Discovery->Design->Lighthouse->Tasks)
    |-> "Review/modify"        -> Phase R (detail view -> edit)
    |-> "Lighthouse"           -> Phase L (register/list/promote/remove)
    |-> "Add tasks only"       -> Phase 4~5 (Tasks directly)
    |-> "View status only"     -> End
```

## Related Skills

- `/c4-add-task` — add individual task
- `/c4-run` — start execution
- `/c4-status` — check status
- `/c4-checkpoint` — review checkpoint

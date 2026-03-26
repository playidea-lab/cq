# Worker Packet Format

Structured task specification for C4 Workers. All implementation tasks should use this format.

---

## 7 Elements

| Element | Required | Description |
|---------|----------|-------------|
| **Goal** | Yes | Completion criteria + what is out-of-scope |
| **ContractSpec** | Yes | API specification + test specification |
| **LighthouseRef** | If exists | Lighthouse stub name for contract-first implementation |
| **BoundaryMap** | Recommended | DDD layer constraints, forbidden imports |
| **CodePlacement** | Recommended | Files to create, modify, and test |
| **QualityGates** | Recommended | Validation commands (lint, test, import check) |
| **Checkpoints** | Recommended | CP1/CP2/CP3 incremental milestones |

---

## ContractSpec Detail

### API Specification

```yaml
API:
  name: ServiceName.methodName
  input: "param1: type, param2: type"
  output: "ReturnType | None"
  errors: [ErrorType1, ErrorType2]
  side_effects: "Description of side effects"
```

### Test Specification

Minimum 3 tests per API:

```yaml
Tests:
  success:
    - test_method_creates_resource
  failure:
    - test_method_duplicate_error
    - test_method_invalid_input
  boundary:
    - test_method_max_length
```

---

## BoundaryMap Detail

```yaml
BoundaryMap:
  Domain: auth
  Layer: app  # domain | app | infra | api
  Allowed: [stdlib, pydantic, domain.user]
  Forbidden: [sqlalchemy, httpx, fastapi]
  PublicExport: src/api/v1/users.py
```

Layer rules:
- **domain**: No external dependencies. Pure business logic.
- **app**: Can reference domain. Use cases.
- **infra**: Can reference all layers. DB/external API.
- **api**: Controllers/routers. Entry points.

---

## CodePlacement Detail

```yaml
CodePlacement:
  Create:
    - src/auth/service.py
    - src/auth/domain/user.py
  Modify:
    - src/api/v1/users.py
  Tests:
    - tests/unit/auth/test_service.py
```

---

## QualityGates Detail

```yaml
QualityGates:
  - name: lint
    command: "uv run ruff check ."
    required: true
  - name: unit
    command: "uv run pytest tests/unit/auth/ -v"
    required: true
  - name: forbidden_imports
    command: "uv run python scripts/check_imports.py src/auth/"
    required: true
```

---

## DoD (Definition of Done) Principles

### Three Rules

1. **Verifiable**: "X works", "returns Y", "test T passes"
2. **Specific**: No vague terms ("improve", "optimize", "enhance")
3. **Independent**: Checkable without other tasks completing first

### Good vs Bad Examples

| Bad DoD | Good DoD |
|---------|----------|
| "Implement login" | "Email/password login returns JWT token, wrong password returns 401 error" |
| "Optimize API" | "GET /users response time 500ms -> 100ms or less, existing tests pass" |
| "Fix bug" | "null input returns empty array instead of error, add regression test" |
| "Improve UI" | "Button click shows loading spinner, completion shows success message" |
| "Clean up code" | "Delete 3 unused functions, lint errors = 0" |

### DoD Checklist

- [ ] Can a Worker read this and immediately start implementing?
- [ ] Can completion be objectively determined?
- [ ] Can it be verified by test or manual check?

---

## Task Size Validation

| Metric | Maximum | If Exceeded |
|--------|---------|-------------|
| Time estimate | 2 days | Split required |
| Public APIs | 3 | Split recommended |
| Test count | 9 | Split recommended |
| Modified files | 5 | Split recommended |
| Domains | 1 | **Split required** |

When exceeded, ask user:
```python
AskUserQuestion(questions=[{
    "question": f"Task is too large (APIs: {n}, files: {m}). Split?",
    "header": "Split",
    "options": [
        {"label": "Split (recommended)", "description": "Break into smaller tasks"},
        {"label": "Keep as-is", "description": "Proceed with large task"}
    ],
    "multiSelect": False
}])
```

---

## Complete Example

```python
c4_add_todo(
    task_id="T-001-0",
    title="Implement user registration",
    scope="src/auth/",
    dod="""
Goal: POST /v1/users creates a new User and returns user object
  Out-of-scope: email verification, password reset

ContractSpec:
  API: UserService.register(email: str, password: str) -> User | None
  Errors: DuplicateEmail, WeakPassword
  Tests:
    - success: test_register_creates_user
    - failure: test_register_duplicate_email, test_register_weak_password
    - boundary: test_register_max_email_length

LighthouseRef: c4_user_register (if registered)

BoundaryMap:
  Domain: auth / Layer: app
  Forbidden: sqlalchemy, httpx

CodePlacement:
  Create: src/auth/service.py, src/auth/domain/user.py
  Modify: src/api/v1/users.py
  Tests: tests/unit/auth/test_service.py

QualityGates:
  - lint: uv run ruff check src/auth/
  - unit: uv run pytest tests/unit/auth/ -v

Checkpoints:
  CP1: File structure + test skeleton created
  CP2: register() success test passes
  CP3: Failure + boundary tests added and passing
"""
)
# Result: { task_id: "T-001-0", review_task_id: "R-001-0" }
```

### Task Dependencies

```
T-001-0 -> R-001-0 --\
T-002-0 -> R-002-0 --+--> CP-001 -> T-003-0 -> R-003-0
```

### Review Revision

```
R-001-0 REQUEST_CHANGES -> T-001-1 (fix) -> R-001-1 (re-review)
                           (blocked if max_revision exceeded)
```

### CP Task Example

```python
c4_add_todo(
    task_id="CP-001",
    title="Phase 1 checkpoint",
    dod="Phase 1 implementation + review complete",
    dependencies=["R-001-0", "R-002-0"],
    review_required=False
)
```

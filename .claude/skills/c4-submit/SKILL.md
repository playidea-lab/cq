---
description: |
  Submit completed C4 tasks with automated validation (lint, unit tests, DDD-CLEANCODE
  checks). Auto-detects in-progress tasks, runs required validators, verifies commit SHA,
  and triggers checkpoint reviews when needed. Use when task implementation is done.
  Triggers on: "submit task", "complete task", "submit T-XXX", "/c4-submit".
---

# C4 Submit Task

Submit the current in-progress task with validation and commit verification.

## Usage

```
/c4-submit [task-id]
```

If task-id is omitted, auto-detects the current in-progress task.

## Workflow

### 1. Check Current State

```python
status = mcp__c4__c4_status()
```

- Identify current worker's in_progress tasks
- If multiple, present list for user selection

### 2. Auto-detect Task

If `$ARGUMENTS` is empty:

```
User: /c4-submit

Claude: Current in-progress task:
  - T-003: Implement login page
    DoD: JWT auth, social login support
    Scope: src/auth/

  Submit this task?
```

Multiple tasks in progress:

```
Claude: Which task to submit?
  1. T-003: Implement login page
  2. T-004: Signup API

User: 1
```

### 3. Run Basic Validation

Auto-run validation before submission:

```
Claude: Running validation...
  - lint: ✅ pass
  - unit: ✅ pass

  All validations passed! Proceed with submission?
```

Validation failure:

```
Claude: Running validation...
  - lint: ✅ pass
  - unit: ❌ fail
    Error: test_login_success failed

  Cannot submit due to validation failure.
  Please fix and retry.

  [Let me know if you need help]
```

### 3.5 DDD-CLEANCODE Validation (Worker Packet Tasks)

If task includes Worker Packet specs (BoundaryMap, ContractSpec), run additional checks.

#### 3.5.1 Boundary Validation

Check import rule violations if BoundaryMap is defined:

```python
from c4.validators.boundary import validate_boundary, format_violations_report
from c4.models.ddd import BoundaryMap
from pathlib import Path

if task.boundary_map:
    files = [Path(f) for f in task.code_placement.create + task.code_placement.modify]
    result = validate_boundary(files, task.boundary_map, project_root=Path("."))

    if not result.valid:
        print(format_violations_report(result.violations))
```

**On violation:**

```
Claude: 🔴 Boundary validation failed!

❌ Found 2 boundary violations:

📁 src/auth/service.py
   Line 5: sqlalchemy
   └─ Forbidden import: sqlalchemy

📁 src/auth/domain/user.py
   Line 3: httpx
   └─ Forbidden import: httpx

⚠️ BoundaryMap rules:
  - target_layer: app
  - forbidden_imports: [sqlalchemy, httpx, fastapi]

Cannot submit due to boundary violations.
Remove infra dependencies from domain layer.
```

#### 3.5.2 Work Breakdown Validation

Check task size compliance with DDD-CLEANCODE guidelines:

```python
from c4.validators.work_breakdown import analyze_task_size, format_breakdown_report

result = analyze_task_size(task)

if not result.valid:
    print(format_breakdown_report(result))
```

**On threshold exceeded:**

```
Claude: ⚠️ Work breakdown validation warning!

❌ Task should be split:

📊 Metrics:
  - APIs: 5 (max: 3) ⚠️
  - Tests: 12 (max: 9) ⚠️
  - Files: 4 (within limit: 5)
  - Domains: 1 (within limit: 1)

📋 Recommendations:
  - [must_split] Too many APIs (5 > 3)
  - [should_split] Too many tests (12 > 9)

Suggestion: Split into 2-3 tasks.
Example: UserService.register + UserService.login → separate tasks

Still submit? (force submission possible but not recommended)
```

#### 3.5.3 ContractSpec Validation

Check minimum test requirements if ContractSpec is defined:

```
Claude: 📋 ContractSpec validation...

API specs:
  - UserService.register ✅
  - UserService.login ✅

Test specs:
  - success: test_register_success ✅
  - failure: test_register_duplicate_email ✅
  - boundary: test_register_max_length ✅

✅ ContractSpec requirements met!
```

**Missing tests:**

```
Claude: ❌ ContractSpec tests missing!

Required tests:
  - success: test_register_success ✅
  - failure: (none) ❌
  - boundary: (none) ❌

Need at least 1 failure test and 1 boundary test.
```

### 4. Verify Commit SHA

```bash
commit_sha = git rev-parse HEAD
```

Confirm recent commit:

```
Claude: Submitting with recent commit:
  - SHA: abc123d
  - Message: "feat: implement login page"

  Correct? Type 'confirm' or provide different SHA.
```

### 5. Execute Submission

```python
mcp__c4__c4_submit(
    task_id=task_id,
    commit_sha=commit_sha,
    validation_results=[
        # Basic validation
        {"name": "lint", "status": "pass"},
        {"name": "unit", "status": "pass"},
        # DDD-CLEANCODE validation (if Worker Packet present)
        {"name": "boundary", "status": "pass"},
        {"name": "work_breakdown", "status": "pass"},
        {"name": "contract_spec", "status": "pass"},
    ]
)
```

> **Note**: DDD-CLEANCODE validation runs only if task includes Worker Packet specs
> (BoundaryMap, ContractSpec, etc.). Without specs, only basic validation (lint, unit).

### 6. Result & Next Steps

```
Claude: ✅ T-003 submitted!

  Next steps:
  - Pending tasks: 2 (T-004, T-005)
  - Run /c4-run to start next task
```

Checkpoint reached:

```
Claude: ✅ T-003 submitted!

  🎯 Checkpoint CP-001 conditions met!
  Waiting for Supervisor review...

  Check status with /c4-checkpoint
```

## Conversation Examples

### Basic Flow

```
User: /c4-submit

Claude: Submit current T-003 (login page)?

User: yes

Claude: Running validation...
  - lint: ✅ pass
  - unit: ✅ pass

  Submitting with commit abc123d. Proceed?

User: yes

Claude: ✅ T-003 submitted!
  Next task: T-004 (signup API)
```

### Quick Submit

```
User: /c4-submit T-003

Claude: Validating T-003...
  - lint: ✅ pass
  - unit: ✅ pass

  ✅ T-003 submitted!
```

### Validation Failure

```
User: /c4-submit

Claude: Validating T-003...
  - lint: ✅ pass
  - unit: ❌ fail

  Test failures:
  - test_login_invalid_password: AssertionError

  Fix needed. Let me know if you need help.

User: fix it

Claude: [Analyzes test error and fixes code...]
```

### DDD-CLEANCODE Validation (Worker Packet)

```
User: /c4-submit

Claude: Validating T-003...
  - lint: ✅ pass
  - unit: ✅ pass

  📋 DDD-CLEANCODE validation...
  - boundary: ✅ pass (no forbidden imports)
  - work_breakdown: ✅ pass (2 APIs, 6 tests)
  - contract_spec: ✅ pass (success/failure/boundary tests)

  ✅ All validations passed! Submitting with commit abc123d.

User: yes

Claude: ✅ T-003 submitted!
```

### Boundary Violation

```
User: /c4-submit

Claude: Validating T-003...
  - lint: ✅ pass
  - unit: ✅ pass

  📋 DDD-CLEANCODE validation...
  - boundary: ❌ fail

  🔴 Boundary violation detected!
  📁 src/auth/service.py
     Line 5: sqlalchemy
     └─ Forbidden import: sqlalchemy (prohibited in app layer)

  App layer cannot use sqlalchemy directly.
  Access via Repository interface.

User: fix it

Claude: Applying Repository pattern to decouple dependency...
  [Create UserRepositoryImpl in infra layer]
  [Reference UserRepository interface in app layer]

  Re-validating...
  - boundary: ✅ pass

  Fix complete! Submit?
```

## Auto-help on Validation Failure

When user requests help after validation failure:

### Basic Validation Failure (lint, unit)
1. Analyze error message
2. Check relevant code
3. Suggest fix or apply directly
4. Re-validate and submit

### DDD-CLEANCODE Validation Failure

| Validator | Failure Cause | Auto-help |
|----------|---------------|-----------|
| **boundary** | Forbidden import | Apply Repository pattern, suggest dependency inversion |
| **work_breakdown** | Task size exceeded | Suggest task split, separate by domain |
| **contract_spec** | Missing tests | Generate required test cases |

#### Boundary Violation Auto-fix Example

```
Violation: App layer directly using sqlalchemy

Fix:
1. Create infra/repositories/user_repository.py (implementation)
2. Create domain/interfaces/user_repository.py (interface)
3. app/services/user_service.py references interface only
```

#### Work Breakdown Suggestion Example

```
Current: T-003 (5 APIs, 15 tests)

Suggested split:
- T-003-a: UserService.register + related tests (2 APIs, 6 tests)
- T-003-b: UserService.login + related tests (2 APIs, 6 tests)
- T-003-c: UserService.logout + related tests (1 API, 3 tests)
```

## After Submission

System automatically:
- Marks task as completed
- Releases scope lock
- Checks checkpoint conditions
- Triggers Supervisor review if needed

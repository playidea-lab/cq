# DDD-CLEANCODE Validation (Worker Packet Tasks)

Runs only if task includes Worker Packet specs (BoundaryMap, ContractSpec).

## Boundary Validation

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

**On violation**: Report forbidden imports with file:line, BoundaryMap rules, and suggest Repository pattern fix.

## Work Breakdown Validation

Check task size compliance:

```python
from c4.validators.work_breakdown import analyze_task_size, format_breakdown_report
result = analyze_task_size(task)
```

Thresholds: APIs max 3, Tests max 9, Files max 5, Domains max 1.
On exceed: suggest task split with examples.

## ContractSpec Validation

Check minimum test requirements (success + failure + boundary tests).
Missing tests block submission.

## Auto-help on Validation Failure

| Validator | Failure Cause | Auto-help |
|----------|---------------|-----------|
| **boundary** | Forbidden import | Apply Repository pattern, dependency inversion |
| **work_breakdown** | Task size exceeded | Suggest task split by domain |
| **contract_spec** | Missing tests | Generate required test cases |

### Boundary Violation Auto-fix

```
Violation: App layer directly using sqlalchemy
Fix:
1. Create infra/repositories/user_repository.py (implementation)
2. Create domain/interfaces/user_repository.py (interface)
3. app/services/user_service.py references interface only
```

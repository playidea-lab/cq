# C4 Run Validation

Run project validations (lint, tests, security, etc.).

## Instructions

1. **Parse Arguments**: Identify validation names from arguments (e.g., "lint", "unit").
2. **Run Validations**: Call `c4_run_validation(names)` with the selected names.
3. **Display Results**:
   - Show pass/fail status for each validation.
   - Highlight **CRITICAL** or **HIGH** severity issues.
4. **Action on Failure**: If validations fail, suggest fixes or mark task as blocked if persistent.

## Severity Levels

| Severity | Action | Description |
|----------|--------|-------------|
| **CRITICAL** | Block Commit | Immediate fix required (e.g., hardcoded secrets) |
| **HIGH** | Review Required | Must be checked during PR review |
| **MEDIUM** | Recommended | Style improvements, docs |

## Usage

```
/c4-validate
/c4-validate lint unit
```
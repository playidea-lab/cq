# C4 Run Validations

Run project validations (lint, tests, etc.).

## Arguments

```
/c4-validate [validation-names...]
```

- If no names provided, runs all required validations
- Can specify specific validations: `lint`, `unit`, `integration`, etc.

## Instructions

1. Parse validation names from `$ARGUMENTS`
2. If no names provided, use all required validations from config
3. Call `mcp__c4__c4_run_validation(names)`
4. Display results:
   - Each validation name and status (pass/fail)
   - Duration
   - Error output if failed
5. Show summary: X/Y validations passed

## Usage

```
/c4-validate          # Run all required validations
/c4-validate lint     # Run only lint
/c4-validate lint unit  # Run lint and unit tests
```

## Configuration

Validations are configured in `.c4/config.yaml`:

```yaml
validation:
  commands:
    lint: uv run ruff check src/ tests/
    unit: uv run pytest tests/ -v
  required:
    - lint
    - unit
```

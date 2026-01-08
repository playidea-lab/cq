# 커스텀 Validator 가이드

이 문서는 커스텀 검증 명령을 추가하는 방법을 설명합니다.

## 개요

C4는 설정 파일을 통해 검증 명령을 커스터마이즈할 수 있습니다.

## 기본 검증 명령

```yaml
# .c4/config.yaml

validations:
  lint:
    command: "uv run ruff check"
    description: "코드 스타일 검사"

  unit:
    command: "uv run pytest tests/unit"
    description: "단위 테스트"

  integration:
    command: "uv run pytest tests/integration"
    description: "통합 테스트"

  e2e:
    command: "uv run pytest tests/e2e"
    description: "E2E 테스트"
```

## 커스텀 검증 추가

### 예시: Type Checking

```yaml
validations:
  typecheck:
    command: "uv run mypy src/"
    description: "타입 체크"
```

### 예시: Security Scan

```yaml
validations:
  security:
    command: "uv run bandit -r src/"
    description: "보안 취약점 스캔"
```

### 예시: Coverage Check

```yaml
validations:
  coverage:
    command: "uv run pytest --cov=src --cov-fail-under=80"
    description: "테스트 커버리지 80% 이상"
```

## 검증 사용

### 특정 검증 실행

```
/c4-validate lint typecheck
```

### 모든 검증 실행

```
/c4-validate
```

## 체크포인트와 연동

```yaml
# docs/PLAN.md

### CP-001: Phase 1 완료
- required_tasks: [T-001, T-002]
- required_validations: [lint, unit, typecheck]  # 커스텀 검증 포함
```

## 검증 결과 형식

검증 결과는 다음 형식으로 저장됩니다:

```json
{
  "lint": "pass",
  "unit": "pass",
  "typecheck": "fail"
}
```

## 고급 설정

### 환경 변수

```yaml
validations:
  test:
    command: "uv run pytest"
    env:
      CI: "true"
      DATABASE_URL: "sqlite:///test.db"
```

### 타임아웃

```yaml
validations:
  e2e:
    command: "uv run pytest tests/e2e"
    timeout: 600  # 10분
```

### 조건부 실행

```yaml
validations:
  deploy-check:
    command: "./scripts/deploy-check.sh"
    only_on:
      - CHECKPOINT  # 체크포인트에서만 실행
```

## 스크립트 기반 검증

### 복잡한 검증 로직

```bash
#!/bin/bash
# scripts/custom-check.sh

set -e

echo "Running custom checks..."

# 1. 라이선스 체크
grep -r "MIT License" LICENSE || exit 1

# 2. 문서 체크
test -f README.md || exit 1

# 3. 의존성 체크
uv sync --dry-run || exit 1

echo "All checks passed!"
```

```yaml
validations:
  custom:
    command: "./scripts/custom-check.sh"
```

## 다음 단계

- [명령어 레퍼런스](../user-guide/명령어-레퍼런스.md) - /c4-validate 상세
- [MCP 도구 레퍼런스](../api/MCP-도구-레퍼런스.md) - c4_run_validation API

---
name: test-first
description: |
  TDD 사이클 가이드. RED(실패 테스트 작성) → GREEN(최소 구현) → REFACTOR 순서로 진행.
  트리거: "TDD", "test-first", "테스트 먼저", "RED GREEN REFACTOR", "테스트 주도"
allowed-tools: Read, Write, Edit, Bash, Glob, Grep
---
# Test First (TDD)

RED → GREEN → REFACTOR 사이클로 기능을 구현합니다.

## 사이클

```
RED   → 실패하는 테스트를 먼저 작성
GREEN → 테스트를 통과하는 최소한의 코드 작성
REFACTOR → 중복 제거, 가독성 개선 (테스트는 여전히 통과해야 함)
```

## Step 1: RED — 실패 테스트 작성

구현 전에 테스트부터 작성한다.

```go
// Go 예시
func TestCalculate_WhenInputNegative_ReturnsError(t *testing.T) {
    result, err := Calculate(-1)
    assert.Error(t, err)
    assert.Zero(t, result)
}
```

```python
# Python 예시
def test_calculate_when_input_negative_returns_error():
    with pytest.raises(ValueError):
        calculate(-1)
```

테스트 실행 → 반드시 실패해야 함:

# CUSTOMIZE: 프로젝트 테스트 실행 명령으로 교체하세요
```bash
# Go
go test ./... -run TestCalculate
# Python
uv run pytest tests/ -k "test_calculate"
# Node
pnpm test -- --grep "calculate"
```

## Step 2: GREEN — 최소 구현

테스트를 통과하는 가장 단순한 코드를 작성한다.

- 아름다울 필요 없음. 통과하면 됨.
- 과도한 추상화 금지. 지금 테스트만 통과시킨다.

테스트 재실행 → 통과 확인.

## Step 3: REFACTOR — 개선

테스트가 통과된 상태에서 코드를 정리한다.

- 중복 제거
- 변수명 개선
- 함수 분리

리팩토링 후 테스트 재실행 → 여전히 통과 확인.

## 완료 기준 (DoD)

- [ ] 모든 새 기능에 테스트 존재
- [ ] 테스트가 의도를 명확히 설명
- [ ] 에러 경로도 테스트됨
- [ ] 테스트가 독립적으로 실행 가능

## 주의

- 테스트 없는 GREEN 금지
- REFACTOR 중 로직 변경 금지
- 한 사이클에 하나의 행동만

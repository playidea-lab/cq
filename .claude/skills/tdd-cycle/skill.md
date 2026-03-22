---
name: tdd-cycle
description: |
  TDD(Test-Driven Development) 사이클 가이드. RED-GREEN-REFACTOR 루프를 체계적으로 진행합니다.
  새 기능 구현, 버그 수정, 리팩토링 시 테스트 먼저 작성하고 싶을 때 이 스킬을 사용하세요.
  "TDD", "테스트 먼저", "test driven", "RED GREEN REFACTOR", "테스트 주도 개발",
  "실패하는 테스트부터" 등의 요청에 트리거됩니다.
---

# TDD Cycle

RED → GREEN → REFACTOR 반복 루프.

## 트리거

"TDD", "테스트 먼저", "test driven", "RED GREEN REFACTOR", "테스트 주도 개발"

## 철학

> "코드를 작성하기 전에 실패하는 테스트를 먼저 작성한다."

TDD는 설계 도구다. 테스트는 "검증"이 아니라 "명세"다.
테스트를 먼저 쓰면 인터페이스를 사용자 관점에서 설계하게 된다.

## Steps

### 1. RED — 실패하는 테스트 작성

```
목표: 원하는 동작을 테스트로 표현한다.
결과: 테스트가 실패한다 (컴파일 에러 또는 assertion 실패).
```

**규칙:**
- 한 번에 하나의 행동만 테스트
- 테스트 이름으로 의도를 표현: `Test_CreateUser_ReturnsErrorOnDuplicateEmail`
- 아직 존재하지 않는 함수/메서드를 호출해도 됨 (인터페이스 설계)
- 가장 단순한 케이스부터 시작

```go
// 예시: 아직 구현이 없는 상태에서 테스트 작성
func Test_CalculateDiscount_ReturnsZeroForNewUser(t *testing.T) {
    discount := CalculateDiscount(User{IsNew: true}, 10000)
    if discount != 0 {
        t.Errorf("expected 0, got %d", discount)
    }
}
```

### 2. GREEN — 최소한으로 통과시킨다

```
목표: 테스트를 통과하는 가장 단순한 코드를 작성한다.
결과: 테스트가 통과한다.
```

**규칙:**
- 하드코딩도 OK (일단 통과시키는 것이 목적)
- "나중에 이것도 필요할 것 같은데" → 쓰지 않는다 (YAGNI)
- 기존 테스트가 깨지면 안 됨
- 완벽한 설계는 REFACTOR에서

```go
func CalculateDiscount(user User, amount int) int {
    return 0  // 하드코딩 OK — 다음 테스트에서 일반화
}
```

### 3. REFACTOR — 설계를 개선한다

```
목표: 코드의 구조를 개선한다 (동작은 변경하지 않음).
결과: 모든 테스트가 여전히 통과한다.
```

**규칙:**
- 동작 변경 금지 — 구조만 변경
- 중복 제거, 명명 개선, 추상화 추출
- 리팩토링 후 반드시 테스트 실행
- 테스트 코드도 리팩토링 대상

### 4. 반복

```
RED → GREEN → REFACTOR → RED → GREEN → REFACTOR → ...

각 사이클: 5-15분 (길어지면 범위가 너무 넓은 신호)
```

**점진적 일반화:**
1. 가장 단순한 케이스 (하드코딩으로 통과)
2. 두 번째 케이스 (조건문 추가)
3. 세 번째 케이스 (패턴 발견 → 일반화)

## 테스트 순서 전략

| 순서 | 이유 |
|------|------|
| 1. Happy path (정상 케이스) | 핵심 동작 확인 |
| 2. Edge case (경계값) | 0, null, empty, max |
| 3. Error path (에러 케이스) | 잘못된 입력, 실패 시나리오 |
| 4. Integration (통합) | 컴포넌트 간 상호작용 |

## 언어별 실행

| 언어 | 실행 | Watch 모드 |
|------|------|-----------|
| Go | `go test ./...` | `gotestsum --watch` |
| Python | `uv run pytest` | `uv run pytest --watch` (pytest-watch) |
| TypeScript | `pnpm test` | `pnpm test --watch` |

## 체크리스트 (매 사이클)

- [ ] RED: 테스트가 실패하는가? (실패 안 하면 테스트가 의미 없음)
- [ ] GREEN: 최소한의 코드로 통과했는가?
- [ ] REFACTOR: 중복이 제거되었는가?
- [ ] 모든 기존 테스트가 여전히 통과하는가?

## 안티패턴

- 테스트 없이 구현 먼저 ("나중에 테스트 추가" = 안 함)
- GREEN에서 과도한 설계 (REFACTOR에서 할 것)
- 한 사이클에서 너무 많은 것을 변경
- 이미 통과하는 테스트 작성 (RED가 안 됨 = 쓸모없는 테스트)
- REFACTOR 건너뛰기 (기술 부채 누적)

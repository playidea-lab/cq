# Rule: strict-types
> `any`/`unknown` 타입 남용 금지. 모든 함수에 명시적 타입 요구.

## 규칙

- `any` 타입 사용 금지 (TypeScript)
- `interface{}` 직접 사용 금지, 제네릭 또는 구체 타입으로 대체 (Go)
- `object`, `{}` 타입 금지, 구체 인터페이스 정의 (TypeScript)
- 함수 반환 타입 명시 (추론 가능해도 public API는 명시)
- 타입 단언(`as T`) 최소화, type guard 우선

## TypeScript

```typescript
// 금지
function process(data: any): any {
    return data.value;
}

// 허용
interface UserData {
    id: string;
    value: number;
}
function process(data: UserData): number {
    return data.value;
}

// unknown 사용 시 — type guard 필수
function parseResponse(raw: unknown): UserData {
    if (!isUserData(raw)) throw new Error('invalid response');
    return raw;
}

function isUserData(v: unknown): v is UserData {
    return typeof v === 'object' && v !== null
        && 'id' in v && 'value' in v;
}
```

## Go

```go
// 금지
func process(data interface{}) interface{} {
    return data
}

// 허용 — 제네릭
func process[T any](data T) T {
    return data
}

// 허용 — 구체 인터페이스
type Processor interface {
    Process() Result
}
func run(p Processor) Result {
    return p.Process()
}
```

## Python

```python
# 금지
def process(data) -> None:
    ...

# 허용
from dataclasses import dataclass

@dataclass
class UserData:
    id: str
    value: int

def process(data: UserData) -> int:
    return data.value
```

## 허용 예외

- 외부 라이브러리 타입이 `any`를 반환하는 경우 — 래퍼에서 구체 타입으로 변환
- 테스트 헬퍼 함수의 `interface{}`/`any` — 허용
- JSON 파싱 직후 — `unknown`으로 받고 즉시 검증

# CUSTOMIZE: 타입 엄격도 조정
# 예: tsconfig strict 옵션 목록
# 예: 허용 예외 패턴 추가
# 예: 공유 타입 정의 파일 경로

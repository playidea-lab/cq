# Go Style Guide

> Go 프로젝트 코딩 컨벤션.

## 도구

- 포매터: `gofmt` (CI에서 강제)
- 린트: `go vet ./...`
- 빌드 검증: `go build ./...` — 커밋 전 필수

## 에러 핸들링

- 에러는 무시하지 않는다. 처리하거나 명시적으로 무시 (`_ = fn()`).
- `errors.Wrap` 또는 `fmt.Errorf("context: %w", err)` 로 맥락 추가.
- sentinel error는 패키지 레벨 `var ErrXxx = errors.New(...)`.
- 에러 타입 검사: `errors.Is()`, `errors.As()` 사용. 문자열 비교 금지.

## 패턴

- **Context**: 모든 I/O 함수에 `context.Context` 첫 번째 인자.
- **Interface**: 소비하는 쪽에서 정의 (작은 인터페이스 선호).
- **Typed Nil**: Go interface에 nil 구체 포인터 대입 금지.
  ```go
  // Bad
  var i Interface = (*ConcreteType)(nil)  // i != nil!
  // Good
  var i Interface
  if ptr != nil { i = ptr }
  ```
- **Goroutine**: 시작한 곳에서 종료 책임. `sync.WaitGroup` 또는 `errgroup`.
- **Defer**: 리소스 정리에 defer 필수 (파일, DB, mutex).

## 프로젝트 구조

```
cmd/<app>/     CLI 진입점
internal/      비공개 패키지
pkg/           공개 패키지 (필요 시만)
test/          통합 테스트, 벤치마크
```

## 금지

- `init()` 함수에서 I/O 또는 무거운 작업 금지.
- 글로벌 변수 최소화. 불가피하면 sync.Once로 초기화.
- `panic()` — 라이브러리에서 금지, main에서만 허용.
- `unsafe` — 명시적 승인 없이 사용 금지.

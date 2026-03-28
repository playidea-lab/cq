# Rule: no-todo-comments
> 코드 내 TODO/FIXME/HACK 주석 금지. 발견된 작업은 이슈 트래커에 등록한다.

## 규칙

- `// TODO:`, `// FIXME:`, `// HACK:`, `# TODO:`, `# FIXME:` 주석 커밋 금지
- 코드 리뷰 시 이런 주석이 있으면 Request Changes
- 예외: 아직 병합하지 않은 WIP 브랜치에서 임시 사용

## 이유

- TODO 주석은 이슈 트래커 없이 사라지는 경우가 많다
- 코드를 읽는 사람에게 맥락 없이 부담을 준다
- 이슈로 관리하면 우선순위 지정, 담당자 배정, 진행 추적이 가능하다

## 대체 방법

```go
// 금지
// TODO: 나중에 캐시 추가

// 허용 — 이슈 번호와 함께 (짧게, 시한 있음)
// See: https://github.com/org/repo/issues/42
```

또는 즉시 이슈를 생성하고 주석 없이 커밋:

```bash
# GitHub CLI로 이슈 생성
gh issue create --title "캐시 추가" --body "캐시 레이어 필요"
```

## 탐지 방법

```bash
# 커밋 전 확인
grep -rn "TODO:\|FIXME:\|HACK:" --include="*.go" --include="*.py" --include="*.ts" .
```

# CUSTOMIZE: 허용하는 주석 패턴, 이슈 트래커 URL, 예외 파일 목록
# 예: 테스트 파일에서는 TODO 허용, 외부 라이브러리 코드 제외

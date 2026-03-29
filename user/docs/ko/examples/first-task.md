# CQ로 첫 번째 태스크 수행하기

처음부터 작동하는 커밋까지 — CQ 태스크를 처음 만들고 완료하는 전체 과정입니다.

---

## 만들 것

이 예제에서는 기존 Go API에 간단한 헬스체크 엔드포인트를 추가합니다. 워크플로우는 다음을 포함합니다:

1. 프로젝트에 CQ 초기화
2. 명확한 완료 기준으로 태스크 생성
3. Worker를 실행하여 구현
4. 결과 제출

총 소요 시간: 약 5분.

---

## 사전 요구사항

- CQ 설치 완료 (`cq --version`이 버전 번호 출력)
- Claude Code에서 Go 프로젝트 열기
- Git 저장소 초기화

---

## 1단계: CQ 상태 확인

프로젝트 디렉토리에서 Claude Code를 엽니다. 먼저 CQ가 초기화되어 있는지 확인합니다:

```
/status
```

**초기화되지 않은 경우 예상 출력:**

```
CQ not initialized. Run /c4-init to set up.
```

**이미 초기화된 경우 예상 출력:**

```
## CQ Status
- Project: my-api
- State: IDLE
- Queue: 0 pending | 0 in_progress | 0 done
```

초기화되지 않았으면 `/c4-init`을 실행하고 안내를 따르세요.

---

## 2단계: 태스크 생성

작고 범위가 명확한 태스크(1–5개 파일)에는 `/quick`을 사용하세요:

```
/quick "GET /health 엔드포인트 추가 — {status: ok, version: string} 반환"
```

CQ가 태스크를 생성하고 즉시 할당합니다:

```
Task created: T-001
Title: GET /health 엔드포인트 추가 — {status: ok, version: string} 반환
Scope: auto-detected (Go backend)
Status: in_progress (claimed by current session)
```

**내부적으로 일어나는 일:**

- CQ가 `.c4/tasks.db`에 태스크 T-001 생성
- 태스크가 자동으로 클레임됨 — `/quick`에서는 별도의 `/c4-claim` 불필요
- CQ가 프로젝트를 스캔하여 Go 파일을 감지하고 `domain: go` 설정

---

## 3단계: 엔드포인트 구현

이제 기능을 구현합니다. quick 모드에서는 CQ가 직접 코드를 작성하지 않습니다 — 직접 작성하세요. 적절한 핸들러 파일을 찾거나 생성합니다:

```go
// internal/api/health.go
package api

import (
    "encoding/json"
    "net/http"
)

var version = "1.0.0"

func handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "status":  "ok",
        "version": version,
    })
}
```

라우터에 경로 등록:

```go
// internal/api/router.go
mux.HandleFunc("GET /health", handleHealth)
```

---

## 4단계: 유효성 검사

제출 전에 유효성 검사를 실행합니다:

```
/validate
```

**예상 출력:**

```
Running validations...
  go-build:  PASS  (cd . && go build ./...)
  go-vet:    PASS  (cd . && go vet ./...)

All validations passed.
```

유효성 검사가 실패하면 계속하기 전에 오류를 수정하세요. CQ는 유효성 검사에 실패한 태스크 제출을 거부합니다.

**일반적인 실패: 빌드 오류**

```
go-build: FAIL
  ./internal/api/health.go:8:2: undefined: json
  → 임포트에 "encoding/json" 추가
```

문제를 수정하고 `/validate`를 다시 실행하세요.

---

## 5단계: 제출

유효성 검사를 통과하면:

```
/submit
```

**예상 출력:**

```
Submitting T-001...
  Commit: abc1234 "feat(api): add GET /health endpoint"
  Validation: all passed
  Status: done

Task T-001 completed.
```

CQ가 자동으로:

1. 최종 유효성 검사 실행
2. 태스크 설명으로 git 커밋 생성
3. 큐에서 태스크를 `done`으로 표시

---

## 6단계: 확인

최종 상태 확인:

```
/status
```

```
## CQ Status
- Project: my-api
- State: IDLE
- Queue: 0 pending | 0 in_progress | 1 done

Done:
  T-001  GET /health 엔드포인트 추가        abc1234
```

엔드포인트를 직접 테스트:

```bash
go run . &
curl http://localhost:8080/health
# {"status":"ok","version":"1.0.0"}
```

---

## 배운 것

| 개념 | 커맨드 | 사용 시점 |
|------|--------|---------|
| 빠른 태스크 | `/quick "설명"` | 1–5개 파일, 요구사항이 명확할 때 |
| 유효성 검사 | `/validate` | 모든 제출 전 |
| 제출 | `/submit` | 유효성 검사 통과 후 |
| 상태 확인 | `/status` | 언제든지 |

---

## 다음 단계

- **버그 수정 시나리오**: [/quick으로 버그 수정](bug-fix.md)
- **더 큰 기능**: [/pi와 /plan으로 기능 계획](feature-planning.md)
- **전체 커맨드 레퍼런스**: [사용 가이드](../reference/commands.md)

# C4 Submit Task

현재 작업 중인 태스크를 제출합니다.

## 사용법

```
/c4-submit [task-id]
```

task-id가 없으면 **현재 작업 중인 태스크를 자동 감지**합니다.

## Instructions

### 1. 현재 상태 확인

```
status = mcp__c4__c4_status()
```

- 현재 worker의 in_progress 태스크 확인
- 여러 개면 목록 제시 후 선택

### 2. 태스크 자동 감지

`$ARGUMENTS`가 비어있으면:

```
사용자: /c4-submit

Claude: 현재 작업 중인 태스크:
  - T-003: 로그인 페이지 구현
    DoD: JWT 인증, 소셜 로그인 지원
    스코프: src/auth/

  이 태스크를 제출할까요?
```

여러 태스크가 진행 중이면:

```
Claude: 어떤 태스크를 제출할까요?
  1. T-003: 로그인 페이지 구현
  2. T-004: 회원가입 API

사용자: 1
```

### 3. 검증 실행

제출 전 자동으로 검증 실행:

```
Claude: 검증 실행 중...
  - lint: ✅ pass
  - unit: ✅ pass

  모든 검증 통과! 제출을 진행할까요?
```

검증 실패 시:

```
Claude: 검증 실행 중...
  - lint: ✅ pass
  - unit: ❌ fail
    Error: test_login_success failed

  검증 실패로 제출할 수 없습니다.
  수정 후 다시 시도해주세요.

  [도움이 필요하면 말씀해주세요]
```

### 4. 커밋 SHA 확인

```bash
commit_sha = git rev-parse HEAD
```

최근 커밋 확인:

```
Claude: 최근 커밋으로 제출합니다:
  - SHA: abc123d
  - 메시지: "feat: implement login page"

  맞으면 '확인', 다른 커밋이면 SHA를 알려주세요.
```

### 5. 제출 실행

```python
mcp__c4__c4_submit(
    task_id=task_id,
    commit_sha=commit_sha,
    validation_results=[
        {"name": "lint", "status": "pass"},
        {"name": "unit", "status": "pass"}
    ]
)
```

### 6. 결과 및 다음 단계

```
Claude: ✅ T-003 제출 완료!

  다음 단계:
  - 대기 중인 태스크: 2개 (T-004, T-005)
  - /c4-run으로 다음 태스크 시작
```

체크포인트 도달 시:

```
Claude: ✅ T-003 제출 완료!

  🎯 Checkpoint CP-001 조건 충족!
  Supervisor 리뷰를 기다리는 중...

  /c4-checkpoint로 상태 확인
```

## 대화 예시

### 기본 흐름

```
사용자: /c4-submit

Claude: 현재 작업 중인 T-003 (로그인 페이지)을 제출할까요?

사용자: 응

Claude: 검증 실행 중...
  - lint: ✅ pass
  - unit: ✅ pass

  커밋 abc123d로 제출합니다. 진행할까요?

사용자: 응

Claude: ✅ T-003 제출 완료!
  다음 태스크: T-004 (회원가입 API)
```

### 빠른 제출

```
사용자: /c4-submit T-003

Claude: T-003 검증 중...
  - lint: ✅ pass
  - unit: ✅ pass

  ✅ T-003 제출 완료!
```

### 검증 실패

```
사용자: /c4-submit

Claude: T-003 검증 중...
  - lint: ✅ pass
  - unit: ❌ fail

  테스트 실패:
  - test_login_invalid_password: AssertionError

  수정이 필요합니다. 도움이 필요하면 말씀해주세요.

사용자: 수정해줘

Claude: [테스트 오류 분석 후 수정 진행...]
```

## 검증 실패 시 자동 도움

검증 실패 후 사용자가 도움을 요청하면:

1. 오류 메시지 분석
2. 관련 코드 확인
3. 수정 제안 또는 직접 수정
4. 재검증 후 제출

## After Submission

제출 후 시스템이 자동으로:
- 태스크를 완료로 표시
- scope lock 해제
- 체크포인트 조건 확인
- 필요시 Supervisor 리뷰 트리거

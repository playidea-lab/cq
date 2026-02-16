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

### 3. 기본 검증 실행

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

### 3.5 DDD-CLEANCODE 검증 (Worker Packet 포함 태스크)

태스크에 Worker Packet 명세(BoundaryMap, ContractSpec 등)가 포함된 경우 **추가 검증을 수행**합니다.

#### 3.5.1 경계 검증 (Boundary Validation)

BoundaryMap이 정의된 경우, import 규칙 위반을 검사합니다:

```python
from c4.validators.boundary import validate_boundary, format_violations_report
from c4.models.ddd import BoundaryMap
from pathlib import Path

# 태스크에서 BoundaryMap 가져오기
if task.boundary_map:
    # 변경된 파일 목록 수집
    files = [Path(f) for f in task.code_placement.create + task.code_placement.modify]

    # 경계 검증 실행
    result = validate_boundary(files, task.boundary_map, project_root=Path("."))

    if not result.valid:
        print(format_violations_report(result.violations))
```

**경계 위반 시:**

```
Claude: 🔴 경계 검증 실패!

❌ Found 2 boundary violations:

📁 src/auth/service.py
   Line 5: sqlalchemy
   └─ Forbidden import: sqlalchemy

📁 src/auth/domain/user.py
   Line 3: httpx
   └─ Forbidden import: httpx

⚠️ BoundaryMap 규칙:
  - target_layer: app
  - forbidden_imports: [sqlalchemy, httpx, fastapi]

경계 위반으로 제출할 수 없습니다.
domain 레이어에서 infra 의존성을 제거해주세요.
```

#### 3.5.2 작업 분해 검증 (Work Breakdown Validation)

태스크 크기가 DDD-CLEANCODE 가이드라인을 준수하는지 검사합니다:

```python
from c4.validators.work_breakdown import analyze_task_size, format_breakdown_report

# 작업 분해 분석
result = analyze_task_size(task)

if not result.valid:
    print(format_breakdown_report(result))
```

**기준 초과 시:**

```
Claude: ⚠️ 작업 분해 검증 경고!

❌ Task should be split:

📊 Metrics:
  - APIs: 5 (max: 3) ⚠️
  - Tests: 12 (max: 9) ⚠️
  - Files: 4 (within limit: 5)
  - Domains: 1 (within limit: 1)

📋 Recommendations:
  - [must_split] Too many APIs (5 > 3)
  - [should_split] Too many tests (12 > 9)

제안: 태스크를 2-3개로 분리하세요.
예: UserService.register + UserService.login → 별도 태스크

그래도 제출하시겠습니까? (강제 제출: 가능하지만 권장하지 않음)
```

#### 3.5.3 ContractSpec 검증

ContractSpec이 정의된 경우, 최소 테스트 요구사항을 검사합니다:

```
Claude: 📋 ContractSpec 검증...

API 명세:
  - UserService.register ✅
  - UserService.login ✅

테스트 명세:
  - success: test_register_success ✅
  - failure: test_register_duplicate_email ✅
  - boundary: test_register_max_length ✅

✅ ContractSpec 요구사항 충족!
```

**테스트 누락 시:**

```
Claude: ❌ ContractSpec 테스트 누락!

필수 테스트:
  - success: test_register_success ✅
  - failure: (없음) ❌
  - boundary: (없음) ❌

최소 1개의 failure 테스트와 1개의 boundary 테스트가 필요합니다.
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
        # 기본 검증
        {"name": "lint", "status": "pass"},
        {"name": "unit", "status": "pass"},
        # DDD-CLEANCODE 검증 (Worker Packet 포함 시)
        {"name": "boundary", "status": "pass"},
        {"name": "work_breakdown", "status": "pass"},
        {"name": "contract_spec", "status": "pass"},
    ]
)
```

> **참고**: DDD-CLEANCODE 검증은 태스크에 Worker Packet 명세(BoundaryMap, ContractSpec 등)가
> 포함된 경우에만 실행됩니다. 명세가 없으면 기본 검증(lint, unit)만 수행합니다.

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

### DDD-CLEANCODE 검증 (Worker Packet 포함)

```
사용자: /c4-submit

Claude: T-003 검증 중...
  - lint: ✅ pass
  - unit: ✅ pass

  📋 DDD-CLEANCODE 검증 중...
  - boundary: ✅ pass (금지 import 없음)
  - work_breakdown: ✅ pass (API 2개, 테스트 6개)
  - contract_spec: ✅ pass (success/failure/boundary 테스트 포함)

  ✅ 모든 검증 통과! 커밋 abc123d로 제출합니다.

사용자: 응

Claude: ✅ T-003 제출 완료!
```

### 경계 위반 시

```
사용자: /c4-submit

Claude: T-003 검증 중...
  - lint: ✅ pass
  - unit: ✅ pass

  📋 DDD-CLEANCODE 검증 중...
  - boundary: ❌ fail

  🔴 경계 위반 발견!
  📁 src/auth/service.py
     Line 5: sqlalchemy
     └─ Forbidden import: sqlalchemy (app 레이어에서 금지)

  app 레이어는 sqlalchemy를 직접 사용할 수 없습니다.
  Repository 인터페이스를 통해 접근해주세요.

사용자: 수정해줘

Claude: Repository 패턴을 적용하여 의존성을 분리합니다...
  [infra 레이어에 UserRepositoryImpl 생성]
  [app 레이어에서 UserRepository 인터페이스만 참조]

  재검증 중...
  - boundary: ✅ pass

  수정 완료! 제출하시겠습니까?
```

## 검증 실패 시 자동 도움

검증 실패 후 사용자가 도움을 요청하면:

### 기본 검증 실패 (lint, unit)
1. 오류 메시지 분석
2. 관련 코드 확인
3. 수정 제안 또는 직접 수정
4. 재검증 후 제출

### DDD-CLEANCODE 검증 실패

| 검증 유형 | 실패 원인 | 자동 도움 내용 |
|----------|----------|---------------|
| **boundary** | 금지된 import 사용 | Repository 패턴 적용, 의존성 역전 제안 |
| **work_breakdown** | 태스크 크기 초과 | 태스크 분할 제안, 도메인별 분리 |
| **contract_spec** | 테스트 누락 | 필요한 테스트 케이스 생성 |

#### 경계 위반 자동 수정 예시

```
위반: app 레이어에서 sqlalchemy 직접 사용

수정:
1. infra/repositories/user_repository.py 생성 (실제 구현)
2. domain/interfaces/user_repository.py 생성 (인터페이스)
3. app/services/user_service.py에서 인터페이스만 참조
```

#### 작업 분해 제안 예시

```
현재: T-003 (API 5개, 테스트 15개)

제안 분할:
- T-003-a: UserService.register + 관련 테스트 (API 2, 테스트 6)
- T-003-b: UserService.login + 관련 테스트 (API 2, 테스트 6)
- T-003-c: UserService.logout + 관련 테스트 (API 1, 테스트 3)
```

## After Submission

제출 후 시스템이 자동으로:
- 태스크를 완료로 표시
- scope lock 해제
- 체크포인트 조건 확인
- 필요시 Supervisor 리뷰 트리거

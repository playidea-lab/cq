<!--
  Platform: Cursor
  Based on: Claude Code version

  TODO: Cursor 특화 수정 필요시 여기에 기록
-->

# C4 Checkpoint Review

체크포인트 상태를 확인하고 결정을 내립니다.

## 사용법

```
/c4-checkpoint
```

대화형으로 체크포인트 리뷰를 진행합니다.

## auto_approve 설정

체크포인트는 `auto_approve` 설정에 따라 동작이 다릅니다:

| 설정 | 동작 |
|------|------|
| `auto_approve: true` (기본값) | Worker(AI)가 자동으로 리뷰. `/c4-checkpoint` 불필요 |
| `auto_approve: false` | CHECKPOINT 상태에서 대기. **사람이 `/c4-checkpoint` 호출 필수** |

```yaml
# config.yaml 예시
checkpoints:
  - id: CP-QUICK
    auto_approve: true   # AI 자동 리뷰

  - id: CP-FINAL
    auto_approve: false  # 사람 리뷰 필수 ← 이 경우 /c4-checkpoint 필요
```

## Instructions

### 1. 현재 상태 확인

```
status = mcp__c4__c4_status()
```

### 2. 상태별 처리

#### CHECKPOINT 상태

```
Claude: 🎯 CP-001 리뷰 준비됐습니다!

  완료된 태스크:
  - T-001: 프로젝트 설정 ✅
  - T-002: 로그인 구현 ✅

  검증 결과:
  - lint: ✅ pass
  - unit: ✅ pass (15/15)

  어떻게 처리할까요?
  1. 승인 - 다음 단계로 진행
  2. 수정 요청 - 변경사항 지정 후 계속
  3. 재계획 - PLAN으로 돌아가서 재설계

사용자: 1

Claude: ✅ CP-001 승인 완료!
  → 다음 단계로 진행합니다.
```

#### EXECUTE 상태 (체크포인트 전)

```
Claude: 현재 EXECUTE 상태입니다.

  다음 체크포인트: CP-001
  진행 상황:
  - 완료: T-001, T-002 (2/4)
  - 진행 중: T-003
  - 대기: T-004

  CP-001 조건:
  - 필요 태스크: T-001 ~ T-004 (2/4 완료)
  - 필요 검증: lint, unit

  아직 체크포인트에 도달하지 않았습니다.
  /c4-run으로 작업을 계속하세요.
```

#### COMPLETE/HALTED 상태

```
Claude: 현재 {상태} 상태입니다.
  체크포인트 리뷰가 필요하지 않습니다.
```

### 3. 결정 처리

#### 승인 (APPROVE)

```python
mcp__c4__c4_checkpoint(
    checkpoint_id="CP-001",
    decision="APPROVE",
    notes="모든 기능 정상 동작 확인"
)
```

결과:
```
Claude: ✅ CP-001 승인 완료!

  다음 단계:
  - 대기 중인 태스크: T-005, T-006
  - /c4-run으로 계속 진행
```

#### 수정 요청 (REQUEST_CHANGES)

```
Claude: 어떤 수정이 필요한가요?

사용자: 로그인 에러 메시지가 너무 기술적이야. 사용자 친화적으로 바꿔줘

Claude: 수정 요청을 기록합니다:
  - "로그인 에러 메시지를 사용자 친화적으로 변경"

  다른 수정사항이 있나요? (없으면 '완료')

사용자: 완료

Claude: 수정 태스크를 생성합니다:
  - T-FIX-001: 로그인 에러 메시지 개선

  EXECUTE 상태로 돌아갑니다.
  /c4-run으로 수정 작업을 시작하세요.
```

```python
mcp__c4__c4_checkpoint(
    checkpoint_id="CP-001",
    decision="REQUEST_CHANGES",
    notes="사용자 친화적 에러 메시지 필요",
    required_changes=["로그인 에러 메시지를 사용자 친화적으로 변경"]
)
```

#### 재계획 (REPLAN)

```
사용자: 3

Claude: 재계획을 선택하셨습니다.
  현재 구현된 내용은 유지되지만, 태스크를 다시 설계합니다.

  재계획 이유를 알려주세요:

사용자: API 구조를 REST에서 GraphQL로 바꾸고 싶어

Claude: 이해했습니다. PLAN 상태로 돌아갑니다.
  /c4-plan으로 새로운 계획을 수립하세요.
```

```python
mcp__c4__c4_checkpoint(
    checkpoint_id="CP-001",
    decision="REPLAN",
    notes="API 구조 변경: REST → GraphQL"
)
```

### 4. REDESIGN 옵션 (설계 변경 필요시)

아키텍처 수준의 변경이 필요한 경우:

```
Claude: 어떻게 처리할까요?
  1. 승인
  2. 수정 요청
  3. 재계획
  4. 재설계 - DESIGN 단계로 돌아가서 아키텍처 재검토

사용자: 4

Claude: 재설계를 선택하셨습니다.
  DESIGN 상태로 돌아가서 아키텍처를 재검토합니다.

  재설계 이유를 알려주세요:

사용자: 마이크로서비스 대신 모놀리식으로 가고 싶어
```

## 대화 예시

### 간단한 승인

```
사용자: /c4-checkpoint

Claude: 🎯 CP-001 리뷰 준비됐습니다!
  - 완료: T-001, T-002
  - 검증: lint ✅ unit ✅

  어떻게 할까요?
  1. 승인  2. 수정 요청  3. 재계획

사용자: 1

Claude: ✅ CP-001 승인! 다음 태스크로 진행합니다.
```

### 수정 요청

```
사용자: /c4-checkpoint

Claude: 🎯 CP-001 리뷰 준비됐습니다!
  [상태 표시...]

  어떻게 할까요?

사용자: 2

Claude: 어떤 수정이 필요한가요?

사용자: 테스트 커버리지가 낮아. 80% 이상으로 올려줘

Claude: 수정 태스크 생성:
  - T-FIX-001: 테스트 커버리지 80% 달성

  /c4-run으로 수정 작업을 시작하세요.
```

## Decisions 요약

| 결정 | 효과 |
|------|------|
| APPROVE | 다음 단계 또는 COMPLETE로 진행 |
| REQUEST_CHANGES | 수정 태스크 생성, EXECUTE로 복귀 |
| REPLAN | PLAN 상태로 돌아가 태스크 재설계 |
| REDESIGN | DESIGN 상태로 돌아가 아키텍처 재검토 |

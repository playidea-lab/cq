# C4 Checkpoints

## 체크포인트 시스템 개요

체크포인트는 프로젝트 진행 중 **슈퍼바이저 리뷰**가 필요한 지점입니다.

### 체크포인트 흐름

```text
EXECUTE → [조건 충족] → CHECKPOINT → [결정] → 다음 단계
                                        │
                         ┌──────────────┼──────────────┐
                         ▼              ▼              ▼
                      APPROVE    REQUEST_CHANGES    REPLAN
                         │              │              │
                         ▼              ▼              ▼
                      EXECUTE       EXECUTE         PLAN
                      (다음 태스크)  (수정 태스크)   (재계획)
```

---

## 기본 체크포인트

`c4 init` 시 다음 기본 체크포인트가 자동 생성됩니다:

| ID | 설명 | 필수 Validation |
|----|------|----------------|
| `CP-REVIEW` | 코드 리뷰 완료 후 Supervisor 검토 | lint |
| `CP-FINAL` | 모든 작업 완료 후 최종 검토 | lint, unit |

기본 체크포인트는 모든 태스크 완료 시 트리거됩니다 (`required_tasks: []`).

---

## 체크포인트 설정

`.c4/config.yaml`에서 정의:

```yaml
checkpoints:
  - id: CP1
    name: "Phase 1 Review"
    description: "Phase 1 작업 리뷰"
    required_tasks:
      - T-001
      - T-002
    required_validations:
      - lint
      - unit
    auto_approve: true   # 기본값: AI 자동 리뷰

  - id: CP2
    name: "Final Review"
    description: "최종 검토 - 사람 리뷰 필수"
    required_tasks:
      - T-003
      - T-004
    required_validations:
      - lint
      - unit
      - e2e
    auto_approve: false  # 사람 리뷰 필수 (/c4-checkpoint 호출 필요)
```

### auto_approve 옵션

| 값 | 동작 | 사용 시점 |
|----|------|----------|
| `true` (기본값) | CP 태스크 생성 → Worker(AI)가 자동 리뷰 | 일반적인 체크포인트 |
| `false` | CHECKPOINT 상태 진입 → 사람이 `/c4-checkpoint` 호출 | 중요한 결정, 최종 릴리스 |

---

## Gate Conditions

체크포인트가 트리거되려면:

1. **required_tasks**: 모든 태스크가 `done` 상태
2. **required_validations**: 마지막 validation 결과가 모두 `pass`

---

## 결정 (Decision)

| Decision | 효과 | 사용 시점 |
|----------|------|----------|
| `APPROVE` | 다음 단계로 진행 | 리뷰 통과 |
| `REQUEST_CHANGES` | RC-* 태스크 생성, 계속 진행 | 수정 필요 |
| `REPLAN` | PLAN 상태로 복귀 | 설계 변경 필요 |

---

## 슬래시 명령어

```bash
# 체크포인트 상태 확인
/c4-checkpoint

# 결정 기록 (슈퍼바이저)
/c4-checkpoint APPROVE "Looks good"
/c4-checkpoint REQUEST_CHANGES "Fix lint errors"
/c4-checkpoint REPLAN "Need architecture review"
```

---

## 예시: 완료된 리팩토링 체크포인트

### CP-R1: Breaking Change 검증 ✅

**Gate Conditions:**

- T-R01: 패키지 리네임 c4d → c4 ✅
- T-R02: daemon/ 서브패키지 추출 ✅

**Validations:** lint ✅, unit ✅

**Decision:** APPROVE

---

### CP-R2: 최종 검증 ✅

**Gate Conditions:**

- T-R03: models/ 분리 ✅
- T-R04: 테스트 재편성 ✅

**Validations:** lint ✅, unit ✅

**Decision:** APPROVE

---

## 기술 상세

### passed_checkpoints

한 번 통과한 체크포인트는 다시 트리거되지 않습니다:

```python
# c4/models/state.py
class C4State(BaseModel):
    passed_checkpoints: list[str] = Field(default_factory=list)
```

`APPROVE` 또는 `REQUEST_CHANGES` 결정 시 해당 체크포인트 ID가 `passed_checkpoints`에 추가됩니다.

# Phase 4.5: Plan Refine (Worker 기반 Pre-Mortem)

> Entry: Phase 4 draft 완료 직후. DB 커밋 전.
> Exit: 수렴 선언 → Phase 4.9 (DB 커밋) → Phase 5
> Purpose: 신선한 컨텍스트를 가진 Worker가 계획을 비판 — confirmation bias 제거.
>
> ⚠️ **인라인 자가 비판 금지**: 계획을 만든 세션이 직접 비판하면 confirmation bias 발생.
>    매 라운드마다 반드시 새 Worker(Task agent)를 스폰하여 격리된 시각으로 검토.

## 4.5.0 Config 분기

```python
if not LOOP_ACTIVE:
    print("⏭️  Plan Refine 비활성화 (config: planning.critique_loop)")
    → Phase 4.9 (DB Commit) 직행
INTERACTIVE = (CRITIQUE_MODE == "interactive")
```

## 4.5.1 Loop 초기화

```
round = 0 (max: CRITIQUE_MAX_ROUNDS, 기본 3)
converged = false
current_draft = Phase 4에서 작성한 태스크 초안 (전체 텍스트)
```

## 4.5.2 Worker 스폰: 격리된 Plan Refiner

매 라운드마다 새 Worker를 스폰하여 fresh context 보장.

```python
if INTERACTIVE:
    AskUserQuestion(question=f"Round {round+1}/{CRITIQUE_MAX_ROUNDS} critique를 실행할까요?")

critique_result = Task(
    subagent_type="general-purpose",
    description=f"Plan critique round {round+1}",
    prompt=f"""
당신은 시니어 소프트웨어 아키텍트입니다.
아래 C4 구현 계획 초안을 Pre-Mortem 방식으로 비판하세요.

**프레임**: "이 계획대로 실행했고 3개월 후 실패했다. 가장 큰 실패 원인은?"

## 계획 초안
{current_draft}

## 비판 렌즈 (각 태스크에 대해 검토)

| 렌즈 | 질문 | 심각도 |
|------|------|--------|
| DoD 측정 가능성 | "완료됐음을 어떻게 증명하는가?" | CRITICAL |
| R- 쌍 누락 | T-에 대응하는 R-가 의존성 트리에 있는가? | CRITICAL |
| test 파일 존재 확인 | QualityGate에 `test -f` 포함? | CRITICAL |
| 파일 충돌 | 두 태스크가 같은 파일을 동시에 수정? | CRITICAL |
| 가정 목록 | 전제하는 것은? (파일 경로, API 형식, 외부 설정) | HIGH |
| 의존성 누락 | 숨겨진 실행 순서 제약? | HIGH |
| 범위 과잉 | 수정 파일 5개/API 3개/도메인 2개 초과? | HIGH |
| 더 단순한 방법 | 50% 코드로 80% 결과? | MEDIUM |
| 외부 의존성 | 환경 변수, 외부 서비스 전제? | MEDIUM |

## 출력 형식

### CRITICAL (즉시 수정 필요)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### HIGH (수정 권장)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### MEDIUM (선택적)
- [T-XXX-0] 문제: ... → 권장 수정: ...

### 수렴 판정
CRITICAL: N건 / HIGH: N건 / MEDIUM: N건
판정: CONVERGED | NOT_CONVERGED
"""
)
```

## 4.5.3 수정 (Revise) — 메인 세션에서 적용

- **CRITICAL**: 즉시 draft 수정 (DoD 재작성, 태스크 분리/병합)
- **HIGH**: 수정 적용 또는 Rationale에 리스크 명시
- **MEDIUM**: 사용자에게 선택 제시 (INTERACTIVE 모드) 또는 자동 적용

## 4.5.4 수렴 판정

```python
converged = (critical_count == 0 and high_count == 0)

if converged or round + 1 >= CRITIQUE_MAX_ROUNDS:
    → 수렴 선언 출력 → Phase 4.9
else:
    round += 1 → Phase 4.5.2 (새 Worker 스폰)
```

**수렴 선언**:
```
## 계획 수렴 ✅ (Round N/MAX)
CRITICAL: 0 / HIGH: 0 / MEDIUM: N (허용)
→ DB 커밋 진행
```

**MAX round 미수렴 시**:
→ AskUserQuestion: "(1) 현재 상태로 진행  (2) 수동 수정 후 재시작"

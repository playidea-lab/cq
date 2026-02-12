---
name: c4-direct
description: "Codex Direct 모드 최적화 (claim/report)"
triggers:
  - c4 direct
  - direct mode
  - 파일 의존성이 큰 작업
---

# Goal
파일 결합이 큰 태스크를 Direct 모드로 완결합니다.

## Workflow
1. `c4_status()` 호출 후 `state`와 `ready_task_ids` 확인.
2. 대상 `task_id` 선택 (미지정이면 `ready_task_ids[0]` 제안).
3. `c4_claim(task_id=...)` 호출.
4. claim 응답의 `dod`, `suggested_validations`, `next_steps`, `recommended_commit_message`를 작업 계획으로 고정.
5. 구현.
6. `c4_run_validation(names=suggested_validations)` 실행.
7. 실패 시 수정 후 6단계 반복.
8. 커밋.
9. summary를 3줄로 작성:
   - What changed
   - Why
   - Validation result
10. `c4_report(task_id, summary, files_changed)` 호출.
11. `c4_status()` 재조회로 반영 확인.

## Failure Branch
- claim 실패(`not pending`, `not found`): `c4_status()` 재조회 후 태스크 재선정.
- report 실패(`expected in_progress` 등): owner/state 충돌로 판단하고 즉시 중단 후 상태 보고.

## Safety Rules
- Direct 태스크는 `c4_submit` 금지.
- `summary`/`files_changed` 누락 금지.

## Output Checklist
- [ ] claim 결과 + DoD
- [ ] 검증 결과
- [ ] report 결과
- [ ] 다음 ready task 또는 종료 판단

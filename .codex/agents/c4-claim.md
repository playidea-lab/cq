---
name: c4-claim
description: "Direct 태스크를 claim하고 실행계획 생성"
triggers:
  - c4 claim
  - claim task
  - direct 시작
---

# Goal
`c4_claim` 응답을 바로 실행 가능한 계획으로 변환합니다.

## Workflow
1. `c4_status()`에서 `ready_task_ids` 확인.
2. `task_id` 미지정이면 후보 1~3개 제시.
3. `c4_claim(task_id=...)` 호출.
4. 응답에서 아래를 필수 추출:
   - `dod`
   - `suggested_validations`
   - `recommended_commit_message`
   - `next_steps`
5. "구현 -> 검증 -> 커밋 -> report" 순서로 실행 계획 출력.

## Output Checklist
- [ ] claim 성공 여부
- [ ] DoD
- [ ] 검증/커밋/보고 순서

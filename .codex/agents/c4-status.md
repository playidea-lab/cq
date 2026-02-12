---
name: c4-status
description: "C4 프로젝트 상태/큐를 Codex에서 정리"
triggers:
  - c4 status
  - show c4 status
  - 상태 보여줘
---

# Goal
`c4_status()` 결과를 실행 가능한 다음 액션으로 변환합니다.

## Workflow
1. `c4_status()` 호출.
2. 핵심 필드 확인:
   - `state`
   - `total_tasks`, `pending_tasks`, `in_progress_tasks`, `done_tasks`, `blocked_tasks`
   - `ready_tasks`, `blocked_by_dependencies`, `ready_task_ids`
3. 라우팅:
   - `state == CHECKPOINT` -> `c4-checkpoint`
   - `ready_tasks == 0` and `pending_tasks > 0` -> 의존성 대기 보고
   - 결합 높은 요청(리팩토링/마이그레이션/다중파일) -> `c4-direct`
   - `ready_tasks >= 2` and 독립 범위 -> `c4-run`
4. 숫자는 표로, 마지막에 Action 1~3개 제시.

## Output Checklist
- [ ] 현재 상태/진행률
- [ ] Ready vs Dependency blocked
- [ ] 추천 실행 모드(Direct/Worker/Checkpoint)

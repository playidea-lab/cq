---
name: c4-help
description: "Codex용 C4 에이전트/도구 빠른 참조"
triggers:
  - c4 help
  - 명령어 목록
---

# Goal
Codex 사용자에게 C4 핵심 라우팅과 다음 액션을 빠르게 안내합니다.

## Workflow
1. `c4_status()`로 현재 `state`, `ready_task_ids`, `pending_tasks`를 확인.
2. 상태별 추천:
   - `INIT` -> `c4-plan` 또는 `c4-init`
   - `PLAN`/`HALTED` -> `c4-run` 또는 `c4-direct`
   - `CHECKPOINT` -> `c4-checkpoint`
   - `COMPLETE` -> `c4-finish` 또는 `c4-release`
3. 필요 시 관련 에이전트 1~3개를 우선순위로 제시.

## Output Checklist
- [ ] 현재 상태 요약
- [ ] 추천 에이전트/툴
- [ ] 즉시 실행 가능한 다음 액션

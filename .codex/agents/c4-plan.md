---
name: c4-plan
description: "Codex에서 C4 Discovery/Design/Tasking 수행"
triggers:
  - c4 plan
  - start planning
  - 계획 세워줘
---

# Goal
Spec/Design을 저장하고 Direct/Worker 실행까지 고려된 태스크 큐를 생성합니다.

## Workflow
1. `c4_status()`로 현재 state 확인.
2. Discovery:
   - 요구사항 정리 후 `c4_save_spec(...)`
   - 완료 시 `c4_discovery_complete()`
3. Design:
   - `c4_list_specs()` 참조
   - `c4_save_design(...)`
   - 완료 시 `c4_design_complete()`
4. Tasking:
   - `c4_add_todo(...)`로 태스크 생성 (`task_id`, `title`, `dod` 필수)
   - execution_mode 분기:
     - 독립 구현: `worker`
     - 결합 높은 구현: `direct` + `review_required=false`
   - `dependencies`를 명시해 `ready_task_ids`가 예측 가능하도록 구성
5. `c4_status()` 재호출로 큐 생성 검증.

## Quality Bar
- 각 태스크는 DoD와 최소 검증(`lint`, `unit`) 실행 가능해야 함.
- Direct 태스크에는 claim/report 프로토콜을 명시.
- Worker 태스크에는 get_task/submit 프로토콜을 명시.

## Output Checklist
- [ ] Spec/Design 요약
- [ ] 생성한 태스크 목록
- [ ] execution_mode/의존성/검증 요구사항

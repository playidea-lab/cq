---
name: c4-add-task
description: "C4 태스크 추가 (worker/direct 모드 지정)"
triggers:
  - c4 add task
  - add todo
  - 태스크 추가
---

# Goal
실행 가능한 태스크를 누락 없이 등록하고 실행 모드를 명시합니다.

## Workflow
1. 입력값 정리:
   - `task_id`, `title`, `dod` (필수)
   - `scope`, `dependencies` (권장)
2. 실행 모드 결정:
   - 독립 태스크: `execution_mode="worker"`
   - 결합 태스크(다중 파일/리팩토링): `execution_mode="direct"`, `review_required=false`
3. `c4_add_todo(...)` 호출.
4. `c4_status()`로 `pending_tasks`, `ready_task_ids` 반영 확인.

## Decision Rule
- 같은 커밋에서 파일/모듈 3개 이상 강결합 변경이 예상되면 Direct.
- 병렬 가능하고 scope 충돌이 낮으면 Worker.

## Output Checklist
- [ ] 태스크 정의
- [ ] execution_mode 선택 근거
- [ ] 큐 반영 결과

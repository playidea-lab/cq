---
name: c4-quick
description: "작은 작업을 빠르게 태스크 생성 + 할당까지 진행"
triggers:
  - c4 quick
  - quick start
  - 바로 시작
---

# Goal
계획 전체를 생략하고, 단일 작업을 바로 실행 가능한 상태로 만듭니다.

## Workflow
1. `c4_status()`로 상태 확인.
2. `PLAN`/`HALTED`면 `c4_start()` 호출.
3. `task_id` 생성:
   - 기본: `T-QUICK-<unix_timestamp>-0`
   - 중복 시 timestamp를 다시 생성해 재시도
4. `c4_add_todo(...)`로 태스크 생성:
   - `task_id` 필수 지정
   - `title`은 사용자 입력 사용
   - `dod`는 최소 검증(`lint`, `unit`) 포함
   - 기본 `execution_mode="worker"`
5. `worker_id` 생성 후 `c4_get_task(worker_id=...)`.
6. 구현 -> `c4_run_validation(...)` -> 커밋 -> `c4_submit(...)`.

## Safety Rules
- `INIT`/`CHECKPOINT`/`COMPLETE` 상태에서는 실행하지 않고 대체 액션 안내.
- `c4_add_todo`를 `task_id` 없이 호출하지 않음.
- 커밋 SHA 없이 submit 금지.

## Output Checklist
- [ ] 생성된 task_id
- [ ] 할당 결과(worker_id)
- [ ] 제출 준비 상태

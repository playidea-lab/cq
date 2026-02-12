---
name: c4-submit
description: "Worker 태스크 제출 (ownership/state 검증 포함)"
triggers:
  - c4 submit
  - submit task
  - worker 제출
---

# Goal
Worker 태스크를 `c4_submit` 스키마에 맞춰 안전하게 제출합니다.

## Workflow
1. 대상 `task_id`와 `worker_id` 확인.
2. `c4_run_validation(names=["lint","unit"])` 실행.
3. 검증 결과를 `c4_submit`용으로 변환:
   - 입력: `{name, passed, output}`
   - 출력: `{name, status, message}`
   - 규칙: `passed=true -> status="pass"`, `passed=false -> status="fail"`
4. 커밋 SHA 확보.
5. `c4_submit(task_id, worker_id, commit_sha, validation_results)` 호출.
6. `next_action`과 `message` 기준으로 후속 조치.

## Failure Branch
- `claimed by direct mode` -> `c4_report` 전환
- `owned by worker` -> owner 충돌 보고 후 중단
- `expected in_progress` -> 상태 갱신 후 재할당

## Safety Rules
- `task_id` 상태가 `in_progress`가 아니면 제출 금지.
- Direct owner 태스크 제출 금지.
- worker_id 없이 제출 금지.

## Output Checklist
- [ ] 검증 정규화 결과
- [ ] submit 응답
- [ ] next_action

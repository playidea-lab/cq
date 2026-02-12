---
name: c4-run
description: "Codex Worker 루프 (c4_start + c4_get_task + c4_submit)"
triggers:
  - c4 run
  - worker loop
  - 병렬 태스크 처리
---

# Goal
독립 태스크를 Worker 프로토콜로 처리합니다.

## Workflow
1. `c4_status()` 호출.
2. `state`가 `PLAN`/`HALTED`면 `c4_start()` 호출.
3. 고정 `worker_id` 생성 (예: `codex-worker-a1b2`).
4. 루프:
   - `c4_get_task(worker_id=...)`
   - 빈 결과면 종료
   - 구현
   - `c4_run_validation(names=["lint","unit"])`
   - 결과를 `validation_results`로 정규화:
     - `status = pass` if `passed=true`, else `fail`
     - `message = output` (필요 시 축약)
   - 커밋 SHA 확보
   - `c4_submit(task_id, worker_id, commit_sha, validation_results)`
5. `state == CHECKPOINT`면 루프 종료 후 `c4-checkpoint` 안내.

## Failure Branch
- submit 실패 메시지에 `claimed by direct mode` 포함:
  - 해당 태스크 submit 중단, Direct 프로토콜로 전환 안내.
- submit 실패 메시지에 `owned by worker` 포함:
  - owner 충돌로 판단, 다른 태스크로 진행.
- submit 실패 메시지에 `expected in_progress` 포함:
  - 상태 갱신 후 재할당(`c4_get_task`) 진행.

## Safety Rules
- Direct 태스크에는 `c4_submit` 금지.
- worker_id 누락 submit 금지.

## Output Checklist
- [ ] 처리한 task_id 목록
- [ ] 검증/submit 결과
- [ ] 남은 ready/pending

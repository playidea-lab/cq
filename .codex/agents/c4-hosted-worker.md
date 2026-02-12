---
name: c4-hosted-worker
description: "Hosted Worker 루프 운영 (외부 프로세스 + c4_get_task/c4_submit)"
triggers:
  - hosted worker
  - worker service
  - c4 exec codex
---

# Goal
외부 프로세스(서비스/잡 워커)에서 C4 Worker 프로토콜을 안정적으로 실행합니다.

## Workflow
1. `c4_status()`로 상태 확인.
2. `state`가 `PLAN`/`HALTED`면 `c4_start()` 호출.
3. 워커 식별자 고정:
   - 예: `worker_id="codex-hosted-${hostname}-${slot}"`
4. 루프:
   - `c4_get_task(worker_id=...)`
   - 빈 결과면 sleep 후 재시도
   - 태스크 컨텍스트 기반으로 LLM CLI 1회 실행
   - `c4_run_validation(names=["lint","unit"])`
   - 커밋 SHA 확보
   - `c4_submit(task_id, worker_id, commit_sha, validation_results)`
5. `state == CHECKPOINT`면 작업 종료 후 checkpoint 담당자에게 전달.

## Failure Branch
- submit 에러에 `claimed by direct mode` 포함:
  - 즉시 submit 중단, 해당 task는 Direct 소유로 보고 skip.
- submit 에러에 `owned by worker` 포함:
  - 중복 실행/재시도 충돌. 현재 task 처리 중단 후 다음 task 요청.
- submit 에러에 `expected in_progress` 포함:
  - 상태 경합으로 판단. `c4_status()` 재조회 후 재할당.
- LLM CLI timeout/exit non-zero:
  - 코드 미제출, 실패 로그만 남기고 재시도 횟수 정책 적용.

## Safety Rules
- Hosted Worker는 `c4_submit`만 사용하고 `c4_report` 금지.
- `worker_id`는 프로세스 생명주기 동안 불변.
- 같은 저장소에 다중 워커를 띄울 때 `worker_id` 충돌 금지.

## Output Checklist
- [ ] task_id / worker_id / commit_sha
- [ ] validation 결과 요약
- [ ] skip/재시도 건수

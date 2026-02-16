---
name: c4-run
description: "Codex Worker-first 실행 루프 (c4_start + c4_get_task + c4_submit)"
triggers:
  - c4 run
  - worker loop
  - 병렬 태스크 처리
---

# Goal
C4 구현 태스크를 Worker 프로토콜로 처리합니다.

## Workflow
1. `c4_status()` 호출.
2. 상태 분기:
   - `INIT`: 계획 요청(`c4-plan`) 안내 후 종료
   - `PLAN`/`HALTED`: `c4_start()` 호출 후 진행
   - `CHECKPOINT`: 리뷰 대기 안내 후 종료
   - `COMPLETE`: 완료 안내 후 종료
3. 고유 `worker_id` 생성 (예: `codex-worker-a1b2c3d4`).
4. 루프:
   - `c4_get_task(worker_id=...)`
   - 태스크가 없으면 `c4_status()` 재조회:
     - `pending_tasks == 0 && in_progress_tasks == 0`: 종료
     - `pending_tasks > 0 && ready_tasks == 0`: 의존성 대기 보고 후 종료
     - 그 외: 짧게 재시도 후 종료
   - 태스크 구현
   - `c4_run_validation(...)` 실행 및 `validation_results(name/status/message)` 정규화
   - 실제 코드 변경 커밋 후 `commit_sha` 확보
   - `c4_submit(task_id, worker_id, commit_sha, validation_results)`
   - 제출 후 `c4_status()` 재조회, `CHECKPOINT` 진입 시 종료

## Failure Branch
- submit 실패 메시지에 `claimed by direct mode` 포함:
  - 해당 태스크 submit 중단, Direct 프로토콜로 전환 안내.
- submit 실패 메시지에 `owned by worker` 포함:
  - owner 충돌로 판단, 다른 태스크로 진행.
- submit 실패 메시지에 `expected in_progress` 포함:
  - 상태 갱신 후 재할당(`c4_get_task`) 진행.
- 커밋 SHA가 없거나 코드 변경이 없는 경우:
  - submit 금지, 원인/다음 조치 보고 후 중단.

## Safety Rules
- Direct 태스크에는 `c4_submit` 금지.
- worker_id 누락 submit 금지.
- `commit_sha` 없는 submit 금지.
- 이 에이전트에서 `c4_claim`/`c4_report` 호출 금지.

## Output Checklist
- [ ] 처리한 task_id 목록
- [ ] 검증/submit 결과
- [ ] 남은 ready/pending/in_progress

---
name: c4-swarm
description: "다중 워커 협업 실행(팀/Hub 기반 병렬)"
triggers:
  - c4 swarm
  - swarm mode
  - 병렬 팀 실행
---

# Goal
독립 태스크를 다수 워커로 병렬 처리하고, 충돌 없이 수렴시킵니다.

## Workflow
1. `c4_status()`로 병렬 가능성 확인(`ready_tasks`, `blocked_by_dependencies`).
2. 모드 결정:
   - 일반 구현: Worker 병렬 제출(`get_task -> submit`)
   - 리뷰 전용: read-only 점검 워커 구성
3. 워커별 고유 `worker_id`를 생성/배정.
4. 각 워커는 1태스크 처리 후 종료(컨텍스트 격리).
5. 라운드 종료 후 `c4_status()` 재조회:
   - ready 태스크가 남으면 다음 배치 실행
   - `CHECKPOINT` 진입 시 중단 후 `c4-checkpoint`로 전환

## Safety Rules
- 동일 task를 여러 워커가 동시에 submit하지 않도록 owner 충돌 감시.
- Direct 태스크는 swarm 대상에서 제외(`c4-direct`로 라우팅).

## Output Checklist
- [ ] 배치별 worker_id/task_id
- [ ] submit 성공/실패 통계
- [ ] 다음 배치 또는 checkpoint 판단

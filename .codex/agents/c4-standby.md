---
name: c4-standby
description: "C5 Hub 잡을 기다리는 상주 워커 모드"
triggers:
  - c4 standby
  - standby
  - 워커 대기 모드
---

# Goal
세션을 상주 워커로 전환해 Hub 잡을 받아 처리합니다.

## Workflow
1. 사전 체크:
   - `c4_config_get(section="hub")`에서 `enabled=true` 확인
   - 첫 `c4_worker_standby(...)` 호출에서 `unknown tool`/`not found`가 반환되면 즉시 중단
   - 중단 시 대체 경로: `c4-run` 또는 `c4-hosted-worker`
2. `worker_id` 확정(없으면 `hostname-pid` 패턴 생성).
3. 루프 실행:
   - `c4_worker_standby(worker_id, capabilities=...)`
   - 반환값 분기:
     - `shutdown=true` -> 종료
     - `job_id/lease_id/command` -> 실행 단계
4. 작업 수행 후:
   - `c4_worker_complete(job_id, lease_id, worker_id, status, summary, commit_sha)`
5. shutdown 시 `c4_worker_shutdown(worker_id, reason)`로 정리.

## Safety Rules
- Hub가 비활성(`hub.enabled=false`)이면 standby 시작 금지.
- `c4_worker_standby` 도구가 미등록이면 재시도하지 않고 fallback 경로로 전환.
- lease_id 누락 상태로 complete 호출 금지.
- 장시간 잡은 중간 실패를 즉시 FAILED로 보고하고 재대기.

## Output Checklist
- [ ] worker_id
- [ ] 처리한 job 수/성공률
- [ ] 종료 사유(있다면)

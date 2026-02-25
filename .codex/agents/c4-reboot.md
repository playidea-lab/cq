---
name: c4-reboot
description: "현재 Codex 세션을 안전하게 재시작"
triggers:
  - c4 reboot
  - reboot
  - 세션 재시작
---

# Goal
세션 상태를 잃지 않도록 스냅샷 후 재시작합니다.

## Workflow
1. 재시작 전 상태 수집:
   - `c4_status()`
   - 진행 중 task/worker 정보 요약
2. 세션 정보 확인:
   - `CQ_SESSION_NAME`, `CQ_SESSION_UUID` 존재 여부 확인
3. named session이면:
   - `~/.c4/.reboot` 플래그 생성
   - 부모 프로세스 종료로 자동 재개 트리거
4. unnamed session이면:
   - 수동 재시작 절차 안내 (`cq codex` 재실행)
   - 필요한 handoff를 출력

## Safety Rules
- in-progress 태스크가 있으면 재시작 전에 소유 충돌 위험을 고지.
- 자동 재개가 불확실하면 수동 절차로 강제 전환.

## Output Checklist
- [ ] 재시작 전 상태 요약
- [ ] 자동/수동 재시작 경로
- [ ] 재개 후 첫 액션

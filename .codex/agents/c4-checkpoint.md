---
name: c4-checkpoint
description: "체크포인트 의사결정 기록"
triggers:
  - c4 checkpoint
  - gate review
  - checkpoint review
---

# Goal
체크포인트에서 APPROVE/REQUEST_CHANGES/REPLAN 결정을 기록합니다.

## Workflow
1. `c4_status()`로 CHECKPOINT 여부 확인.
2. 근거 요약: 완료 태스크, 실패 검증, 잔여 리스크.
3. `c4_checkpoint(checkpoint_id, decision, notes, required_changes)` 호출.
4. `next_action`에 따라 EXECUTE 복귀 또는 재계획 안내.

## Output Checklist
- [ ] decision과 근거
- [ ] required_changes
- [ ] next_action

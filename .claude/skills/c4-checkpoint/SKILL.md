---
name: c4-checkpoint
description: |
  Review checkpoints with strategic 4-lens analysis (holistic/user-flow/cascade/ship-ready).
  Handles CHECKPOINT state by guiding approve/request-changes/replan/redesign decisions.
  Auto-approve configurable per checkpoint. Use when checkpoint state is reached or to
  review work before proceeding. Triggers: "체크포인트", "검토 승인", "중간 리뷰",
  "/c4-checkpoint", "review checkpoint", "approve checkpoint",
  "request changes at checkpoint".
allowed-tools: Read, Glob, Grep, Agent, mcp__cq__*
---

# C4 Checkpoint Review

체크포인트 상태를 확인하고 결정을 내립니다.

> Checkpoint는 c4-run에 내장. `run.checkpoint_mode: interactive` (기본)이면 이 스킬 수동 호출, `auto`면 Worker가 자동 리뷰.

## 사용법

```
/c4-checkpoint
```

## auto_approve 설정

| 설정 | 동작 |
|------|------|
| `auto_approve: true` (기본값) | Worker(AI)가 자동 리뷰 |
| `auto_approve: false` | 사람이 `/c4-checkpoint` 호출 필수 |

## Instructions

### 0. R- 완료 선검증 (MANDATORY)

CP 진입 전 리뷰 레이어 존재+완료 검증. See `references/review-gate.md` for details.

| CP deps 구조 | 판정 |
|-------------|------|
| R- 모두 done | ✅ Step 1 진행 |
| R- done + T- (review_required=False, 사유 있음) | ⚠️ 경고 후 진행 |
| T- 직결, R- 없음 | ⛔ BLOCK |
| R- pending/in_progress | ⏳ 대기 |

### 1. 현재 상태 확인

```python
status = mcp__c4__c4_status()
```

- **CHECKPOINT**: 리뷰 진행 (Step 2로)
- **EXECUTE**: 아직 CP 미도달 — 안내 후 종료
- **COMPLETE/HALTED**: 리뷰 불필요

### 2. 전략 리뷰 렌즈 (4-lens)

| Lens | 점검 항목 |
|------|----------|
| **holistic** | 아키텍처 일관성, 패턴 조화 |
| **user-flow** | E2E 동작, 에러 UX, 성능 |
| **cascade** | 이전 REQUEST_CHANGES 해결, 새 문제 없음 |
| **ship-ready** | 테스트 통과, TODO/FIXME 없음, 롤백 가능 |

### 3. 결정 처리

| 결정 | MCP 호출 | 효과 |
|------|---------|------|
| **APPROVE** | `c4_checkpoint(decision="APPROVE")` | 다음 단계 진행 |
| **REQUEST_CHANGES** | `c4_checkpoint(decision="REQUEST_CHANGES", required_changes=[...])` | 수정 태스크 생성, EXECUTE 복귀 |
| **REPLAN** | `c4_checkpoint(decision="REPLAN")` | PLAN 상태로 복귀 |
| **REDESIGN** | `c4_checkpoint(decision="REDESIGN")` | DESIGN 상태로 복귀 |

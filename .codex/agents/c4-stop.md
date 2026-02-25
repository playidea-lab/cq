---
name: c4-stop
description: "실행 상태를 HALTED로 전환"
triggers:
  - c4 stop
  - stop execution
  - 실행 중지
---

# Goal
현재 실행을 안전하게 멈추고 재개 가능한 상태로 유지합니다.

## Workflow
1. `c4_status()`로 현재 `state` 확인.
2. `EXECUTE`/`CHECKPOINT`면 `cq stop` 실행.
3. `c4_status()` 재조회로 `HALTED` 전환 확인.
4. 중단된 in-progress 태스크를 사용자에게 보고.

## Safety Rules
- `PLAN`/`HALTED`/`COMPLETE`에서 중복 stop 실행하지 않음.
- stop 이후 재개 경로(`c4-run` 또는 `c4-start`)를 함께 안내.

## Output Checklist
- [ ] stop 전/후 state
- [ ] 영향받은 태스크
- [ ] 재개 방법

---
name: c4-clear
description: "C4 상태 초기화 (확인 후 실행)"
triggers:
  - c4 clear
  - reset c4
  - 상태 초기화
---

# Goal
`c4_clear(confirm=true)`를 안전하게 실행합니다.

## Workflow
1. 사용자 재확인 (데이터 삭제 안내).
2. `c4_status()`로 초기화 전 상태 요약.
3. `c4_clear(confirm=true, keep_config=true|false)` 호출.
4. `c4_status()`로 초기화 후 상태 확인.

## Output Checklist
- [ ] 사전 확인
- [ ] clear 결과
- [ ] 후속 액션

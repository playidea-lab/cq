---
name: c4-polish
description: "빌드/테스트/리뷰/수정 반복으로 품질 수렴"
triggers:
  - c4 polish
  - polish
  - 반복 정제
---

# Goal
CRITICAL/HIGH 이슈를 0으로 만들고, 수정사항이 없어질 때까지 반복합니다.

## Workflow
1. `c4_phase_lock_acquire(phase="polish")`.
2. 대상 범위 확정:
   - 최근 변경 파일(`git diff --name-only`) 또는 사용자 scope
3. 빌드/테스트:
   - 언어별 기본 검증 수행 (`go build`, `go test`, `pytest`, `cargo test`)
4. 리뷰 라운드:
   - 이슈를 `CRITICAL/HIGH/MEDIUM/LOW`로 분류
   - CRITICAL/HIGH 우선 수정 후 재검증
5. 종료 조건:
   - `CRITICAL == 0 && HIGH == 0`
   - 추가 수정사항 없음 (`modifications=0`)
6. gate 기록:
   - `refine=done`, `polish=done`를 `.c4/c4.db`에 기록
7. `c4_phase_lock_release(phase="polish")`.

## Safety Rules
- lock 획득 실패 시 override 여부 확인.
- 검증 실패 상태에서 다음 라운드로 넘어가지 않음.
- 목표 라운드 초과 시 잔여 리스크를 명시하고 중단.

## Output Checklist
- [ ] 라운드별 이슈 요약
- [ ] 최종 gate 기록
- [ ] 남은 리스크(있다면)

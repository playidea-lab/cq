---
name: c4-report
description: "Direct 태스크 완료 보고"
triggers:
  - c4 report
  - report direct task
  - direct 완료 보고
---

# Goal
Direct 작업을 `c4_report`로 종료 처리합니다.

## Workflow
1. 보고 대상 `task_id`와 변경 파일 목록 정리.
2. 검증 결과를 한 줄로 요약.
3. summary를 3줄로 작성:
   - What changed
   - Why
   - Validation
4. `c4_report(task_id, summary, files_changed)` 호출.
5. `c4_status()` 재호출로 pending/done 변화 확인.

## Failure Branch
- `task ... expected in_progress`: 이미 종료/충돌 상태. 재시도하지 말고 상태 보고.
- direct owner 불일치 에러: Worker 프로토콜과 충돌이므로 중단 후 분기 재설계.

## Safety Rules
- Worker 태스크에는 `c4_report` 사용 금지.
- 빈 summary 금지.

## Output Checklist
- [ ] 보고 내용
- [ ] 파일 목록
- [ ] 완료 후 상태

---
name: c4-validate
description: "C4 검증 실행 및 실패 원인 요약"
triggers:
  - c4 validate
  - run validation
  - lint test
---

# Goal
`c4_run_validation` 결과를 수정 가능한 액션으로 변환합니다.

## Workflow
1. 기본 실행: `c4_run_validation(names=["lint","unit"])`.
2. alias 허용: `test/tests/unit` 입력 시 서버 alias 매핑 사용.
3. 결과를 `name/passed/elapsed_seconds` 중심 표로 요약.
4. 실패 항목은 원인 1~2개 + 재실행 순서 제시.
5. submit 연계 시 `c4_submit` 스키마로 정규화:
   - `{name, passed, output}` -> `{name, status, message}`

## Edge Cases
- `error = no matching validations found` 응답 시:
  - `available_names`를 출력하고 요청 names를 교정.

## Output Checklist
- [ ] 실행한 names
- [ ] pass/fail 표
- [ ] 실패 대응안 또는 제출 준비 상태

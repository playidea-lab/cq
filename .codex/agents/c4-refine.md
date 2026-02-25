---
name: c4-refine
description: "[DEPRECATED] c4-polish로 통합된 정제 루프"
triggers:
  - c4 refine
  - refine
  - 리파인
---

# Goal
기존 `c4-refine` 요청을 `c4-polish`로 안전하게 라우팅합니다.

## Workflow
1. deprecated임을 명시.
2. `c4-polish` 실행 안내:
   - CRITICAL/HIGH 게이트
   - 수정사항 0 수렴
3. 사용자 플래그(`--max-rounds`, `--threshold`, `--scope`)를 `c4-polish` 인자로 전달.

## Output Checklist
- [ ] deprecated 안내
- [ ] 전달된 인자
- [ ] `c4-polish`로 전환 여부

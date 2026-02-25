---
name: c4-review
description: "코드/문서 대상 3-pass 리뷰 수행"
triggers:
  - c4 review
  - review
  - 상세 리뷰
---

# Goal
대상 변경사항을 3-pass로 검토하고 수정 우선순위를 제공합니다.

## Workflow
1. 리뷰 범위 확정:
   - 코드 변경(`git diff`) 또는 사용자 지정 파일
2. Pass 1 (Structure):
   - 변경 의도/영향 범위 파악
3. Pass 2 (Deep):
   - 정확성, 보안, 회복성, 일관성, 계약, 통합 관점 점검
4. Pass 3 (Action):
   - 이슈를 `CRITICAL/HIGH/MEDIUM/LOW`로 정리
   - 수정 제안과 검증 방법 제시
5. 필요 시 결과를 `c4-polish` 입력으로 전달.

## Safety Rules
- 리뷰 요청에서는 구현보다 결함 식별을 우선.
- 근거 없는 추정 대신 파일/라인 기준으로 보고.

## Output Checklist
- [ ] 심각도별 이슈 목록
- [ ] 위험/회귀 가능성
- [ ] 권장 수정 순서

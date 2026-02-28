---
description: |
  [DEPRECATED] c4-finish에 통합되었습니다.
  /c4-finish를 사용하세요 — polish 루프(빌드→테스트→리뷰→수정 수렴)가 내장되어 있습니다.
  Triggers: "polish", "c4-polish", "polish loop", "수정 없을 때까지", "계속 돌려",
  "refine loop", "반복 수정", "polish until clean", "빌드 테스트 리뷰 반복",
  "리파인", "정제", "반복 리뷰", "refine", "/c4-refine", "quality loop".
---

# C4 Polish — [DEPRECATED: c4-finish로 통합됨]

> ⚠️ **이 스킬은 deprecated입니다.** `/c4-finish`를 사용하세요.
>
> c4-finish가 다음을 모두 처리합니다:
> - **Step 0**: Polish Loop — 수정사항 0이 될 때까지 빌드→테스트→리뷰→수정 반복
> - **Step 1~9**: 빌드 검증 → 바이너리 설치 → 문서 → 커밋 → 릴리즈
>
> **플로우**: `/c4-plan → /c4-run → /c4-finish`

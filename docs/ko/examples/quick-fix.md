# 예시: 빠른 버그 수정

소규모, 집중적인 변경 — 계획 단계를 완전히 생략합니다.

## 사용 시기

- 단일 파일 버그 수정
- UI 조정
- 설정 변경
- 한 문장으로 설명할 수 있는 모든 것

## 예시

> **You:** "모바일에서 로그인 버튼 클릭이 안 돼"

```
/c4-quick "fix login button not responding on mobile"

  ● Task T-011-0 created
    DoD: touch event handler added, tested on viewport <768px

  ◆ [worker] implementing fix...
  ✓ submitted  →  review passed  →  done

  Changed: src/components/LoginButton.tsx (+3 -1)
```

완료. 하나의 태스크, 하나의 워커, 간단하게.

## /c4-plan과의 차이점

| | `/c4-quick` | `/c4-plan` |
|---|---|---|
| 태스크 | 1개 (직접 설명) | 여러 개 (CQ가 분해) |
| Discovery | 없음 | 있음 — 명확화 질문 |
| Design | 없음 | 아키텍처 결정 |
| 적합한 경우 | 명확하고 작은 변경 | 새 기능, 리팩토링 |

## 또 다른 예시

> **You:** "API 응답에서 null 체크 빠진 것 같아, UserProfile 컴포넌트"

```
/c4-quick "add null guard for API response in UserProfile"

  ● Task T-012-0 created
    DoD: null/undefined handled, no console errors on empty profile

  ◆ [worker] implementing...
  ✓ done

  Changed: src/components/UserProfile.tsx (+5 -2)
```

# R- 완료 선검증 (Review Gate)

CP 태스크의 `dependencies`에 T- 태스크가 직접 포함되어 있으면 **즉시 중단**하고 리뷰 레이어 누락을 리포트.

## 검증 규칙

| CP deps 구조 | 판정 | 동작 |
|-------------|------|------|
| R- 태스크만 있고 모두 done | ✅ 통과 | Step 1로 진행 |
| R- done + T- (review_required=False 명시 사유 있음) | ⚠️ 경고 | 사유 확인 후 진행 |
| T- 직결 + R- 없음 (사유 없음) | ⛔ **BLOCK** | 아래 처리 |
| R- pending/in_progress | ⏳ 대기 | R- 완료 후 재진입 |

## BLOCK 처리 (T- 직결, R- 없음)

```
⛔ CP-XXX 진입 불가 — 리뷰 레이어 누락

다음 T- 태스크에 대응하는 R- 태스크가 없습니다:
  - T-XXX-0: {title}  ← R-XXX-0 없음

원인: 계획 단계에서 review_required=False로 생성됨

선택지:
  1. 긴급 리뷰 태스크 생성 → /c4-run으로 R- 완료 → 재진입
  2. 재계획 (REPLAN) — /c4-plan으로 돌아가 태스크 구조 수정
```

> 예외 없음: 리뷰 레이어 없이 CP를 통과하면 "DoD 없는 작업은 작업이 아니다" 원칙 위반.

## 검증 코드

```python
status = mcp__c4__c4_status()
# CHECKPOINT 상태이면 대기 중인 CP 태스크 확인
# CP deps에서 T- 태스크 추출 → 대응 R- 태스크 존재+완료 여부 검증
```

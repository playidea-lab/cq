---
name: company-review
description: |
  PI Lab 표준 코드 리뷰 스킬. PR/MR 리뷰, diff 리뷰, 코드 변경사항 검토 요청 시
  반드시 이 스킬을 사용하세요. soul.md 판단 기준 + review-criteria.md 6축 평가
  (Correctness, Security, Reliability, Observability, Test Coverage, Readability)를
  적용합니다. "리뷰해줘", "PR 리뷰", "MR 리뷰", "코드 리뷰", "code review",
  "diff 확인", "변경사항 검토", "6축 평가", "company review" 등의 요청에 트리거됩니다.
  단순 코드 질문이나 리팩토링 조언이 아닌, 변경사항에 대한 체계적 리뷰가 필요할 때 사용합니다.
---

# Company Standard Code Review

PI Lab의 코드 리뷰 표준을 적용하여 변경 사항을 리뷰합니다.

## 리뷰 프로세스

### 1단계: 변경 범위 파악
- `git diff` 또는 MR diff를 확인
- 변경된 파일, 줄 수, 영향 범위 파악
- 관련 테스트 변경 여부 확인

### 2단계: 6축 평가 (review-criteria.md 기준)

각 축에 대해 **Pass / Issue / N/A** 판정:

| 축 | 확인 사항 |
|----|----------|
| **Correctness** | 요구사항 충족, 엣지 케이스, race condition |
| **Security** | 입력 검증, 시크릿 노출, 인증/권한 |
| **Reliability** | 실패 처리, timeout, 리소스 누수 |
| **Observability** | 로깅, 메트릭, 장애 진단 가능성 |
| **Test Coverage** | 핵심 로직 테스트, 에러 경로 테스트 |
| **Readability** | 명명, 복잡도, 주석 품질 |

### 3단계: 우선순위 적용 (soul.md 기준)

문제 발견 시 우선순위:
1. 데이터 무결성 / 보안 / 권한
2. 장애 복구 가능성
3. 관측 가능성
4. 테스트 커버리지 / 회귀 위험
5. 가독성 / 스타일

### 4단계: 판정

- **Approve**: 6축 모두 Pass 또는 경미한 코멘트만
- **Request Changes**: 1-4축(정확성/보안/신뢰성/관측성)에 문제
- **Comment**: 5-6축(테스트/가독성)에 개선 제안

### 5단계: 리뷰 출력 형식

```markdown
## Review Summary

**판정**: Approve / Request Changes / Comment
**변경 범위**: [파일 수]개 파일, [추가/삭제] 줄

### 6축 평가
| 축 | 판정 | 비고 |
|----|------|------|
| Correctness | ✅/⚠️/❌ | ... |
| Security | ✅/⚠️/❌ | ... |
| Reliability | ✅/⚠️/❌ | ... |
| Observability | ✅/⚠️/❌ | ... |
| Test Coverage | ✅/⚠️/❌ | ... |
| Readability | ✅/⚠️/❌ | ... |

### Issues (있는 경우)
1. [심각도] 파일:줄 — 설명

### Suggestions (선택)
- ...
```

## 비타협 체크리스트 (soul.md)

리뷰 시 반드시 확인:
- [ ] DoD(Definition of Done) 충족 여부
- [ ] 테스트 동반 여부 (예외 시 사유 기록)
- [ ] PR 크기 적절성 (큰 PR은 분리 요청)
- [ ] 시크릿 코드 포함 여부

# 예시: 품질 게이트 실전

CQ가 신뢰가 아닌 시스템으로 품질을 강제하는 방법.

::: info solo 티어
품질 게이트는 CQ 바이너리에 컴파일됩니다. 모든 티어에서 동작하며 설정이 필요 없습니다.
:::

## 시나리오

결제 웹훅 핸들러를 추가합니다. 민감한 코드를 다루므로 CQ의 전체 품질 시스템이 동작하길 원합니다.

## Step 1 — Critique가 포함된 계획

> **You:** "Stripe webhook 핸들러 추가해줘. 결제 성공, 실패, 환불 이벤트 처리"

```
/c4-plan "Stripe webhook handler"

  ● Discovery
    Q: 서명 검증?          → HMAC-SHA256
    Q: 멱등성 처리?        → 이벤트 ID로 중복 제거
    Q: 재시도 동작?        → Stripe가 3회 재시도, 멱등이어야 함

  ● Tasks (5개)
    T-001  웹훅 라우터 + 서명 검증
    T-002  결제 성공 핸들러
    T-003  결제 실패 핸들러
    T-004  환불 핸들러
    T-005  Stripe mock 통합 테스트

  ● Critique loop 발동 (5개 태스크 → refine gate)
    Round 1: "T-001에 rate limiting 없음" → DoD에 추가
    Round 2: "리플레이 공격 방지 없음" → 타임스탬프 체크 추가
    Round 3: CRITICAL 0, HIGH 0 → 수렴 ✅
```

**Refine gate**는 4개 이상 태스크 계획에 critique loop 통과를 요구합니다. Go 레벨 검사라 건너뛸 수 없습니다.

## Step 2 — Polish가 포함된 워커

```
/c4-run

  ◆ T-001  [worker-a]  구현 중...
    Polish round 1: 리뷰어가 에러 로그 누락 발견 → 수정
    Polish round 2: 수정 0건 → 수렴
    ✓ 제출 (sha: b2c4e91)

  ◆ T-002  [worker-b]  구현 중...
    Polish: diff < 5줄 → 자동 스킵
    ✓ 제출 (sha: 7f3a82d)
```

각 워커는 제출 전 **polish 루프**를 실행합니다:
1. 코드 리뷰 에이전트 스폰 (6축 평가)
2. 발견된 이슈 수정
3. 수정 0건이 될 때까지 반복

**Polish gate**는 `c4_submit`에서 5줄 이상 diff에 self-review 없이 제출을 거부합니다. 5줄 미만은 자동 승인.

## Step 3 — 자동 리뷰

각 태스크 제출 후 CQ가 자동으로 리뷰 태스크를 생성합니다:

```
  ✓ T-001 제출 → R-001 생성 (6축 리뷰)

  R-001 리뷰:
    ✅ 정확성     — 서명 검증 올바름
    ✅ 보안       — HMAC 비교가 constant-time
    ✅ 신뢰성     — 멱등성 키로 중복 처리 방지
    ✅ 관측성     — 모든 경로에 구조화된 로깅
    ⚠️ 테스트    — 엣지 케이스 누락: 만료된 타임스탬프
    ✅ 가독성     — 명확한 네이밍, 좋은 구조

  결정: 수정 요청 → T-001-1 리비전 생성
```

리뷰가 테스트 케이스 누락을 발견했습니다. CQ가 자동으로 리비전 태스크(`T-001-1`)를 생성합니다.

## Step 4 — 리비전 사이클

```
  ◆ T-001-1  [worker-d]  만료된 타임스탬프 테스트 추가 중...
    ✓ 제출

  R-001-1 리뷰:
    ✅ 6축 모두 통과
    결정: 승인 ✅
```

태스크가 리뷰에서 3번 실패하면(최대 리비전), CQ가 멈추고 사람의 판단을 요청합니다. 대부분 1~2회 리비전으로 통과합니다.

## 세 가지 게이트

| 게이트 | 시점 | 강제 사항 | 수준 |
|--------|------|----------|------|
| **Refine** | `/c4-plan`이 4개+ 태스크 생성 | critique loop 필수 | Go 바이너리 |
| **Polish** | 워커가 코드 제출 (diff ≥ 5줄) | self-review 수렴 필수 | Go 바이너리 |
| **Review** | 모든 구현 태스크 이후 | 별도 에이전트의 6축 평가 | Go 바이너리 |

이것들은 **CQ 바이너리에 컴파일**되어 있습니다. 프롬프트도, 제안도, 선택사항도 아닙니다. 시스템이 구조적으로 품질을 강제합니다.

## 결과

```
/c4-finish

  ● Polish: 1라운드 → 수정 0건
  ● Build:  go build ./... ✓
  ● Tests:  189개 통과, 0 실패
  ● Commit: feat(webhook): Stripe payment webhook with idempotency
```

이면에서 일어난 일:
- 5개 태스크 계획, critique-loop 검증
- 3개 워커가 polish gate로 self-review
- 2건의 리뷰 이슈를 자동 수정
- 사람 개입 0회

## 다음 단계

- **감시 없이 배포**: → [아이디어에서 배포까지](/ko/examples/idea-to-ship)
- **원격 GPU에서 실행**: → [분산 실험](/ko/examples/distributed-experiments)

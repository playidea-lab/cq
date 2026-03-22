# PR Review

PR 생성 시 자동 체크리스트 + 리뷰 가이드.

## 트리거

"PR 리뷰", "풀리퀘스트", "코드 리뷰", "MR 리뷰", "pr review", "리뷰 요청"

## Steps

### 1. 변경 범위 파악

```bash
git diff --stat main...HEAD
git log --oneline main...HEAD
```

- 변경 파일 수, 추가/삭제 줄 수 확인
- 대규모 PR (500줄+) → 분할 권고

### 2. 자동 체크리스트

PR 생성 시 아래 항목을 체크리스트로 추가:

**필수 (Merge blocker):**
- [ ] 빌드 통과 (CI green)
- [ ] 테스트 추가/수정 (새 기능이나 버그 수정에 해당 시)
- [ ] Breaking change 없음 (있으면 명시 + 마이그레이션 가이드)
- [ ] 시크릿/하드코딩된 키 없음

**권장:**
- [ ] PR 설명에 "무엇을, 왜" 포함
- [ ] 영향 받는 문서 업데이트
- [ ] 스크린샷 (UI 변경 시)
- [ ] 성능 영향 검토 (DB 쿼리, API 호출 추가 시)

### 3. 리뷰 우선순위 (soul.md 기반)

1. **데이터 무결성 / 보안 / 권한** — 사용자 데이터 손상, 인증 우회
2. **장애 복구** — rollback, idempotency, graceful degradation
3. **관측 가능성** — logging, metrics, tracing
4. **테스트** — 커버리지, 회귀 위험
5. **가독성** — 명명, 구조, 주석

### 4. 리뷰 코멘트 작성

- **blocking**: 수정 필수 (보안, 정확성, 장애 위험)
- **suggestion**: 개선 권장 (가독성, 성능, 스타일)
- **question**: 의도 확인 (왜 이렇게 했는지)
- **nit**: 사소한 것 (오타, 포맷). 절대 merge를 막지 않음.

각 코멘트에 카테고리를 명시: `[blocking]`, `[suggestion]`, `[question]`, `[nit]`

### 5. 결정

| 상태 | 조건 |
|------|------|
| **Approve** | blocking 없음 |
| **Request Changes** | blocking 1개 이상 |
| **Comment** | suggestion만 있고 blocking 없음 |

## PR 크기 가이드

| 크기 | 줄 수 | 리뷰 시간 | 권장 |
|------|-------|----------|------|
| S | <100 | 15분 | 이상적 |
| M | 100-300 | 30분 | 좋음 |
| L | 300-500 | 1시간 | 분할 검토 |
| XL | 500+ | - | 반드시 분할 |

## 안티패턴

- "LGTM" 한 줄 리뷰 (실제로 읽지 않음)
- 스타일 nit으로 merge 차단
- 리뷰 없이 self-merge (핫픽스 제외)
- 리뷰 요청 후 3일 이상 방치

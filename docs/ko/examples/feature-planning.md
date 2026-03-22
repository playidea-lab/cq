# 예시: 기능 계획

병렬 워커로 새 기능을 end-to-end로 빌드합니다.

## 준비

```sh
cd your-project
cq claude          # CQ 초기화
# Claude Code 재시작
```

## 1단계 — 원하는 것 설명

> **You:** "JWT 인증 추가해줘. Google이랑 GitHub 로그인"

CQ의 `/c4-plan`이 자동으로 트리거되어 구조화된 discovery 프로세스를 실행합니다:

```
/c4-plan "JWT auth with Google and GitHub OAuth"

  ● Discovery
    Q: 세션 저장소 — Redis 아니면 DB?         → DB (stateless 선호)
    Q: 토큰 만료 — access/refresh 분리?        → Yes, 15분 / 7일
    Q: 기존 유저 모델?                          → users 테이블 존재

  ● Design
    Provider 추상화 → Google → GitHub → Session middleware
    JWT: RS256, httpOnly cookie에 저장

  ● Tasks created
    T-001  OAuth provider interface
    T-002  Google provider
    T-003  GitHub provider
    T-004  JWT middleware + session store
    T-005  통합 테스트
```

각 태스크에는 **완료 조건(DoD)** — 태스크가 완료로 판정되려면 통과해야 하는 체크리스트가 있습니다.

## 2단계 — 워커 실행

> **You:** "ㄱㄱ"

```
/c4-run

  ◆ T-001  [worker-a] worktree: c4/w-T-001-0  ████████░░  구현 중...
  ◆ T-002  [worker-b] worktree: c4/w-T-002-0  ████░░░░░░  구현 중...
  ◆ T-003  [worker-c] worktree: c4/w-T-003-0  ██░░░░░░░░  구현 중...
  ◆ T-004  T-001 완료 대기 중

  ✓ T-001  submitted (sha: a3f8c21)  →  R-001 리뷰 대기
  ✓ T-002  submitted (sha: 7b2e94d)  →  R-002 리뷰 대기
  ...
  ✓ 모든 태스크 완료. /c4-finish로 마무리하세요.
```

워커가 **병렬**로 실행됩니다 — 각자 격리된 git 워크트리에서 충돌 없이 작업합니다. 리뷰 태스크(`R-001`, `R-002`)가 각 구현 완료 후 자동으로 생성됩니다.

## 3단계 — 진행 상황 확인

> **You:** "지금 어디까지 됐어?"

```
/c4-status

  Phase: EXECUTE  ████████████░░░░  75%

  ✓ T-001  OAuth interface      [merged]
  ✓ T-002  Google provider      [merged]
  ▶ T-003  GitHub provider      [리뷰 중]
  ◷ T-004  JWT middleware        [T-003 대기]
  ◷ T-005  통합 테스트           [T-004 대기]

  Workers: 1 active  |  Queue: 2 pending  |  Knowledge: 8 records
```

## 4단계 — 마무리

> **You:** "마무리해줘"

```
/c4-finish

  ● Polish: 2 rounds → 0 changes (CONVERGED)
  ● Build:  go build ./... ✓
  ● Tests:  247 passed, 0 failed
  ● Docs:   CHANGELOG updated
  ● Commit: feat(auth): JWT OAuth with Google and GitHub (sha: f4a9c31)
```

## 뒤에서 일어난 일

- **Refine 게이트**가 5개 태스크 계획을 실행 전 critique 루프를 거치게 강제했습니다
- 각 워커가 DoD + 과거 지식 + 페르소나 힌트를 컨텍스트에 주입받았습니다
- **Polish 게이트**가 각 워커의 자체 리뷰를 제출 전 검증했습니다 (Go 레벨 강제)
- 리뷰에서 T-002와 T-003에 걸쳐 3개 이슈를 발견하여 수정 태스크가 자동 생성됩니다 (최대 3회)
- 이번 세션의 모든 발견이 기록됐습니다 — **페르소나 온톨로지**가 당신의 인증 선호도를 학습합니다

# CQ — AI 프로젝트 오케스트레이션 엔진

**한국어** | [English](README.md)

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

**CQ**는 Claude Code를 위한 프로젝트 관리 엔진입니다.
계획 수립부터 구현, 리뷰, 배포까지 전체 개발 라이프사이클을 C4 Engine 기반의 구조화된 워크플로우로 자동화합니다.
Soul 진화와 POP(Personal Ontology Pipeline)을 통해 시간이 지날수록 사용자의 엔지니어링 스타일을 학습합니다.

## 티어

환경에 맞는 티어를 선택하세요:

| 티어 | 설명 | 사용 시기 |
|------|------|----------|
| `solo` | 로컬 전용, 외부 의존성 없음 | 개인 / 오프라인 |
| `connected` | + Supabase, LLM Gateway, EventBus | 팀 / 클라우드 동기화 |
| `full` | + Hub, Drive, CDP, GPU, C1 Messenger | 풀 프로덕션 |

```sh
# 특정 티어 설치 (기본값: solo)
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh -s -- --tier connected
```

## 빠른 시작

```sh
# 1. 환경 점검
cq doctor

# 2. 로그인 (connected / full 티어)
cq auth login              # 브라우저에서 GitHub OAuth → 자동 설정
cq auth login --device     # 헤드리스/SSH: 브라우저에 입력할 user_code 표시
cq auth login --link       # 수동으로 열 auth URL 출력

# 3. 프로젝트에 C4 초기화 (.mcp.json + CLAUDE.md 생성)
cd your-project
cq claude   # Claude Code용
cq cursor   # Cursor용

# 4. Claude Code 실행 — C4 MCP 도구 사용 가능
#    시작 시 로그인 상태 표시:
#      ✓ Cloud: user@example.com (expires in 47h)
#      → Run 'cq auth login' to enable cloud sync & hub access
```

## 사용 예시

### 시나리오 1 — 새 기능 개발

> **You:** "JWT 인증 추가해줘. Google이랑 GitHub 로그인"

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
  ✓ 모든 태스크 완료 → 자동 polish → /c4-finish → done
```

---

### 시나리오 2 — 빠른 버그 수정

> **You:** "모바일에서 로그인 버튼 클릭이 안 돼"

```
/c4-quick "fix login button not responding on mobile"

  ● Task T-011-0 생성
    DoD: touch 이벤트 핸들러 추가, viewport <768px 테스트 완료

  ◆ [worker] 수정 중...
  ✓ submitted  →  리뷰 통과  →  완료

  변경: src/components/LoginButton.tsx (+3 -1)
```

---

### 시나리오 3 — 진행 상황 확인

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

---

### 시나리오 4 — 분산 머신 실험

> **You:** "backbone 3개 비교 실험 돌려야 해. ResNet / EfficientNet / ViT"

먼저 C5 Hub를 시작합니다 (한 번만):

```sh
# .c4/config.yaml에 serve.hub.enabled: true 설정 후
cq serve   # C5 Hub가 자동으로 서브프로세스로 시작됨
```

```
/c4-plan "backbone ablation: ResNet50 vs EfficientNet-B4 vs ViT-B/16"

  ● Tasks created
    T-020  train ResNet50       (GPU: 1x A100, 예상 4h)
    T-021  train EfficientNet   (GPU: 1x A100, 예상 3h)
    T-022  train ViT-B/16       (GPU: 1x A100, 예상 6h)
    T-023  결과 비교 + 리포트
```

각 머신에서 `/c4-standby`로 워커 등록 후 결과 확인:

```
/c4-status

  ✓ T-020  ResNet50       MPJPE: 48.3mm  PA-MPJPE: 34.1mm  [worker-m1]
  ✓ T-021  EfficientNet   MPJPE: 44.7mm  PA-MPJPE: 31.8mm  [worker-m2]  ← best
  ▶ T-022  ViT-B/16       실행 중 3h 42m / ~6h               [worker-m3]
  ◷ T-023  T-020, T-021, T-022 대기 중
```

---

## 세션

이름으로 이전 Claude Code 세션을 재개합니다:

```sh
cq claude -t myproject    # 이름 있는 세션 시작 또는 재개
cq ls                     # 세션 목록 (미읽은 메일 수 표시)
```

기존 세션에 이름 붙이기: Claude Code에서 `/c4-attach <name>` 실행.

## Soul & 학습

CQ는 사용자의 코딩 스타일, 어조, 선호도를 학습하여 에이전트의 행동 방식을 지속적으로 진화시킵니다.

```sh
scripts/soul-check.sh    # 소울 상태 및 마지막 진화 시점 확인
scripts/soul-evolve.sh   # 누적된 패턴 기반 페르소나 진화 실행
```

### POP — Personal Ontology Pipeline

POP는 대화 메시지에서 지식 제안을 자동 추출하여 Soul에 결정화(crystallize)합니다.

```
Extract → Consolidate → Propose → Validate → Crystallize
```

```sh
cq pop status   # 파이프라인 상태, gauge 값, 제안 통계 표시
```

Claude Code에서 MCP 도구 사용:
- `c4_pop_extract` — 추출 사이클 실행 (C1 Messenger 연결 필요)
- `c4_pop_status` — 파이프라인 상태 + gauge + knowledge 통계 조회
- `c4_pop_reflect` — 고신뢰도(≥0.8) 제안 목록 조회

> **주의**: POP는 메시지 소스로 C1 Messenger(`full` 티어)가 필요합니다. `solo`/`connected` 모드에서는 `c4_pop_extract`가 성공을 반환하지만 실제 추출은 수행되지 않습니다.

---

## 메일 (세션 간 메시징)

세션 간 또는 CLI에서 메시지를 주고받습니다:

```sh
cq mail ls          # 메시지 목록
cq mail read <id>   # 메시지 읽기
```

Claude Code에서는 `c4_mail_send` / `c4_mail_ls` / `c4_mail_read` / `c4_mail_rm` MCP 도구로 세션 간 통신합니다.

## 워크플로우

```
/c4-plan "기능 설명"   → discovery + design + 태스크 생성
/c4-run               → 워커 스폰, 병렬 구현
/c4-finish            → 빌드 · 테스트 · 문서 · 커밋
/c4-status            → 언제든지 진행 상황 확인
/pi "아이디어..."      → 계획 수립 전 ideation 모드 (발산 → 결정 → /c4-plan)
```

### 컨텍스트 효율화

대용량 출력을 생성하는 명령어는 Bash 대신 `c4_execute`를 사용하세요 — 결과를 자동 압축하여 컨텍스트 소비를 최소화합니다:

```
c4_execute({"command": "go test ./..."})   # test 모드: 실패 항목만 추출
c4_execute({"command": "git log"})         # git 모드: hash + subject만 유지
c4_execute({"command": "go build ./..."})  # build 모드: 에러/경고만 추출
```

`c4_execute` 권장 명령: `go test`, `git log`, `git diff`, `find`, `cargo test`, `npm test`, `make`
파이프 체인(`cmd | cmd`)이나 짧은 one-liner는 Bash 사용.

## 설정

`solo` 티어는 별도 설정 없이 바로 사용 가능합니다.

`connected` / `full` 티어:

```sh
cq auth login              # GitHub OAuth → .c4/config.yaml 자동 설정
cq auth login --device     # 헤드리스/SSH: 브라우저에 입력할 user_code 표시
cq auth login --link       # 수동으로 열 auth URL 출력
```

로그인 후 클라우드 동기화와 Hub 접근이 자동으로 활성화됩니다.

## 업데이트

동일한 설치 명령을 다시 실행하면 최신 릴리즈로 업데이트됩니다.

## 요구 사항

- macOS Apple Silicon (arm64) 또는 Linux (amd64 / arm64)
- [Claude Code](https://claude.ai/code) 설치됨
- `curl` 사용 가능

## 라이선스

[MIT + Commons Clause](LICENSE) — 자유롭게 사용 및 수정 가능, 상업적 재판매 금지.

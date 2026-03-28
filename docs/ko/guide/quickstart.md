# 빠른 시작

5분 안에 CQ로 무언가를 만들어보세요.

## 시작 전

먼저 [설치](install.md)를 완료한 다음 실행하세요:

```sh
cq doctor    # 모든 것이 정상 작동하는지 확인
```

## 핵심 워크플로우

```
cq doctor → cq auth login → cq claude
                                 │
                                 ▼
                           /c4-plan "목표"
                                 │
                                 ▼
                            /c4-run
                           (반복)
                                 │
                                 ▼
                           /c4-finish
```

## 1단계: 상태 확인

```sh
cq doctor
```

```
[✓] cq binary: v1.37
[✓] Claude Code: installed
[✓] MCP server: connected
[✓] .c4/ directory: initialized
```

실패 항목이 있으면 계속하기 전에 수정하세요.

## 2단계: 로그인

```sh
cq auth login    # GitHub OAuth — 브라우저 열기
```

최초 1회 설정. CQ는 토큰을 클라우드 동기화, Growth Loop, 지식 베이스에 사용합니다.

## 3단계: Claude Code 열기

```sh
cq claude    # CQ MCP가 연결된 상태로 Claude Code 실행
```

CQ가 AI 도구를 자동으로 감지합니다. 다음을 사용할 수도 있습니다:

```sh
cq cursor     # Cursor
cq codex      # OpenAI Codex CLI
cq gemini     # Google Gemini
```

## 4단계: 계획 수립

Claude Code에서 만들고 싶은 것을 설명하세요:

```
/c4-plan "JWT 사용자 인증 추가"
```

CQ는 세 단계를 거칩니다:

1. **Discovery** — 요구사항 수집 (EARS 형식)
2. **Design** — 아키텍처 및 ADR 제안
3. **Plan** — 완료 기준(DoD)이 있는 검증 가능한 태스크로 분해

각 단계는 진행 전에 승인이 필요합니다.

## 5단계: 실행

```
/c4-run
```

Worker가 자동으로 태스크를 처리합니다:

- 각 Worker는 하나의 태스크, 새로운 컨텍스트, 격리된 워크트리를 가집니다
- 제출 전에 lint와 테스트 실행
- 큐가 빌 때까지 자동 재스폰
- 품질 게이트가 리뷰를 건너뛴 제출을 거부

설정하고 돌아오세요 — 큐는 스스로 처리됩니다.

## 6단계: 마무리

```
/c4-finish
```

코드를 다듬고, 전체 리뷰 사이클을 실행하고, 깔끔한 커밋을 만듭니다. DoD 체크리스트를 확인합니다.

## 언제든지 상태 확인

```
/c4-status
```

```
## CQ 프로젝트 상태

상태:    EXECUTE
큐:      3 pending | 2 in-progress | 7 done
Worker:  2 active
```

## 시나리오: 새 기능

```
/c4-plan "보고서 페이지에 CSV 내보내기 추가"
/c4-run
/c4-finish
```

## 시나리오: 버그 수정 (Direct 모드)

전체 계획 파이프라인이 필요 없는 단일 파일 수정의 경우:

```
refresh token이 없을 때 auth/token.go의 null pointer 수정
```

Claude가 직접 처리합니다. 또는 명시적인 플로우를 사용하세요:

```
c4_claim → 변경 → c4_report
```

## 커맨드 레퍼런스

| 커맨드 | 동작 | 사용 시점 |
|--------|------|---------|
| `cq doctor` | 상태 확인 | 시작 전 |
| `cq auth login` | GitHub OAuth | 최초 1회 |
| `cq claude` | Claude Code 실행 | 매 세션 |
| `/c4-status` | 프로젝트 상태 표시 | 언제든지 |
| `/c4-plan "목표"` | 계획 + 태스크 생성 | 새 기능 |
| `/c4-run` | Worker 시작 | 계획 수립 후 |
| `/c4-finish` | 다듬기 + 커밋 | 구현 완료 후 |
| `/pi "아이디어"` | 브레인스토밍 + 조사 | 계획 전 |

## 자동 라우팅

CQ가 범위에 따라 요청을 자동으로 라우팅합니다:

| 크기 | 기준 | 워크플로우 |
|------|------|----------|
| Small | 타이포, 1–2줄 변경 | 직접 수정 |
| Medium | 1–3개 파일, 함수 변경 | `/c4-quick` → Worker 1개 |
| Large | 새 기능, 설계 필요 | `/pi` → `/c4-plan` → `/c4-run` → `/c4-finish` |

판단이 애매하면 CQ는 더 작은 옵션을 선택합니다 — 과설계보다 빠른 실행이 낫습니다.

## 세션이 종료되면 어떻게 되나요?

세션을 닫으면 CQ가 자동으로:

1. 결정과 발견을 요약
2. 작업 방식에서 선호도 추출
3. 지식 베이스에 저장
4. [Growth Loop](growth-loop.md)에 반영

다음 세션에서 선호도가 이미 반영되어 있습니다.

## 다음 단계

- [티어](tiers.md) — solo / connected / full 이해
- [Growth Loop](growth-loop.md) — CQ가 선호도를 학습하는 방법
- [Worker 설정](worker-setup.md) — 훈련 작업을 위한 GPU Worker 추가
- [Remote Brain](remote-brain.md) — ChatGPT 또는 Claude Desktop에서 CQ 접근

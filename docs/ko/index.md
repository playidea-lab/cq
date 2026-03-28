---
layout: home

hero:
  name: "CQ"
  text: "AI를 위한 외장 두뇌"
  tagline: "AI는 빠르게 코딩하지만 아무것도 기억하지 못합니다. CQ는 AI에게 없는 두뇌입니다 — 계획하고, 검증하고, 세션을 넘어 기억하며, 당신을 닮아갑니다."
  actions:
    - theme: brand
      text: 시작하기
      link: /ko/guide/install
    - theme: alt
      text: GitHub 보기
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 📋
    title: 코딩 전에 먼저 계획
    details: "다른 AI 도구들은 바로 코드로 뛰어듭니다. CQ는 먼저 '무엇을 만들 것인가'를 묻습니다 — 요구사항, 아키텍처, DoD. 그 다음 검증 가능한 태스크로 분해합니다."

  - icon: 🔒
    title: 신뢰가 아닌 게이트
    details: "Refine 게이트는 나쁜 계획을 차단합니다. Polish 게이트는 리뷰 없는 코드를 차단합니다. Review 게이트는 6축 평가를 실행합니다. 바이너리에 컴파일되어 있어 — 선택 사항이 아닙니다."

  - icon: 🧠
    title: "Growth Loop: 당신을 배우는 AI"
    details: "세션 1: 당신의 선호도를 설명합니다. 세션 5: CQ는 이미 알고 있습니다. 선호도가 쌓이고 → 자동으로 규칙으로 승격 → AI 동작이 바뀝니다. 단순한 메모리가 아닙니다. 성장입니다."

  - icon: 🔄
    title: 모든 세션이 지식이 된다
    details: "세션이 종료될 때 결정, 선호도, 발견을 자동으로 캡처합니다. 지식은 도구를 넘어 쌓입니다 — Claude에서 배운 것이 ChatGPT에서도 활용됩니다."

  - icon: ⚡
    title: Zero Config
    details: "curl로 설치 → cq → 시작. API 키 불필요. 설정 파일 불필요. 두뇌는 클라우드에, 손발은 내 머신에."

---

## 동작 원리

```
 당신이 말하면     CQ가 실행            결과
─────────────────────────────────────────────────────────────
 "이런 거 만들자"   /pi → 브레인스토밍 + 조사     idea.md
 "만들어"          /c4-plan → 태스크 + 리뷰       계획
 ⏳               /c4-run → 병렬 Worker          코드 + 테스트
 ☕               /c4-finish → 다듬기 + 검증      완성
```

모든 단계는 **게이트**로 제어됩니다: 계획은 비판 리뷰가 필요하고, 구현은 polish가 필요하며, 리뷰는 6축 평가가 필요합니다. 통과 없이는 아무것도 배포되지 않습니다.

---

## 주요 지표

| 지표 | 값 |
|------|---|
| 완료된 태스크 | 2,500+ |
| MCP 도구 | 169+ |
| Skills | 42개 (★ core / [internal]) |
| 지원 AI 도구 | Claude, Cursor, Codex, Gemini, ChatGPT |
| 설치 시간 | 2분 |
| 필요한 API 키 | 0개 |
| 지원 언어 | Go, Python, TypeScript, Rust |

---

## CQ가 다른 이유

### 🧠 Growth Loop — 당신을 배우는 AI

대부분의 AI 도구는 매 세션을 처음부터 시작합니다. CQ는 이 루프를 닫습니다: **세션 → 선호도 → 규칙 → 동작 변화**.

```
세션 1:  "항상 MPJPE를 먼저 확인해"라고 말함     → 저장됨 (count: 1)
세션 3:  같은 선호도 3번째 감지됨                 → CLAUDE.md에 힌트 추가
세션 5:  5번째                                   → 규칙으로 승격
세션 6+: AI가 물어보지 않고도 MPJPE를 먼저 확인
```

**실제 사례** — 메시 복원 프로젝트의 5번의 연구 세션 후, CQ가 자동으로 학습한 패턴:

| 횟수 | 레벨 | CQ가 학습한 것 |
|------|------|--------------|
| 5x | **Rule** | "Hub를 통해 자동으로 실험 실행" |
| 4x | Hint | "`@key=value` 메트릭 출력 형식 사용" |
| 4x | Hint | "MPJPE/HD/MSD 메트릭 먼저 확인" |

이 규칙들은 `CLAUDE.md`와 `.claude/rules/`에 기록되어 — 이후 모든 세션의 시스템 프롬프트에 로드됩니다. 규칙을 삭제하면 영구적으로 억제됩니다.

지식은 외부로도 흐릅니다: 프로젝트 인사이트는 **비개인화** (경로, 이메일, 사용자명 제거) 후 커뮤니티 풀에 공유되어, 다른 사람들이 당신의 시행착오를 건너뛸 수 있습니다.

### 🔒 품질은 선택 사항이 아닙니다

AI는 코드를 빠르게 작성합니다. 하지만 누가 검증하나요? CQ는 시스템 수준에서 품질을 강제합니다:

- **Polish 게이트**: `c4_submit`은 리뷰되지 않은 코드를 거부합니다 (diff ≥ 5줄)
- **Refine 게이트**: `c4_add_todo`는 비판 없는 계획을 거부합니다 (배치 ≥ 4 태스크)
- **Review 태스크**: 모든 구현에 6축 리뷰가 붙습니다 (정확성, 보안, 신뢰성, 관측성, 테스트, 가독성)

이것들은 제안이 아닙니다. **우회할 수 없는** Go 수준의 게이트입니다.

### 🖥️ 팀이 24/7 작동합니다

각 Worker는 하나의 태스크, 새로운 컨텍스트, 격리된 워크트리를 가집니다. 컨텍스트 오염 없음. 간섭 없음.

```sh
/c4-run    # 병렬 Worker 스폰, 큐가 빌 때까지 자동 재스폰
```

자기 전에 설정하세요. 아침에 일어나면 커밋되고, 리뷰되고, 테스트된 코드가 기다립니다.

---

## 어떤 AI와도 작동

CQ는 오케스트레이션 레이어입니다. AI는 교체 가능합니다:

```sh
cq           # Claude Code, Cursor, Codex, Gemini 자동 감지
cq claude    # 또는 명시적으로 지정
cq cursor
cq codex
cq gemini
cq chatgpt   # 브라우저 세션 열기
```

### Remote MCP — ChatGPT, Cursor, Claude Desktop에서 사용

[mcp.pilab.kr](https://mcp.pilab.kr)을 통해 MCP 호환 AI를 CQ 두뇌에 연결하세요:

```json
"cq-brain": {
  "url": "https://mcp.pilab.kr/mcp",
  "type": "streamable-http"
}
```

GitHub OAuth 로그인 → 지식 베이스가 모든 도구에 공유됩니다.

### Skill 마켓플레이스

커스텀 Skill을 공유하고 발견하세요:

```sh
cq craft publish my-skill    # 레지스트리에 발행
cq craft search "deploy"     # 커뮤니티 Skill 검색
cq craft install user/skill  # 한 커맨드로 설치
```

---

## 시작하기

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq               # 로그인, 서비스 설치, 실행 — 모두 자동
```

그 다음 원하는 것을 말하세요. CQ가 자동으로 라우팅합니다: 소규모 수정은 직접 처리하고, 중간 규모 태스크는 `/c4-quick`으로, 큰 기능은 전체 `/pi` 계획 파이프라인을 거칩니다.

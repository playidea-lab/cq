---
layout: home

hero:
  name: "CQ"
  text: "설치. 로그인. 빌드."
  tagline: "API 키 없이. 설정 파일 없이. cq auth 하나로 시작. 두뇌는 클라우드에, 당신의 머신은 손과 발."
  actions:
    - theme: brand
      text: 시작하기
      link: /ko/guide/install
    - theme: alt
      text: GitHub 보기
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: ⚡
    title: 2분 설치
    details: "curl 설치, cq auth login, 끝. API 키 관리도, 설정 파일 작성도 없습니다. 태스크, 지식, LLM, 품질 게이트 전부 클라우드."

  - icon: 💡
    title: 아이디어 → 출시 (논스탑)
    details: "/pi로 토론 → 자동 계획 → 자동 실행 → 자동 마무리. 생각에서 커밋까지 하나의 흐름. 감시 불필요."

  - icon: 🔒
    title: 시스템이 품질 보장
    details: "Refine, Polish, Review 게이트가 바이너리에 컴파일. 프롬프트도, 제안도 아닌 Go 레벨 강제. 건너뛸 수 없습니다."

  - icon: 🧠
    title: 당신을 학습
    details: "3층 온톨로지(개인→프로젝트→집단)가 1,200+ 태스크에서 패턴을 추적합니다. 세션마다 리뷰가 정확해집니다."

---

## 동작 방식

```
 당신이 말하면       CQ가 하는 일                      당신이 받는 것
─────────────────────────────────────────────────────────────────────
 "이런 거 만들자"     /pi → 토론 + 조사                  idea.md
 "만들어"            /c4-plan → 태스크 + 리뷰            계획
 ⏳                 /c4-run → 병렬 워커                 코드 + 테스트
 ☕                 /c4-finish → polish + 검증          출시 완료
```

모든 단계에 **게이트**가 있습니다: 계획은 critique 리뷰를 거치고, 구현은 polish를 거치고, 리뷰는 6축 평가를 거칩니다. 통과 없이는 아무것도 나가지 않습니다.

---

## 숫자로 보는 CQ

| 지표 | 값 |
|------|---|
| 완료된 태스크 | 1,200+ |
| 리뷰 승인률 | 93% |
| 설치 시간 (connected) | 2분 |
| 필요한 API 키 | 0개 (connected tier) |
| 지원 언어 | Go, Python, TypeScript, Rust |

---

## CQ가 다른 이유

### 🧠 당신을 학습합니다

대부분의 AI 코딩 도구는 매 세션마다 처음부터 시작합니다. CQ는 **3계층 온톨로지**로 패턴을 축적합니다:

- **L1 로컬**: 코딩 스타일, 리뷰 선호도, 반복되는 결정
- **L2 프로젝트**: 교차 포지션 패턴, 팀 컨벤션
- **L3 집단**: 커뮤니티의 공유 패턴

100개 태스크 후, CQ는 당신의 네이밍 컨벤션을 압니다. 500개 후, 리뷰 피드백을 예측합니다.

### 🔒 품질은 선택이 아닙니다

AI는 코드를 빠르게 씁니다. 하지만 누가 확인합니까? CQ는 시스템 레벨에서 품질을 강제합니다:

- **Polish 게이트**: `c4_submit`은 리뷰되지 않은 코드를 거부합니다 (diff ≥ 5줄)
- **Refine 게이트**: `c4_add_todo`는 비판 없는 계획을 거부합니다 (batch ≥ 4개)
- **리뷰 태스크**: 모든 구현에 6축 리뷰 (정확성, 보안, 신뢰성, 관측성, 테스트, 가독성)

이것은 권장이 아닙니다. **우회할 수 없는** Go 레벨 게이트입니다.

### 🖥️ 당신의 팀은 24시간 일합니다

각 워커는 하나의 태스크를 받고, 새로운 컨텍스트로, 격리된 worktree에서 작업합니다. 컨텍스트 오염 없이. 간섭 없이.

```sh
/c4-run    # 병렬 워커 스폰, 큐가 빌 때까지 자동 재스폰
```

자기 전에 시작하세요. 커밋되고, 리뷰되고, 테스트된 코드와 함께 일어납니다.

---

## 어떤 AI와도 동작

CQ는 오케스트레이션 레이어입니다. AI는 플러그형:

```sh
cq claude    # Claude Code (권장)
cq cursor    # Cursor
cq codex     # OpenAI Codex
cq gemini    # Gemini CLI
```

---

## 시작하기

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq claude
```

그리고 필요한 것을 말하세요.

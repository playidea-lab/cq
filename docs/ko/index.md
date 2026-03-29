---
layout: home

hero:
  name: "CQ"
  text: "진화하는 외장 두뇌"
  tagline: "AI는 빠르게 코딩하지만 아무것도 기억하지 못합니다. CQ는 계획하고, 검증하고, 세션을 넘어 기억하며, 당신을 닮아갑니다."
  actions:
    - theme: brand
      text: 시작하기
      link: /ko/guide/install
    - theme: alt
      text: GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: "\U0001F4CB"
    title: 코딩 전에 먼저 계획
    details: "다른 AI 도구들은 바로 코드로 뛰어듭니다. CQ는 먼저 '무엇을 만들 것인가'를 묻습니다 — 요구사항, 아키텍처, DoD. 그 다음 검증 가능한 태스크로 분해합니다."

  - icon: "\U0001F9E0"
    title: 당신과 함께 진화
    details: "세션 1: 당신의 선호도를 설명합니다. 세션 5: CQ는 이미 알고 있습니다. 선호도가 쌓이고 → 자동으로 규칙으로 승격 → AI 동작이 바뀝니다. 단순한 메모리가 아닙니다. 성장입니다."

  - icon: "\U0001F504"
    title: 어떤 AI와도 동작
    details: "Claude, Cursor, ChatGPT, Codex, Gemini. 지식은 도구를 넘어 흐릅니다 — ChatGPT에서 배운 것이 Claude에서도 활용됩니다. 하나의 두뇌, 여러 개의 손."

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

## 시작하기

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq
```

원하는 것을 말하세요. CQ가 자동으로 라우팅합니다: 소규모 수정은 직접 처리, 중간 규모는 `/c4-quick`, 큰 기능은 전체 `/pi` 계획 파이프라인을 거칩니다.

**[예제 보기 →](/ko/examples/first-task)**

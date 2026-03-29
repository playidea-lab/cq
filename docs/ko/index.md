---
layout: home

hero:
  name: "CQ"
  text: "모든 LLM을 연결하다"
  tagline: "Claude, ChatGPT, Cursor, Codex, Gemini — 하나의 두뇌. 한 AI에서 배운 것이 모든 AI에서 활용됩니다."
  actions:
    - theme: brand
      text: 설치하기
      link: /cq/ko/guide/install
    - theme: alt
      text: GitHub
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: "\U0001F4E6"
    title: 원라인 설치
    details: "curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh"

  - icon: "\U0001F9E0"
    title: 공유 두뇌
    details: "ChatGPT에서 설계 결정을 저장. Claude Code에서 불러오기. 지식은 도구에 흩어지지 않고 한 곳에."

  - icon: "\U0001F4C8"
    title: 당신과 함께 진화
    details: "세션이 쌓일수록 선호도를 학습. 5번째 세션부터 AI가 물어보지 않아도 당신의 방식대로."

  - icon: "\U0001F504"
    title: 계획 먼저, 코드는 그 다음
    details: "요구사항 → 아키텍처 → 태스크. Worker가 병렬로 구현하는 동안 커피 한 잔."

---

## 동작 원리

```
 당신이 말하면     CQ가 실행            결과
─────────────────────────────────────────────────────────────
 "이런 거 만들자"   /pi  → 브레인스토밍 + 조사     idea.md
 "만들어"          /c4-plan → 태스크 + 리뷰       계획
 ⏳               /c4-run  → 병렬 Worker          코드 + 테스트
 ☕               /c4-finish → 다듬기 + 검증      완성
```

## 크로스-AI 지식 흐름

```
ChatGPT  ──snapshot──►  CQ Brain  ◄──recall──  Claude Code
Cursor   ──snapshot──►            ◄──recall──  Codex
                    ▲
                    │
              mcp.pilab.kr
          (MCP — 설치 불필요)
```

**[예제 보기 →](/cq/ko/examples/first-task)**

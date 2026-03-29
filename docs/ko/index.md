---
layout: home

hero:
  name: "CQ"
  text: "진화하는 외장 두뇌"
  tagline: "당신을 배우고, 당신 대신 일하고, 세션마다 진화하는 AI."
  actions:
    - theme: alt
      text: "\U0001F4CB GitHub"
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: "\U0001F310"
    title: 분산 (Distribute)
    details: "Worker들이 독립적으로 병렬 실행. 자기 전에 셋업 — 아침에 테스트되고 리뷰된 코드가 기다립니다."

  - icon: "\U0001F517"
    title: 연결 (Connect)
    details: "Claude, ChatGPT, Cursor, Codex, Gemini — 하나의 두뇌. 한 AI에서 저장한 지식이 모든 AI에서 활용됩니다."

  - icon: "\U0001F9EC"
    title: 모방 (Mimic)
    details: "당신의 판단 기준, 습관, 선호도를 학습합니다. 당신처럼 결정하는 페르소나를 만듭니다."

  - icon: "\U0001F4C8"
    title: 진화 (Evolve)
    details: "나쁜 패턴은 도태. 좋은 패턴은 규칙으로 승격. 5번째 세션부터 AI가 물어보지 않아도 당신의 방식대로."

---

<div class="install-block">
  <code id="install-cmd">curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh</code>
  <button class="copy-btn" onclick="navigator.clipboard.writeText(document.getElementById('install-cmd').textContent).then(()=>{this.textContent='복사됨!';setTimeout(()=>this.textContent='복사',1500)})">복사</button>
</div>

## 동작 원리

```
    분산 → 연결 → 모방 → 진화
     ↑                      │
     └──────────────────────┘
```

```
 당신이 말하면     CQ가 실행            결과
─────────────────────────────────────────────────────────────
 "이런 거 만들자"   /pi  → 브레인스토밍 + 조사     idea.md
 "만들어"          /c4-plan → 태스크 + 리뷰       계획
 ⏳               /c4-run  → 병렬 Worker          코드 + 테스트
 ☕               /c4-finish → 다듬기 + 검증      완성
```

**[예제 보기 →](/cq/ko/examples/first-task)**

---
layout: home

hero:
  name: "CQ"
  text: "AI 연구자를 위한 코딩 팀"
  tagline: 실험도, 툴 개발도 — AI가 끝까지.
  actions:
    - theme: brand
      text: 설치하기
      link: /ko/guide/install
    - theme: alt
      text: GitHub 보기
      link: https://github.com/PlayIdea-Lab/cq

features:
  - icon: 🗣️
    title: 말하면 됩니다
    details: 무엇이 필요한지 말하세요. AI가 코드를 작성하고, 실행하고, 리뷰합니다. IDE 불필요.

  - icon: 🖥️
    title: 어디서든 실행
    details: 여러 서버에 AI 워커를 병렬 실행합니다. 실험과 개발이 동시에, 쉬지 않고 진행됩니다.

  - icon: ☀️
    title: 자고 일어나면 완료
    details: 자기 전에 시작하세요. 일어나면 코드가 작성되고, 테스트되고, 커밋되어 있습니다.

---

## 왜 CQ인가?

딥러닝 실험을 여러 서버에서 돌리면서 *동시에* 그 주변 툴도 개발해야 했습니다. 컨텍스트를 왔다갔다하지 않고.

실험은 분산 워커로 돌리고, 툴 개발은 `/pi`로 방향을 말하면 AI가 끝까지 처리합니다. 사람은 방향만 잡으면 됩니다.

이게 **Human Outside the Loop**입니다 — "AI가 도와주는 것"이 아니라 "AI가 하고, 내가 방향을 잡는 것".

CQ는 어떤 AI 코딩 어시스턴트와도 동작합니다: Claude Code, Gemini CLI, Codex, Cursor. 설정 한 번. 워크플로우 하나.

---

## 두 가지 사용 방법

### 🔬 실험 자동화

여러 GPU 또는 클라우드 서버에서 ML 실험 실행:

```
/pi  →  실험 설정을 설명하세요
      AI가 파이프라인을 설계하고, 학습 스크립트를 작성하고,
      여러 서버에 잡을 제출하고,
      완료되면 결과를 보고합니다.
```

비교 결과표를 보며 일어납니다.

### 🛠️ 툴 개발

연구 툴, CLI, 내부 시스템 개발:

```
/pi  →  툴이 무엇을 해야 하는지 설명하세요
      AI가 구조를 설계하고, 기능을 구현하고,
      테스트를 실행하고, 동작하는 코드를 커밋합니다.
```

보일러플레이트 없이. 디버깅 루프 없이. 말하고 출시합니다.

---

## 30초 시작

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
cq claude   # 또는: cq cursor / cq codex / cq gemini
```

필요한 것을 말하면 됩니다. 나머지는 AI가 합니다.
---

---
name: pi
description: |
  Play Idea — 아이디어 발산·수렴 모드. c4-plan 이전 단계.
  온라인 조사, 지식 베이스 활용, 무한 토론을 통해 막연한 아이디어를
  명확한 개념으로 결정(結晶)시킨다. 토론이 충분히 무르익으면
  idea.md를 생성하고 /c4-plan을 직접 호출한다.
  Triggers: "/pi", "play idea", "아이디어 탐색", "ideation",
  "기획 토론", "뭔가 만들고 싶은데", "이런 거 어때", "브레인스토밍".
---

# /pi — Play Idea

> "아직 뭘 만들지 모를 때 쓰는 모드. 계획 전의 계획."

---

## 모드 선언 (진입 시 즉시)

```
✗ 코드 작성 금지  ✗ 기술 스택 선택 금지  ✗ 태스크 분해 금지
✓ 조사, 질문, 토론, 정리만
```

알림: `c4_notify(message='Ideation 시작', event='pi.start')`

---

## 철학

```
c4-interview  =  이미 무엇을 만들지 알 때, 요구사항을 파낸다
c4-plan       =  무엇을 만들지 확정됐을 때, 구현 계획을 짠다
/pi           =  아직 뭘 만들지 모를 때, 아이디어를 결정(結晶)시킨다
```

토론을 끊지 않는다. 검색은 아끼지 않는다. 동의만 하지 않는다.

---

## 시작: 씨앗 이해

씨앗(args)이 있으면 그것을, 없으면 **"어떤 아이디어를 탐구해볼까요?"**

씨앗을 받으면 즉시 병렬 실행:

```python
c4_knowledge_search(query="{seed_idea}")
Glob(".c4/ideas/*.md")  # 기존 idea.md 스캔 — 이미 탐구한 아이디어 확인
WebSearch("{seed_idea} 2026")
WebSearch("{seed_idea} alternatives")
WebSearch("{seed_idea} problems OR pain points")
```

기존 idea.md가 발견되면: "이전에 **{title}** 아이디어가 있었습니다. 연장/확장/별개?" 확인.

첫 반응: **Landscape** (기존) + **핵심 긴장** (왜 흥미로운지) + **첫 날카로운 질문**

---

## 토론 엔진: 4개 모드

| 모드 | 조건 | 목적 |
|------|------|------|
| **발산** | 아이디어가 좁거나 단순 | 시야를 넓힌다 |
| **수렴** | 방향이 너무 많아 산만 | 핵심 하나 추출 |
| **심화** | 방향이 잡히기 시작 | "왜?", "누가?", "어떤 상황?" |
| **반론** | 유저 확신이 너무 강함 | 약점 미리 발견 |

## 탐구 렌즈 (필요한 것만)

WHY, WHO, WHEN, ANALOGY, INVERT, SCALE, FAILURE

## 리서치 트리거

모르면 즉시 검색 — existing tools, case studies, market size, feasibility.

---

## 수렴 감지 → 출구

### 감지 조건 (3개 이상)

- 핵심 문제가 한 문장으로 표현 가능
- 타깃 사용자가 구체적
- "왜 지금, 왜 우리"가 설명됨
- 기존 대안과 차별점 명확
- 가장 큰 리스크 인지
- 유저가 "됐다"고 말함

---

## 결정화 프로세스

### Step 1. EARS 요구사항 자동 초안

대화 내용에서 질문 없이 EARS 패턴으로 요구사항 자동 도출.
사용자 수정 반영 후 승인되면 Step 2.

### Step 2. idea.md 생성

`.c4/ideas/{slug}.md` 저장. 템플릿: `references/idea-template.md`

```python
Write(path=idea_path, content=idea_content)
# 에디터 열기: code > open > 인라인 표시 순
```

### Step 3. 지식 베이스 저장

```python
c4_knowledge_record(title="{이름} — /pi 세션", content="{요약}", domain="ideation")
```

### Step 4. 진행 방식 선택

```python
AskUserQuestion(questions=[{
    "question": "다음 단계",
    "options": [
        {"label": "자동 구현", "description": "plan → run → finish 전체 자동"},
        {"label": "계획만", "description": "c4-plan으로 태스크만 생성"}
    ]
}])
```

- **자동 구현** → `Skill("c4-plan", args="--from-pi {slug} --auto-run")`
- **계획만** → `Skill("c4-plan", args="--from-pi {slug}")`

---

## 안티패턴

```
❌ "어떤 기술 스택을 쓸까요?"   → 구현은 c4-plan에서
❌ "태스크로 분해하면..."        → 아직 아님
❌ 유저 말에 무조건 동의         → 반론이 아이디어를 단단하게 함
❌ 검색 없이 "잘 모르겠지만..."  → 모르면 찾는다
❌ 조건 미달인데 수렴 강요       → 유저가 준비됐을 때
```

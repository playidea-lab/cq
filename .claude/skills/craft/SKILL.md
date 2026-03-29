---
name: craft
essential: true
description: |
  대화형으로 나만의 skill, agent, rule, CLAUDE.md를 생성합니다.
  사용자 의도를 파악해 타입을 자동 판단하고, 5턴 이내에 파일을 생성합니다.
  Triggers: "craft", "/craft", "스킬 만들어", "에이전트 만들어", "룰 추가",
  "커스텀 도구", "새 스킬", "skill 만들어", "규칙 추가해",
  "CLAUDE.md 만들어", "프로젝트 설정", "에이전트 지침".
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

# /craft — 나만의 도구 제작

> "워크플로우면 skill, 전문가면 agent, 제약이면 rule, 프로젝트 지침이면 CLAUDE.md."

---

## 진입

args가 있으면 그걸로 시작. 없으면:

```
어떤 도구를 만들고 싶으세요?
예: "코드 리뷰할 때 보안 체크리스트 먼저 확인해줘"
    "데이터 분석 전문가처럼 행동해줘"
    "항상 커밋 전에 go vet 실행해"
```

---

## 타입 자동 판단 (사용자에게 묻지 않음)

| 사용자 의도 | 타입 | 저장 경로 |
|------------|------|----------|
| "X할 때 Y 순서로 해", "X 하면 Y 해줘" | **skill** | `~/.claude/skills/{name}/SKILL.md` |
| "X 전문가처럼", "X처럼 행동해", "페르소나" | **agent** | `~/.claude/agents/{name}.md` |
| "항상 X 해", "절대 X 금지", "매번 X" | **rule** | `~/.claude/rules/{name}.md` |
| "CLAUDE.md 만들어", "프로젝트 설정", "빌드 명령 설정" | **claude-md** | `./CLAUDE.md` |

판단 기준:
- **여러 단계 / 워크플로우** → skill
- **성격 / 전문성 / 역할** → agent
- **항상 적용 / 금지 / 제약** → rule
- **프로젝트 전체 지침 / 빌드·테스트 명령 / 코드 스타일** → claude-md

---

## 프리셋 확인

기존 파일 스캔:

```python
Glob("~/.claude/skills/*/SKILL.md")   # 기존 skill 목록
Glob("~/.claude/agents/*.md")         # 기존 agent 목록
Glob("~/.claude/rules/*.md")          # 기존 rule 목록
```

비슷한 게 있으면: "기존 `{name}` {타입}과 비슷합니다. 그걸 수정할까요, 새로 만들까요?"

---

## 대화 흐름 (최대 5턴)

### Turn 1: 의도 파악 + 타입 선언

사용자 의도를 한 문장으로 요약하고 타입 판단 결과를 투명하게 보여준다:

```
[판단] 이건 **skill**입니다 — "X할 때 Y 단계로 실행"하는 워크플로우이기 때문.

확인할게요:
1. 이름: `{suggested-name}` (영문 kebab-case)
2. 트리거: "{trigger1}", "{trigger2}"
3. 핵심 동작: {한 줄 요약}
```

### Turn 2: 핵심 내용 확정

이름/트리거/동작이 맞으면 생성. 수정 요청이 있으면 반영 후 생성.

짧은 확인: "이렇게 만들까요? (수정할 내용 있으면 말씀해주세요)"

### Turn 3~5: 생성 + 저장

확정되면 즉시 파일 생성.

---

## 생성 템플릿

### Skill 템플릿

```markdown
---
name: {name}
essential: true
description: |
  {한 줄 설명}. {상세 설명}.
  Triggers: "{trigger1}", "{trigger2}".
allowed-tools: Read, Write, Edit, Glob, Grep, Bash
---

# /{name}

> "{핵심 철학 한 줄}"

---

## 진입

{args 처리 방법}

---

## 단계

### 1단계: {단계명}

{설명}

### 2단계: {단계명}

{설명}

---

## 안티패턴

- {하지 말아야 할 것 1}
- {하지 말아야 할 것 2}
```

### Agent 템플릿

```markdown
---
name: {name}
essential: true
description: {한 줄 설명}. 트리거: "{trigger1}", "{trigger2}".
---

# {Name} Agent

## 역할

{이 에이전트가 누구인지, 어떤 전문성을 갖는지}

## 행동 원칙

- {원칙 1}
- {원칙 2}
- {원칙 3}

## 전문 영역

{이 에이전트가 잘하는 것, 특화된 것}

## 말투/스타일

{어떻게 소통하는지}
```

### Rule 템플릿

```markdown
# {Name} Rules

> {이 규칙이 왜 필요한지 한 줄}

## 적용 범위

{언제, 어디서 이 규칙이 적용되는지}

## 규칙

- {규칙 1}
- {규칙 2}
- {규칙 3}

## 예외

- {예외 사항 (있는 경우)}
```

### CLAUDE.md 템플릿

프로젝트 루트에 CLAUDE.md를 생성. 프로젝트 구조를 스캔하여 자동으로 내용 채움:

```python
# 프로젝트 자동 감지
Glob("go.mod")           # Go 프로젝트
Glob("pyproject.toml")   # Python 프로젝트
Glob("package.json")     # JS/TS 프로젝트
Glob("Cargo.toml")       # Rust 프로젝트
Bash("ls *.go *.py *.ts 2>/dev/null | head -5")  # 주요 언어 확인
```

감지 결과에 맞는 프리셋(go-project, python-project, web-frontend, general)을 기반으로
빌드/테스트 명령, 코드 스타일, 프로젝트 구조를 자동으로 채운다.

```markdown
# Project Instructions

## Build & Test

{프로젝트에서 감지된 빌드/테스트 명령}

## Code Style

{감지된 언어에 맞는 컨벤션}

## Project Structure

{실제 디렉토리 구조 스캔 결과}

## Key Modules

{주요 파일/패키지 설명 — 사용자에게 확인}
```

---

## 저장 + 완료 출력

파일 저장 후:

```
생성 완료: ~/.claude/{타입}/{name}/SKILL.md (또는 .md)

사용법:
  skill     → /{name} 또는 "/{name}" 입력
  agent     → 대화 자동 감지 (트리거 키워드 사용)
  rule      → 모든 대화에 자동 적용
  claude-md → 프로젝트 루트에 CLAUDE.md 생성됨
```

---

## 안티패턴

```
❌ "이건 skill인가요 agent인가요?" — 자동 판단, 질문 금지
❌ 5턴 초과 — 2턴에 확정 못 하면 제안한 대로 생성
❌ 템플릿 그대로 출력 — 반드시 사용자 의도에 맞게 채운다
❌ 저장 경로 오판 — skill은 디렉토리/SKILL.md, agent/rule은 단일 .md
```

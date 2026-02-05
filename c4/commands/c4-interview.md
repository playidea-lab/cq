---
argument-hint: [feature-name] Feature to interview about
description: Deep exploratory interview to discover hidden requirements
allowed-tools: AskUserQuestion, Write, Read, Glob
---

# C4 Interview - Deep Exploratory Requirements Interview

당신은 **시니어 프로덕트 매니저이자 시스템 아키텍트**입니다.
사용자의 아이디어에서 **숨겨진 요구사항**을 발견하는 것이 목표입니다.

> Danny Postma's Rule: "Ask not obvious questions, very in-depth, and continually until I have complete clarity."

---

## Core Principles

### 1. No Obvious Questions

```
❌ 금지 질문:
- "어떤 언어로 구현할까요?" (뻔함)
- "데이터베이스는 뭘 쓸까요?" (기술 선택은 나중에)
- "언제까지 완료해야 하나요?" (PM 질문)

✅ 좋은 질문:
- "5,000명이 동시에 이 기능을 쓰면 어떻게 되어야 하나요?"
- "이 기능이 실패하면 사용자는 뭘 보게 되나요?"
- "가장 흔한 사용 시나리오와 예외 시나리오의 비율은요?"
```

### 2. Very In-Depth

단순 기능 나열이 아닌, **왜(Why)**와 **만약(What If)**를 파고듭니다.

```
사용자: "로그인 기능이 필요해요"
    ↓ 깊이 파고들기
Q1: "로그인 실패 시 계정 잠금은 어떻게 하죠? 3회? 5회? 점진적 지연?"
Q2: "소셜 로그인만으로 가입한 사용자가 비밀번호 리셋을 요청하면?"
Q3: "세션 만료 중 작성 중인 글이 있으면 어떻게 복구하나요?"
Q4: "다중 디바이스 로그인은 허용? 마지막 세션만? 알림 표시?"
```

### 3. Continually Until Complete

**"충분하다"는 확신이 들 때까지 멈추지 않습니다.**

```python
confident_enough = False
clarity_level = 0

while not confident_enough:
    # 질문
    answers = ask_deep_questions()

    # 분석
    new_insights = analyze_answers(answers)
    clarity_level += len(new_insights)

    # 완료 조건 체크
    confident_enough = all([
        core_features_clear,
        edge_cases_discovered >= 3,
        failure_scenarios_defined,
        tradeoffs_decided,
        hidden_assumption_found
    ])
```

---

## Interview Areas

### Area 1: Core Function Deep Dive

```python
AskUserQuestion(questions=[
    {
        "question": "이 기능의 **가장 중요한 성공 지표**는 무엇인가요?",
        "header": "Success Metrics",
        "options": [
            {"label": "속도 (응답 시간 < Xms)", "description": "성능 중심"},
            {"label": "정확도 (오류율 < X%)", "description": "품질 중심"},
            {"label": "처리량 (동시 요청 처리)", "description": "확장성 중심"},
            {"label": "사용성 (클릭 수 최소화)", "description": "UX 중심"},
            {"label": "기타 (직접 입력)", "description": ""}
        ],
        "multiSelect": True
    }
])
```

### Area 2: Edge Cases Discovery

```python
# 사용자가 생각 못한 엣지 케이스 발굴
AskUserQuestion(questions=[
    {
        "question": "다음 상황에서 시스템은 어떻게 동작해야 하나요?",
        "header": "Edge Cases",
        "options": [
            {"label": "네트워크 끊김 중 작업 완료 시", "description": "오프라인 처리"},
            {"label": "동시에 같은 데이터 수정 시", "description": "충돌 해결"},
            {"label": "데이터 양이 예상의 100배일 때", "description": "확장성"},
            {"label": "관리자가 데이터 롤백 요청 시", "description": "복구"},
            {"label": "고려한 적 없음 (지금 정의)", "description": ""}
        ],
        "multiSelect": True
    }
])
```

### Area 3: Failure Scenarios

```python
# 실패 시나리오는 대부분 간과됨
AskUserQuestion(questions=[
    {
        "question": "이 기능이 **실패**하면 사용자에게 어떤 경험을 제공해야 하나요?",
        "header": "Failure UX",
        "options": [
            {"label": "자동 재시도 후 알림", "description": "시스템이 해결 시도"},
            {"label": "즉시 에러 메시지", "description": "투명한 피드백"},
            {"label": "대체 경로 제시", "description": "fallback 옵션"},
            {"label": "조용히 로깅만", "description": "사용자 인지 불필요"},
            {"label": "직접 설명", "description": ""}
        ],
        "multiSelect": False
    }
])
```

### Area 4: Tradeoffs

```python
# 명시적 트레이드오프 결정
AskUserQuestion(questions=[
    {
        "question": "다음 중 **반드시 양보할 수 없는 것**은?",
        "header": "Non-negotiables",
        "options": [
            {"label": "빠른 출시", "description": "MVP 빨리 → 기능 축소 가능"},
            {"label": "완벽한 기능", "description": "모든 케이스 → 시간 더 필요"},
            {"label": "최고 성능", "description": "최적화 → 복잡도 증가"},
            {"label": "단순한 코드", "description": "유지보수 → 기능 제약"}
        ],
        "multiSelect": False
    },
    {
        "question": "그럼 위 선택을 위해 **양보 가능한 것**은?",
        "header": "Tradeoffs",
        "options": [
            {"label": "출시 일정", "description": "더 늦춰도 됨"},
            {"label": "기능 범위", "description": "일부 제외 가능"},
            {"label": "성능", "description": "충분히 빠르면 됨"},
            {"label": "코드 품질", "description": "나중에 리팩토링"}
        ],
        "multiSelect": True
    }
])
```

### Area 5: Hidden Assumptions

```python
# 사용자가 당연시하지만 중요한 것들
questions_to_reveal_assumptions = [
    "이 기능의 사용자는 **기술적으로 얼마나 숙련**되어 있나요?",
    "데이터는 **얼마나 민감**한가요? (개인정보, 결제정보 등)",
    "이 기능이 **다른 시스템과 연동**되어야 하나요?",
    "**모바일**에서도 사용되나요?",
    "**다국어/다중 시간대** 지원이 필요한가요?"
]
```

---

## Interview Flow

### Step 1: Context Gathering

```python
# 기존 스펙/문서 확인
existing_specs = mcp__c4__c4_list_specs()
existing_docs = Glob("docs/**/*.md")

# 프로젝트 컨텍스트 파악
readme = Read("README.md")
```

### Step 2: Initial Deep Questions

**첫 질문부터 깊이 있게:**

```python
AskUserQuestion(questions=[
    {
        "question": f"'{feature_name}'을 만들려는 **진짜 이유**가 뭔가요?\n(기능이 아니라 해결하려는 문제)",
        "header": "Problem Statement",
        "options": [],  # 오픈 질문
        "multiSelect": False
    }
])
```

### Step 3: Follow-up Based on Answers

이전 답변에서 **후속 질문 도출**:

```python
# 답변 분석
if "성능" in previous_answer:
    follow_up = "구체적인 성능 목표가 있나요? (예: 100ms 이내, 1000 TPS)"
elif "보안" in previous_answer:
    follow_up = "보안 감사나 컴플라이언스 요구사항이 있나요? (SOC2, GDPR 등)"
elif "확장" in previous_answer:
    follow_up = "예상 사용자 수의 최대치와 성장 속도는?"
```

### Step 4: Completion Check

```python
completion_criteria = {
    "core_features": len(identified_features) >= 3,
    "edge_cases": len(edge_cases) >= 3,
    "performance_requirements": performance_defined,
    "security_considerations": security_reviewed,
    "hidden_requirement_found": hidden_reqs >= 1,
    "tradeoffs_decided": tradeoffs_clear
}

if all(completion_criteria.values()):
    proceed_to_spec_generation()
else:
    missing = [k for k, v in completion_criteria.items() if not v]
    ask_more_questions_about(missing)
```

---

## Output: Interview Spec

인터뷰 완료 후 스펙 파일 생성:

**저장 위치:** `.c4/specs/{feature}/interview.md`

### Spec File Format

```markdown
# {Feature} Interview Spec

## Overview
{1-2 sentence summary of the feature}

## Problem Statement
{The real problem this feature solves}

## Core Requirements
1. {Requirement 1}
   - Success Criteria: {measurable criteria}
2. {Requirement 2}
   - Success Criteria: {measurable criteria}
...

## Discovered Edge Cases
| Case | Expected Behavior | Priority |
|------|-------------------|----------|
| {case1} | {behavior} | Must-have |
| {case2} | {behavior} | Nice-to-have |

## Failure Scenarios
| Failure | User Experience | System Behavior |
|---------|-----------------|-----------------|
| {failure1} | {UX} | {system action} |

## Tradeoff Decisions
| Decision | Chosen | Sacrificed | Rationale |
|----------|--------|------------|-----------|
| {decision1} | {choice} | {tradeoff} | {why} |

## Hidden Requirements (Discovered in Interview)
- {requirement that user hadn't considered}
- {implicit assumption made explicit}

## Performance Requirements
- Response time: {target}
- Throughput: {target}
- Concurrent users: {target}

## Security Considerations
- {security requirement 1}
- {security requirement 2}

## Next Steps
After this interview spec is saved:
1. Run `/c4-plan {feature}` to proceed with design phase
2. Or run `/c4-interview {feature}` again to refine requirements

---
*Generated by C4 Interview on {date}*
*Clarity Level: {percentage}%*
```

---

## Integration with C4 Workflow

### Discovery Phase Integration

이 인터뷰 스킬은 `/c4-plan`의 **Discovery 단계**와 연동됩니다:

```
/c4-interview {feature}
       ↓
.c4/specs/{feature}/interview.md 생성
       ↓
/c4-plan {feature}
       ↓
interview.md를 기반으로 EARS 요구사항 변환
       ↓
Design → Tasks 생성
```

### EARS Pattern Conversion

인터뷰 결과는 EARS 패턴으로 자동 변환 가능:

```
Interview: "로그인 5회 실패 시 계정 잠금"
    ↓
EARS: "If login fails 5 times, the system shall lock the account for 30 minutes"
    (Unwanted pattern)
```

---

## Usage Examples

### Basic Usage

```
/c4-interview user-authentication
```

### With Specific Focus

```
/c4-interview payment-system
> Focus on: security, edge cases, failure handling
```

### Re-interview for Refinement

```
/c4-interview user-authentication
> (기존 interview.md가 있으면 그 내용 기반으로 추가 질문)
```

---

## Anti-Patterns (Avoid These)

### 1. Checklist-Style Questions

```
❌ "프론트엔드는 React? Vue? Angular?"
❌ "데이터베이스는 Postgres? MySQL?"
❌ "배포는 AWS? GCP?"

→ 이런 질문은 Discovery 완료 후 Design 단계에서
```

### 2. Yes/No Questions

```
❌ "로그인 기능 필요하죠?"
❌ "테스트 작성할까요?"

✅ "로그인이 실패했을 때 사용자가 겪는 최악의 시나리오는?"
✅ "이 기능이 버그 없이 동작한다는 걸 어떻게 확신할 수 있을까요?"
```

### 3. Premature Solutioning

```
❌ "JWT로 인증하면 될 것 같은데, 동의하시나요?"

✅ "세션 관리에서 가장 중요한 건 뭔가요? 보안? 편의성? 확장성?"
```

---

## Quick Start

```
/c4-interview $ARGUMENTS
```

시작하면:
1. 기존 컨텍스트(README, specs) 확인
2. 첫 번째 깊은 질문으로 인터뷰 시작
3. 답변 기반 후속 질문 진행
4. 완료 조건 충족 시 interview.md 생성
5. `/c4-plan`으로 다음 단계 안내

---

<instructions>
Feature to interview: $ARGUMENTS

Start the deep exploratory interview now. Remember:
- No obvious questions
- Very in-depth
- Continue until complete clarity
</instructions>

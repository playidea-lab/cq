# Phase 2.65: Conflict Gate (소프트 게이트)

> Entry: c4_design_complete() 직후, Phase 2.7 Lighthouse 전.
> 목적: 진행 중인 다른 워커/기존 스펙/지식과의 충돌을 감지하여 검토를 요청한다.
> 충돌이 없으면 조용히 통과. 충돌이 있어도 사용자가 항상 무시하고 진행할 수 있다 (소프트 게이트).

## 2.65.1 충돌 스캔

병렬로 3가지 소스를 스캔한다:

```python
# 1. 파일 충돌 (HIGH) — 활성 워커와 동일 파일 수정 여부
# 실패 시: 해당 소스 건너뜀 (충돌 없음으로 처리) — UX 방해 금지
worktrees = c4_worktree_status()
# c4/w-* 브랜치별 수정 중인 파일 목록 추출
# → 현재 plan scope 파일들과 교집합 확인

# 2. 개념 겹침 (MEDIUM) — 기존 스펙/디자인과 유사한 주제
# 실패 시: 해당 소스 건너뜀 (충돌 없음으로 처리)
specs   = c4_list_specs()
designs = c4_list_designs()
# feature명, 설명, 도메인 키워드로 현재 계획과 유사한 항목 LLM 판단
# → 이름/설명이 현재 feature와 겹치는 것 플래그
# → 최근 수정(14일 이내) spec/design만 비교 대상으로 한정

# 3. 지식 참고 (LOW) — 동일 도메인 최근 기록
# 실패 시: 해당 소스 건너뜀 (충돌 없음으로 처리)
recent = c4_knowledge_search(query=f"{current_feature} {detected_domain}", limit=3)
# 유사 결과 있으면 참고용으로 제시 (blocking 아님)
```

## 2.65.2 결과 처리

**충돌 없음**:
```
✅ 충돌 없음 — 계속 진행합니다
```
→ Phase 2.7로 조용히 진행.

**충돌 감지 시**: ConflictReport 출력 후 AskUserQuestion:

```
⚠️  충돌 감지 — 태스크 생성 전 검토 필요
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[파일 충돌] HIGH
  {file}
  → 워커 {branch} ({task_id}) 수정 중

[개념 겹침] MEDIUM
  기존 Spec/Design: {name}
  → "{현재 feature}"와 유사한 문제를 다루고 있음

[지식 참고] LOW
  Knowledge: "{title}" ({date})
  → 동일 도메인 최근 기록 — 참고할 것

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

```python
AskUserQuestion(questions=[{
    "question": "충돌이 감지됐습니다. 어떻게 진행할까요?",
    "header": "Conflict Review",
    "options": [
        {"label": "검토 완료, 계속 진행", "description": "충돌 내용을 확인했으며 이 계획과 병행 가능하다고 판단"},
        {"label": "충돌 무시하고 계속", "description": "충돌이 있지만 현재 계획을 그대로 진행"},
        {"label": "계획 재검토", "description": "Phase 2.6 Design으로 돌아가 범위/접근 방식 수정"}
    ],
    "multiSelect": False
}])
```

- "검토 완료" → Phase 2.7
- "충돌 무시하고 계속" → Phase 2.7 + `c4_knowledge_record`로 무시 결정 이력 기록
- "계획 재검토" → Phase 2.6으로 돌아감

## 규칙

```
✅ 충돌이 없으면 절대 중단하지 않는다 — UX 방해 금지
✅ 사용자는 항상 충돌을 무시하고 진행할 수 있다
✅ 파일 충돌 > 개념 겹침 > 지식 참고 순으로 심각도 분류
❌ 하드 블로킹 금지 — 소프트 게이트만
```

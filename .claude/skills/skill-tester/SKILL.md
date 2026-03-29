---
name: skill-tester
essential: true
description: |
  스킬의 품질을 테스트하고 평가합니다. eval 케이스 생성, 실행, 채점, 벤치마크.
  Triggers: "스킬 테스트", "skill test", "eval", "스킬 평가", "skill eval",
  "스킬 품질", "벤치마크", "skill benchmark", "/skill-tester"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
  - Agent
---

# /skill-tester — 스킬 품질 테스트 & 평가

> Anthropic skill-creator eval 파이프라인의 CQ 개량 버전.
> 4-agent 파이프라인: Executor → Grader → Analyzer → Reporter.

---

## 모드 선택

args 파싱:

```
/skill-tester <skill-path>                 # 전체 파이프라인
/skill-tester <skill-path> --eval-only     # eval 생성만
/skill-tester <skill-path> --grade-only    # 기존 결과 채점만
/skill-tester <skill-path> --compare A B   # A/B 비교
```

args 없으면:

```
테스트할 스킬 경로를 입력하세요.
예: .claude/skills/craft
    .claude/skills/code-review
```

---

## Phase 0: 스킬 분석

```python
skill_path = args[0]  # e.g., ".claude/skills/craft"
skill_md = Read(f"{skill_path}/SKILL.md")
extra_files = Glob(f"{skill_path}/**/*")

# 프론트매터 파싱
frontmatter = parse_yaml(skill_md)
skill_name = frontmatter.get("name", basename(skill_path))
description = frontmatter.get("description", "")
allowed_tools = frontmatter.get("allowed-tools", [])
has_fork = frontmatter.get("context") == "fork"
```

### 정적 분석 (즉시)

| 체크 | 기준 | 심각도 |
|------|------|--------|
| SKILL.md 존재 | 필수 | CRITICAL |
| description 구체성 | 트리거 문구 2개+ | HIGH |
| 줄 수 | ≤500줄 | MEDIUM |
| allowed-tools 명시 | 빈 값이 아닌지 | MEDIUM |
| references/ 참조 | SKILL.md에서 참조 확인 | LOW |
| examples/ 존재 | 있으면 가산점 | LOW |

결과를 `스킬 건강도 리포트`로 출력.

---

## Phase 1: Eval 케이스 생성

### 자동 생성 전략

description의 트리거 문구 → eval 프롬프트로 변환:

```
description: "코드 리뷰할 때 보안 체크리스트 확인"
  → eval: "이 Python 코드를 리뷰해줘 (SQL injection 취약점 포함)"
  → assertion: "SQL injection 또는 parameterized query 언급"
```

### eval 케이스 구조

```json
{
  "skill_name": "code-review",
  "evals": [
    {
      "id": 0,
      "name": "sql-injection-detection",
      "prompt": "이 코드를 리뷰해줘:\nimport sqlite3\nconn.execute(f'SELECT * FROM users WHERE name={name}')",
      "assertions": [
        {"text": "SQL injection 위험을 지적한다", "type": "semantic"},
        {"text": "parameterized query 대안을 제시한다", "type": "semantic"}
      ],
      "input_files": []
    }
  ]
}
```

### 타입별 assertion 전략

| 스킬 타입 | assertion 예시 |
|-----------|---------------|
| 코드 리뷰 | 특정 패턴 탐지, 대안 제시 |
| 워크플로우 | 단계별 실행 순서, 결과물 생성 |
| 생성형 | 포맷 준수, 필수 섹션 포함 |
| 조회형 | 정확한 데이터 반환, 에러 핸들링 |

eval 케이스 3~5개를 자동 생성 후 사용자 확인.

```python
evals_path = f"{skill_path}/evals/evals.json"
Write(evals_path, generated_evals)
print(f"📋 {len(evals)} eval 케이스 생성 → {evals_path}")
print("수정 후 계속하려면 Enter, 추가하려면 내용 입력:")
```

---

## Phase 2: 실행 (Executor)

각 eval 케이스를 **with-skill / without-skill** 쌍으로 실행.

```python
workspace = f"{skill_path}/{skill_name}-workspace/iteration-1"

for eval_case in evals:
    eval_dir = f"{workspace}/eval-{eval_case['id']}-{eval_case['name']}"

    # with-skill: 스킬 활성화 상태에서 실행
    Agent(
        name=f"executor-with-{eval_case['id']}",
        prompt=f"""
Execute this task with the skill at {skill_path}:
Task: {eval_case['prompt']}
Input files: {eval_case.get('input_files', 'none')}
Save all outputs to: {eval_dir}/with_skill/outputs/
""",
        run_in_background=True
    )

    # without-skill: 스킬 없이 동일 프롬프트 실행
    Agent(
        name=f"executor-without-{eval_case['id']}",
        prompt=f"""
Execute this task WITHOUT any skill:
Task: {eval_case['prompt']}
Save all outputs to: {eval_dir}/without_skill/outputs/
""",
        run_in_background=True
    )
```

실행 중 assertion 초안을 다듬는다 (idle 방지).

---

## Phase 3: 채점 (Grader)

각 실행 결과를 assertion 기준으로 채점.

```python
for eval_case in evals:
    for variant in ["with_skill", "without_skill"]:
        outputs = Read(f"{eval_dir}/{variant}/outputs/")

        grading = []
        for assertion in eval_case["assertions"]:
            if assertion["type"] == "regex":
                passed = regex_match(assertion["pattern"], outputs)
                evidence = f"Pattern {'found' if passed else 'not found'}"
            elif assertion["type"] == "file_exists":
                passed = file_exists(assertion["path"])
                evidence = f"File {'exists' if passed else 'missing'}"
            else:  # semantic
                # LLM 판단
                passed, evidence = llm_grade(assertion["text"], outputs)

            grading.append({
                "text": assertion["text"],
                "passed": passed,
                "evidence": evidence
            })

        Write(f"{eval_dir}/{variant}/grading.json", grading)
```

---

## Phase 4: 분석 & 리포트 (Analyzer + Reporter)

### 집계

```python
results = aggregate(workspace)
# pass_rate: with_skill vs without_skill
# timing: 실행 시간 비교
# token_usage: 토큰 사용량 비교
```

### 리포트 출력

```markdown
## 스킬 테스트 리포트: {skill_name}

### 건강도 (정적 분석)
| 항목 | 결과 |
|------|------|
| SKILL.md | ✅ |
| description 구체성 | ⚠️ 트리거 1개만 |
| 줄 수 (234/500) | ✅ |

### Eval 결과
| Eval | With Skill | Without Skill | Delta |
|------|-----------|--------------|-------|
| sql-injection | 2/2 (100%) | 0/2 (0%) | +100% |
| xss-detection | 1/2 (50%)  | 0/2 (0%) | +50%  |

### 종합
- Pass Rate: 75% (with) vs 0% (without)
- 스킬 효과: 유의미 (+75%)
- 개선 제안: xss-detection에서 대안 미제시 → SKILL.md에 XSS 패턴 추가 권장
```

### 지식 기록

```python
c4_knowledge_record(
    title=f"스킬 테스트: {skill_name} — pass rate {pass_rate}%",
    content=report_summary,
    tags=["skill-eval", skill_name]
)
```

---

## 추가 리소스

### Reference Files
- **`references/assertion-patterns.md`** — assertion 타입별 작성 패턴
- **`references/eval-schemas.md`** — JSON 스키마 상세

### Scripts
- **`scripts/aggregate.py`** — 벤치마크 집계 스크립트

---

## 안티패턴

```
❌ assertion 없이 "잘 되는 것 같다"로 판단
❌ with-skill만 실행 (baseline 없으면 효과 측정 불가)
❌ 주관적 스킬에 100% pass rate 기대
❌ eval 1개로 충분하다고 판단
```

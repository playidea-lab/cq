# Assertion 패턴 가이드

## 타입별 assertion

### semantic (LLM 판단)
가장 범용적. 출력의 의미를 LLM이 판단.
```json
{"text": "SQL injection 위험을 지적한다", "type": "semantic"}
```

### regex (패턴 매칭)
출력에서 특정 패턴이 존재하는지 확인.
```json
{"text": "에러 코드 포함", "type": "regex", "pattern": "E\\d{4}"}
```

### file_exists (파일 생성 확인)
스킬이 특정 파일을 생성하는지 확인.
```json
{"text": "CHANGELOG.md 생성됨", "type": "file_exists", "path": "outputs/CHANGELOG.md"}
```

### contains (문자열 포함)
특정 문자열이 출력에 포함되는지.
```json
{"text": "parameterized query 언급", "type": "contains", "value": "parameterized"}
```

### line_count (줄 수 범위)
출력 줄 수가 범위 내인지.
```json
{"text": "출력이 10-50줄 사이", "type": "line_count", "min": 10, "max": 50}
```

## 스킬 타입별 추천 assertion

| 스킬 타입 | 추천 assertion |
|-----------|---------------|
| 코드 리뷰 | semantic(결함 탐지) + contains(대안 키워드) |
| 생성형 (PR, 릴리즈노트) | file_exists + line_count + contains(필수 섹션) |
| 워크플로우 (deploy, hotfix) | semantic(단계 순서) + file_exists(결과물) |
| 조회형 (status, search) | contains(정확한 값) + regex(포맷) |
| 대화형 (interview, craft) | semantic(질문 품질) — pass rate 100% 기대하지 말 것 |

## 좋은 assertion vs 나쁜 assertion

```
❌ "출력이 좋다" → 주관적, 채점 불가
✅ "보안 취약점을 최소 1개 이상 지적한다" → 검증 가능

❌ "올바른 형식이다" → 모호
✅ "## Summary 헤더가 포함되어 있다" → contains로 검증 가능

❌ "빠르게 실행된다" → 상대적
✅ "실행 시간이 30초 이내이다" → timing.json으로 검증
```

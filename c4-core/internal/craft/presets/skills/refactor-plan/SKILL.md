---
name: refactor-plan
description: |
  리팩토링 계획 수립. 영향 분석→테스트 확보→단계별 실행→검증 순서로 안전하게 진행.
  트리거: "리팩토링", "refactor", "코드 개선 계획", "기술 부채"
allowed-tools: Read, Write, Glob, Grep, Bash
---
# Refactor Plan

리팩토링을 안전하게 계획하고 실행합니다.

## 실행 순서

### Step 1: 대상 파악

리팩토링 대상 코드를 읽고 분석:

```bash
# 파일 크기 순 (God File 탐지)
find . -name "*.go" -o -name "*.py" | xargs wc -l | sort -rn | head -20

# 복잡도가 높은 함수 탐지
grep -rn "func \|def " --include="*.go" --include="*.py" .
```

문제 유형 분류:
- [ ] God Object/File (너무 많은 책임)
- [ ] 중복 코드 (DRY 위반)
- [ ] 깊은 중첩 (복잡도 높음)
- [ ] 긴 함수 (50줄 초과)
- [ ] 불명확한 이름
- [ ] 강한 결합 (테스트 어려움)

### Step 2: 영향 범위 분석

```bash
# 대상 함수/클래스 사용처 탐색
grep -rn "<함수명>\|<클래스명>" --include="*.go" .

# 의존 관계 파악
grep -rn "import\|require" <target-file>
```

- [ ] 몇 개 파일이 영향받는가?
- [ ] 외부 API/인터페이스가 변경되는가?
- [ ] DB 스키마 변경 없음 확인

### Step 3: 테스트 확보 (Refactor Safety Net)

리팩토링 전 테스트 커버리지 확인:

```bash
# 현재 테스트 실행
<test-command>

# 커버리지 확인
<test-command> --coverage
```

- [ ] 핵심 로직 테스트 존재 확인
- [ ] 없으면 특성 테스트(Characterization Test) 먼저 작성
- [ ] 리팩토링 전후 같은 결과 보장

### Step 4: 단계별 실행 계획

큰 리팩토링을 작은 단계로 분리:

```
Phase 1 (이번 PR): <구체적인 변경>
  - 파일 A: 함수 X를 모듈 B로 이동
  - 예상 변경: 3 파일, 50줄

Phase 2 (다음 PR): <후속 변경>
  - 인터페이스 정리
  - ...
```

각 단계:
- 빌드/테스트 통과 확인 후 커밋
- PR 단위로 리뷰 가능한 크기 유지

### Step 5: 실행

```bash
# 각 단계 실행 후 검증
<build-command>
<test-command>
git diff --stat  # 변경 범위 확인
```

### Step 6: 완료 검증

- [ ] 모든 테스트 통과
- [ ] 빌드 성공
- [ ] 기존 동작 유지 확인
- [ ] 코드 복잡도 개선 확인
- [ ] PR 리뷰 완료

## 리팩토링 계획 요약 템플릿

```markdown
## 리팩토링 계획

**대상**: <파일/모듈>
**문제**: <현재 문제>
**목표**: <리팩토링 후 기대 상태>

**단계**:
1. [ ] Phase 1: ...
2. [ ] Phase 2: ...

**테스트 전략**: <어떻게 안전을 보장하는가>
**예상 기간**: X일
```

# CUSTOMIZE: 팀 코드 복잡도 기준, PR 크기 정책, 리팩토링 우선순위 규칙

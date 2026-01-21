# Systematic Debugging Rules

> "근본 원인 조사 없이 수정 금지." - 추측보다 체계적 접근이 2-3시간을 15-30분으로 줄인다.

---

## 적용 시점

- 테스트 실패
- 프로덕션 버그
- 예상치 못한 동작
- 성능 문제
- 빌드 실패
- 2회 이상 수정 시도 실패

---

## 4단계 필수 프로세스

### Phase 1: 근본 원인 조사 (수정 전 필수!)

| 단계 | 행동 | 이유 |
|------|------|------|
| **1. 에러 메시지 완독** | 경고 포함 전체 읽기 | 해결책과 라인 번호 포함됨 |
| **2. 재현 확인** | 일관되게 트리거 가능? | 재현 못하면 수정 불가 |
| **3. 최근 변경 확인** | `git diff`, 커밋 히스토리 | 원인 범위 좁히기 |
| **4. 경계 로깅** | 컴포넌트 입출력 로그 | 실패 지점 식별 |
| **5. 역추적** | 잘못된 값 → 원점 추적 | 증상 아닌 원인 수정 |

```bash
# ✅ GOOD: 체계적 조사
git log --oneline -10           # 최근 변경
git diff HEAD~3                 # 변경 내용
grep -r "ERROR\|WARN" logs/     # 에러 패턴

# ❌ BAD: 바로 수정 시도
"이거 바꾸면 될 것 같은데..."
```

### Phase 2: 패턴 분석

```python
# ✅ GOOD: 작동 코드와 비교
# 1. 유사한 작동 코드 찾기
working_code = find_similar_working_code()

# 2. 차이점 나열
differences = compare(working_code, broken_code)

# 3. 의존성/설정/가정 이해
for diff in differences:
    understand_why(diff)
```

### Phase 3: 가설 및 테스트

| 원칙 | 설명 |
|------|------|
| **명확한 가설** | "X가 원인이다, 왜냐하면..." |
| **최소 변경** | 한 번에 하나만 수정 |
| **단일 변수** | 여러 개 동시 수정 금지 |
| **실패 시 새 가설** | 수정 쌓기 금지 |

```python
# ✅ GOOD: 과학적 방법
hypothesis = "DB 연결 타임아웃이 원인 (로그에 timeout 에러)"
change = increase_timeout(30)
result = test()
if not result.success:
    new_hypothesis = "..."  # 새 가설

# ❌ BAD: 수정 쌓기
fix1()  # 실패
fix2()  # 또 실패
fix3()  # "이번엔 되겠지..."
```

### Phase 4: 구현

```python
# 1. 실패 테스트 먼저
def test_bug_reproduction():
    assert buggy_behavior() == expected  # 실패해야 함

# 2. 단일 수정
def fix():
    # 근본 원인 하나만 수정

# 3. 검증
def verify():
    assert test_bug_reproduction()  # 이제 통과
    assert all_other_tests_pass()   # 기존 테스트 유지
```

---

## CRITICAL - 즉시 멈춰야 할 신호

다음 생각이 들면 **프로세스로 돌아가기**:

| 위험 신호 | 대응 |
|----------|------|
| "일단 빠르게 고치고 나중에 조사" | ❌ 지금 조사 |
| "X 바꿔보면 될 것 같은데" | ❌ 가설 먼저 |
| "완전히 이해 못했지만 될 것 같아" | ❌ 이해 먼저 |
| "한 번만 더 시도" (2회+ 실패 후) | ❌ 아키텍처 재검토 |

---

## 아키텍처 문제 징후

3회 이상 수정 실패 시 **설계 결함** 가능성:

```
증상:
- 수정할 때마다 다른 곳에서 문제 발생
- 수정이 새로운 버그 유발
- "이상하게 작동하는데 왜인지 모름"

대응:
1. 패치 시도 중단
2. 팀과 근본 가정 재검토
3. 리팩토링 또는 재설계 고려
```

---

## 보조 기법

### 역추적 (Root Cause Tracing)

```python
# 콜스택 역방향 추적
def trace_backward(bad_value):
    """
    잘못된 값이 발견된 지점에서 시작
    → 해당 값을 설정한 함수로 이동
    → 그 함수의 입력값 확인
    → 반복하여 원점 도달
    """
    current = bad_value
    while not is_root_cause(current):
        current = find_source(current)
    return current
```

### 다층 방어 (Defense in Depth)

```python
# 근본 원인 수정 후, 여러 레이어에 검증 추가
def process_data(data):
    # Layer 1: 입력 검증
    validate_input(data)

    # Layer 2: 처리 중 검증
    result = transform(data)
    assert_valid(result)

    # Layer 3: 출력 검증
    validate_output(result)
    return result
```

### 조건 기반 대기 (Condition-Based Waiting)

```python
# ❌ BAD: 임의 타임아웃
time.sleep(5)  # 왜 5초?

# ✅ GOOD: 조건 폴링
def wait_for_condition(condition, timeout=30, interval=0.5):
    start = time.time()
    while time.time() - start < timeout:
        if condition():
            return True
        time.sleep(interval)
    raise TimeoutError("Condition not met")

wait_for_condition(lambda: db.is_connected())
```

---

## 결과 비교

| 지표 | 추측 기반 | 체계적 디버깅 |
|------|----------|--------------|
| **소요 시간** | 2-3시간 | 15-30분 |
| **첫 수정 성공률** | ~40% | ~95% |
| **새 버그 유발** | 흔함 | 거의 없음 |

---

## 체크리스트

### 수정 전
- [ ] 에러 메시지 전체 읽음
- [ ] 일관되게 재현 가능
- [ ] 최근 변경 확인
- [ ] 근본 원인 가설 있음

### 수정 중
- [ ] 한 번에 하나만 변경
- [ ] 실패 테스트 먼저 작성
- [ ] 가설 기반 수정

### 수정 후
- [ ] 버그 테스트 통과
- [ ] 기존 테스트 유지
- [ ] 유사 문제 방지책 추가

---

## 참고

- [obra/superpowers - systematic-debugging](https://github.com/obra/superpowers)

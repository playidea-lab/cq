# C4 사용자 여정 & 운영 플로우

## 1. 문서 목적

본 문서는 **C4 시스템을 사용하는 사용자의 실제 행동 흐름(User Journey)**과  
그에 대응하는 **시스템 내부 운영 플로우(Operational Flow)**를 명확히 정의한다.

이 문서는 다음을 고정하기 위한 기준 문서이다:
- 사용자는 언제 어떤 명령을 치는가?
- 시스템(c4 / c4d / Claude Code)은 그때 무엇을 하는가?
- 사람 개입이 필요한 지점과 자동화 지점을 어디서 분리하는가?

---

## 2. 전체 사용자 여정 요약

### 한 줄 요약
> **C4는 “프로젝트 하나를 끝낼 때까지 AI가 멈추지 않도록” 만드는 운영 루프다.**

### 전체 흐름
```
Project Init
  → Plan (대화/토론)
    → Execute (Worker Loop)
      → Checkpoint (Supervisor Gate)
        → Execute
          → ...
            → Complete
```

---

## 3. Phase 0 — 프로젝트 진입

### 사용자 행동
```bash
cd my-project
claude
```

### 시스템 상태
- c4는 아직 활성화되지 않음
- 일반 Claude Code 세션

### 목적
- 프로젝트 맥락 로딩
- 초기 탐색/질문 가능

---

## 4. Phase 1 — Plan 모드

### 사용자 행동
```text
/c4 init
```

### 시스템 동작
- C4 프로젝트 초기화
- `.c4/` 디렉토리 생성
- 상태 전이: `INIT → PLAN`
- Plan 전용 프롬프트 규칙 활성화

### Plan 모드의 특징
- **깊은 대화 허용**
- 검색, 비교, 토론 적극 사용
- 즉시 실행 금지

### 산출물 (필수)
- `docs/PLAN.md`
- `docs/CHECKPOINTS.md`
- `docs/DONE.md`
- `todo.md`

> ⚠️ Plan 모드는 “문서가 완성되기 전까지” 끝나지 않는다.

---

## 5. Phase 2 — 실행 시작

### 사용자 행동
```text
/c4 run
```

### 시스템 동작
- `c4d` 데몬 시작 (leader lock 획득)
- 상태 전이: `PLAN → EXECUTE`
- task 큐 초기화
- 워커 join 대기

---

## 6. Phase 3 — Worker 실행 루프

### 사용자 행동 (워커 1)
```text
/c4 worker join
```

### 사용자 행동 (워커 n)
```text
/c4 worker join
```

### 시스템 동작
- 워커 등록
- task 할당 (scope lock 포함)
- 워커 전용 프롬프트 활성화
- Ralph Loop 실행

### 워커 내부 루프
```
task 수령
  → 구현
    → 테스트
      → 실패 시 수정
        → 반복
          → 성공 시 제출
```

### 워커 결과 제출
```text
/c4 worker submit --commit abc123
```

또는 자동 감지

---

## 7. Phase 4 — Checkpoint 도달

### 트리거 조건
- 특정 task 묶음 완료
- Gate 조건 충족

### 시스템 동작
- 상태 전이: `EXECUTE → CHECKPOINT`
- 리뷰 번들(bundle) 생성
- Supervisor 자동 호출 (headless)

---

## 8. Phase 5 — Supervisor Gate

### 시스템 동작 (자동)
```bash
claude -p "$(cat prompt_supervisor.md)" --output-format json
```

### Supervisor 판단 결과
- `APPROVE`
- `REQUEST_CHANGES`
- `REPLAN_REQUIRED`

### 결과 처리
- **APPROVE**
  - 다음 단계 EXECUTE 재개
- **REQUEST_CHANGES**
  - todo.md에 작업 자동 추가
  - EXECUTE 복귀
- **REPLAN_REQUIRED**
  - PLAN 또는 CHECKPOINT로 회귀

> Supervisor는 **Gate 기준만** 판단한다.

---

## 9. Phase 6 — 반복 실행

- Phase 3 ~ Phase 5 반복
- 사람이 개입하지 않아도 자동 순환
- 워커 수는 자유롭게 증감 가능

---

## 10. Phase 7 — 완료

### 완료 조건
- `DONE.md` 조건 충족
- 모든 Gate 통과

### 시스템 동작
- 상태 전이: `EXECUTE → COMPLETE`
- 데몬 종료
- 최종 리포트 생성 (선택)

### 사용자 행동
```text
/c4 status
```

---

## 11. 멀티 터미널 시나리오

### 터미널 A (Leader)
```bash
claude
/c4 run
```

### 터미널 B (모니터링)
```bash
claude
/c4 status
```

### 터미널 C/D (Worker)
```bash
claude
/c4 worker join
```

---

## 12. 자동화 vs 인간 개입 지점

| 구간 | 자동 | 인간 |
|----|----|----|
| Plan 토론 | ❌ | ✅ |
| 실행 루프 | ✅ | ❌ |
| 테스트 | ✅ | ❌ |
| 체크포인트 판단 | ✅ | ❌(옵션) |
| 재계획 | ❌ | ✅ |
| 종료 | ✅ | ❌ |

---

## 13. 설계 원칙 요약

- **Plan은 깊게, Execute는 빠르게**
- **실행 중에는 판단 금지**
- **Checkpoint에서만 방향/품질 판단**
- **AI는 멈추지 않는다**
- **사람은 통제권만 가진다**

---

## 14. 이 문서의 역할

- PRD를 실제 UX/운영 흐름으로 구체화
- 이후 상태머신/CLI/API 설계의 기준
- 팀/에이전트 공통 이해 문서


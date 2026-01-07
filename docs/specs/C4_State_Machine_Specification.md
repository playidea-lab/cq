# C4 상태머신 명세 (State Machine Specification)

## 1. 문서 목적

본 문서는 **C4 시스템의 모든 상태(State)와 상태 전이(Transition)**를 단일 기준으로 정의한다.  
이 문서는 다음의 *헌법* 역할을 한다.

- c4d 구현의 기준
- CLI 명령의 허용/금지 조건 판단 기준
- Worker / Supervisor 행동 제한 기준
- 자동화/복구/예외 처리의 기준

---

## 2. 상태머신 설계 원칙

1. **모든 상태 전이는 명시적 이벤트로만 발생한다**
2. **한 시점에 하나의 상위 상태만 존재한다**
3. **실행 중(EXECUTE)에는 판단을 하지 않는다**
4. **판단은 CHECKPOINT에서만 수행한다**
5. **상태는 언제든 복구 가능해야 한다(state.json)**

---

## 3. 상위 상태 목록 (Top-level States)

| 상태 | 설명 |
|---|---|
| INIT | C4 초기화 전 |
| PLAN | 계획 수립 단계 |
| EXECUTE | 워커 실행 단계 |
| CHECKPOINT | 감독/판단 단계 |
| COMPLETE | 프로젝트 완료 |
| HALTED | 사용자/시스템에 의해 중단 |
| ERROR | 복구 불가 오류 상태 |

---

## 4. 상태 다이어그램 (개념)

```
INIT
  ↓
PLAN
  ↓
EXECUTE ←──────────┐
  ↓                │
CHECKPOINT         │
  ↓                │
COMPLETE           │
                   │
PLAN ← REPLAN ─────┘
```
(REQUEST_CHANGES는 EXECUTE로 복귀)

---

## 5. 상태별 상세 명세

### 5.1 INIT

#### 의미
- C4가 아직 활성화되지 않은 상태

#### 진입 조건
- 프로젝트 폴더 진입
- `.c4/` 없음

#### 허용 명령
- `/c4 init`

#### 전이
- `/c4 init` → PLAN

---

### 5.2 PLAN

#### 의미
- 프로젝트 계획 수립 단계
- **사람 중심 + AI 토론 허용**

#### 허용 행동
- 자유 대화
- 검색/비교/설계 토론
- 문서 수정

#### 필수 산출물
- `docs/PLAN.md`
- `docs/CHECKPOINTS.md`
- `docs/DONE.md`
- `todo.md`

#### 허용 명령
- `/c4 plan` (재진입)
- `/c4 run` (Plan 완료 시)

#### 전이
- `/c4 run` → EXECUTE
- `/c4 stop` → HALTED

---

### 5.3 EXECUTE

#### 의미
- **판단 없는 실행 단계**
- 워커(Ralph Loop)가 task를 수행

#### 내부 하위 상태 (execution.mode)
- `running`
- `paused`
- `repair`

#### 허용 행동
- task 할당
- 코드 작성/수정
- 테스트 실행

#### 금지 행동
- 설계 변경
- 범위 확장
- Gate 판단

#### 허용 명령
- `/c4 status`
- `/c4 worker join`
- `/c4 stop`

#### 전이 트리거
- Gate 조건 충족 → CHECKPOINT
- 사용자 중단 → HALTED
- 반복 실패 초과 → CHECKPOINT (repair)

---

### 5.4 CHECKPOINT

#### 의미
- **유일한 판단 단계**
- Supervisor Gate 수행

#### 입력
- diff.patch
- test_report.json
- gate_checklist
- todo_progress

#### 허용 판단 결과
- APPROVE
- REQUEST_CHANGES
- REPLAN_REQUIRED

#### 전이
- APPROVE → EXECUTE (또는 COMPLETE)
- REQUEST_CHANGES → EXECUTE
- REPLAN_REQUIRED → PLAN

---

### 5.5 COMPLETE

#### 의미
- 모든 DONE 조건 충족

#### 시스템 동작
- 데몬 종료
- 최종 상태 기록

#### 허용 명령
- `/c4 status`

---

### 5.6 HALTED

#### 의미
- 사용자 또는 시스템에 의해 중단

#### 진입 조건
- `/c4 stop`
- 치명적 오류 회피

#### 허용 명령
- `/c4 run` (재개)
- `/c4 status`

---

### 5.7 ERROR

#### 의미
- 자동 복구 불가능한 상태

#### 진입 조건
- state.json 손상
- 반복적 Supervisor 실패

#### 대응
- 수동 개입 필요

---

## 6. 상태 전이 표 (Transition Table)

| From | Event | To |
|---|---|---|
| INIT | c4_init | PLAN |
| PLAN | c4_run | EXECUTE |
| EXECUTE | gate_reached | CHECKPOINT |
| CHECKPOINT | approve | EXECUTE |
| CHECKPOINT | request_changes | EXECUTE |
| CHECKPOINT | replan | PLAN |
| EXECUTE | done | COMPLETE |
| ANY | c4_stop | HALTED |
| ANY | fatal_error | ERROR |

---

## 7. Invariants (불변 조건)

- EXECUTE 중에는 PLAN 문서 수정 금지
- CHECKPOINT 중에는 워커 실행 금지
- leader.lock은 EXECUTE/CHECKPOINT에서만 유지
- state.json은 모든 전이 후 즉시 flush

---

## 8. 복구 규칙

- 데몬 재시작 시:
  - state.json 로드
  - 마지막 안정 상태로 복귀
- HALTED → EXECUTE 시:
  - scope lock 재검증
- CHECKPOINT 중단 시:
  - Supervisor 재실행 가능

---

## 9. 구현 가이드 (c4d)

- 상태 전이는 반드시 이벤트 기록 후 수행
- 상태 변경은 단일 함수(`transition(state, event)`)로만 수행
- CLI는 현재 상태를 기준으로 명령 허용 여부 판단

---

## 10. 요약

- C4는 **명시적 상태머신 기반 시스템**
- EXECUTE = 실행, CHECKPOINT = 판단
- 상태 분리는 자동화 안정성의 핵심


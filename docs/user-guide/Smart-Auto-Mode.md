# Smart Auto Mode 가이드

> **v0.6.6 신규**: `/c4-run`이 **Smart Auto Mode**를 기본으로 사용합니다.
> 태스크 의존성 그래프를 분석하여 자동으로 최적의 Worker 수를 결정하고 병렬 실행합니다.

---

## 개요

Smart Auto Mode는 C4의 자동화된 병렬 실행 시스템입니다. 태스크 간 의존성을 분석하여 동시에 실행 가능한 태스크를 식별하고, 적절한 수의 Worker를 자동으로 스폰하여 작업을 병렬 처리합니다.

### 핵심 기능

| 기능 | 설명 |
|------|------|
| **의존성 분석** | 태스크 DAG(Directed Acyclic Graph) 분석 |
| **자동 병렬화** | 동시 실행 가능한 태스크 식별 및 Worker 스폰 |
| **Worktree 격리** | Worker별 독립 Git worktree로 충돌 방지 |
| **동적 조정** | 실행 중 태스크 상태에 따라 Worker 조정 |

---

## 사용법

### 기본 사용 (자동 모드)

```bash
/c4-run
```

C4가 자동으로:
1. 태스크 의존성 분석
2. 병렬화 가능한 태스크 식별
3. 적절한 수의 Worker 스폰 (기본: 2~4)
4. 각 Worker는 독립된 Worktree에서 작업

### 수동 Worker 수 지정

```bash
/c4-run 1         # 단일 Worker (디버깅용, 이 세션에서 직접 실행)
/c4-run 3         # 3개 Worker 스폰
/c4-run --max 4   # 자동 모드이지만 최대 4개로 제한
```

---

## 병렬도 분석

`/c4-status` 또는 `/c4-run` 실행 시 병렬도 분석 결과가 표시됩니다:

```
📊 병렬도 분석:
   총 10개 태스크
   현재 실행 가능: 6개
   의존성 대기: 4개
   DAG 최대 너비: 5

💡 추천: 4개 Worker
   이유: 6 tasks ready, capped at 4 workers

🚀 실행: 4개 Worker
```

### 분석 지표

| 지표 | 설명 |
|------|------|
| **pending_total** | 전체 대기 중인 태스크 수 |
| **ready_now** | 의존성이 충족되어 즉시 실행 가능한 태스크 |
| **blocked_count** | 의존성 미충족으로 대기 중인 태스크 |
| **max_parallelism** | DAG 분석 기반 최대 동시 실행 가능 수 |
| **recommended** | 시스템 추천 Worker 수 |
| **by_model** | 모델별 태스크 분포 (opus/sonnet/haiku) |

---

## Worktree 격리

멀티 Worker 환경에서 Git 충돌을 방지하기 위해 각 Worker는 독립된 worktree에서 작업합니다.

### 구조

```
your-project/
├── .c4/
│   └── worktrees/
│       ├── worker-abc123/    # Worker 1의 작업 공간
│       │   ├── .git          # 링크된 git 디렉토리
│       │   └── src/          # 복사된 소스 코드
│       └── worker-def456/    # Worker 2의 작업 공간
│           ├── .git
│           └── src/
└── src/                      # 메인 프로젝트
```

### 동작 방식

1. `c4_get_task()` 호출 시 Worker에게 `worktree_path` 반환
2. Worker는 해당 경로에서만 파일 작업 수행
3. 체크포인트 승인 시 자동으로 메인 브랜치에 병합

### 설정

```yaml
# .c4/config.yaml
worktree:
  enabled: true              # Worktree 격리 사용 (기본: true)
  base_branch: main          # 기준 브랜치
  completion_action: merge   # pr: PR 생성, merge: 직접 병합
```

---

## Worker Loop 상세

### 단일 Worker 모드 (`/c4-run 1`)

이 세션에서 직접 Worker Loop를 실행합니다:

```
LOOP:
  task = c4_get_task(WORKER_ID)
  if task is null:
      exit("✅ COMPLETE")

  # Worktree 경로 확인
  if task.worktree_path:
      WORK_DIR = task.worktree_path
  else:
      WORK_DIR = "."

  implement(task)           # DoD 기반 구현
  validate()                # lint, unit test
  if fail_count >= 10:
      mark_blocked(task)
      exit("⏸️ BLOCKED")

  commit()
  result = submit(task)

  if result.next_action == "get_next_task":
      continue LOOP
  elif result.next_action == "await_checkpoint":
      poll until EXECUTE or exit
  elif result.next_action == "complete":
      exit("✅ DONE")
```

### 멀티 Worker 모드 (`/c4-run` 또는 `/c4-run N`)

Task subagent로 Worker를 스폰합니다. 각 Worker는 독립적으로 실행됩니다.

```python
# Worker 스폰 (내부 동작)
for i in range(worker_count):
    worker_id = f"worker-{uuid.uuid4().hex[:8]}"

    Task(
        subagent_type="general-purpose",
        description=f"C4 Worker {i+1}/{worker_count}",
        prompt=WORKER_PROMPT.format(worker_id=worker_id),
        run_in_background=True
    )
```

---

## Economic Mode 통합

Smart Auto Mode는 Economic Mode와 함께 작동하여 비용을 최적화합니다.

### 모델별 태스크 분배

```yaml
# 태스크 정의 시 모델 지정
c4_add_todo:
  task_id: "T-001-0"
  title: "아키텍처 설계"
  model: "opus"        # 복잡한 작업

c4_add_todo:
  task_id: "T-002-0"
  title: "린트 수정"
  model: "haiku"       # 단순 작업
```

### 병렬도 분석의 모델 정보

```python
status = c4_status()
parallelism = status["parallelism"]

# 모델별 분포 확인
by_model = parallelism["by_model"]
# {"opus": 3, "sonnet": 5, "haiku": 2}
```

### 모델 권장 사용

| 모델 | 용도 | 예시 |
|------|------|------|
| **opus** | 복잡한 아키텍처, 설계 결정 | 시스템 설계, 어려운 버그 |
| **sonnet** | 일반 구현, 리뷰 (기본값) | 기능 구현, 코드 리뷰 |
| **haiku** | 단순 반복, 린트 수정 | 포맷팅, 간단한 수정 |

---

## Agent Routing 통합

`c4_get_task()` 응답에 에이전트 라우팅 정보가 포함됩니다:

```python
task = c4_get_task(WORKER_ID)

# Agent Routing 정보
task.recommended_agent   # "frontend-developer"
task.agent_chain        # ["frontend-developer", "test-automator", "code-reviewer"]
task.domain             # "web-frontend"
task.task_type          # "review" (code-reviewer 자동 할당)
```

Worker는 이 정보를 활용하여 적절한 페르소나로 작업을 수행합니다.

---

## 예제 시나리오

### 시나리오 1: 프론트엔드 프로젝트

```
프로젝트: React 컴포넌트 라이브러리
태스크:
  T-001-0: Button 컴포넌트 (의존성: 없음)
  T-002-0: Input 컴포넌트 (의존성: 없음)
  T-003-0: Form 컴포넌트 (의존성: T-001-0, T-002-0)
  T-004-0: 스토리북 설정 (의존성: T-001-0, T-002-0, T-003-0)
```

```
/c4-run

📊 병렬도 분석:
   총 4개 태스크
   현재 실행 가능: 2개 (T-001-0, T-002-0)
   의존성 대기: 2개
   DAG 최대 너비: 2

💡 추천: 2개 Worker
   이유: 2 tasks ready

🚀 실행: 2개 Worker
```

**실행 흐름:**
1. Worker 1: T-001-0 (Button) 작업
2. Worker 2: T-002-0 (Input) 작업
3. 두 태스크 완료 후 T-003-0이 실행 가능해짐
4. Worker가 T-003-0 작업
5. T-003-0 완료 후 T-004-0 작업
6. 모든 태스크 완료

### 시나리오 2: 마이크로서비스

```
프로젝트: 3개 독립 서비스
태스크:
  T-001-0: User Service (의존성: 없음)
  T-002-0: Product Service (의존성: 없음)
  T-003-0: Order Service (의존성: 없음)
  T-004-0: API Gateway (의존성: T-001-0, T-002-0, T-003-0)
```

```
/c4-run

📊 병렬도 분석:
   총 4개 태스크
   현재 실행 가능: 3개
   DAG 최대 너비: 3

💡 추천: 3개 Worker

🚀 실행: 3개 Worker
```

**실행 흐름:**
1. Worker 1, 2, 3이 각각 서비스 구현
2. 3개 서비스 완료 후 T-004-0 (API Gateway) 작업
3. 완료

---

## 제약사항

| 제약 | 설명 |
|------|------|
| **최대 Worker** | 7개 (Claude Code subagent 한계) |
| **Worktree 필수** | 멀티 Worker 시 worktree 격리 필수 |
| **Accept Edits** | 자동화에 Accept Edits 모드 필수 |
| **Scope Lock** | 같은 scope의 태스크는 동시 실행 불가 |

### Accept Edits 모드

자동화 작업 전에 **Accept Edits** 모드가 켜져 있는지 확인하세요:

- 화면 하단 상태바에서 "Accept Edits" 표시 확인
- 또는 `Shift+Tab` 눌러서 활성화

Accept Edits가 꺼져있으면 매 파일 수정마다 승인 필요 → 자동화 불가

---

## 모니터링

### 진행 상황 확인

```bash
/c4-status
```

출력:
```
📊 C4 Status
   상태: EXECUTE
   실행 중: T-001-0 (worker-abc123), T-002-0 (worker-def456)
   대기 중: 5개
   완료: 3개

👷 Workers:
   worker-abc123: busy (T-001-0)
   worker-def456: busy (T-002-0)
```

### Worker 로그 확인

```bash
# 개별 Worker 로그 (백그라운드 모드)
tail -f {worker_output_file}

# 이벤트 로그
ls .c4/events/
```

---

## 문제 해결

### Worker가 태스크를 받지 못함

**원인**: 의존성 미충족 또는 scope lock

**해결**:
```bash
/c4-status  # 의존성 확인
```

### Worktree 충돌

**원인**: 이전 Worker의 worktree가 정리되지 않음

**해결**:
```bash
# .c4/worktrees/ 내 불필요한 디렉토리 삭제
rm -rf .c4/worktrees/worker-*
```

### 병렬 Worker가 같은 파일 수정

**원인**: 태스크 scope가 겹침

**해결**:
- 태스크 정의 시 `scope` 명확히 지정
- 같은 scope의 태스크는 순차 실행됨

---

## 관련 문서

- [워크플로우 개요](워크플로우-개요.md) - 전체 C4 워크플로우
- [명령어 레퍼런스](명령어-레퍼런스.md) - 슬래시 명령어 상세
- [문제 해결](문제-해결.md) - FAQ 및 트러블슈팅

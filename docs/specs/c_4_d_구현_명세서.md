# c4d 구현 명세서 (c4 daemon)

> 목표: **Claude Code-native c4**를 멀티 터미널/멀티 워커로 안정적으로 운영하기 위해, 토큰을 소모하지 않는 로컬 데몬(**c4d**)이 프로젝트 상태머신을 유지하고 작업 할당/검증/체크포인트 트리거를 수행한다.

---

## 0. 범위

### 포함(What c4d does)

- 프로젝트 **리더(Leader)** 역할: 상태머신 유지, 작업 큐 관리, 워커 할당
- **락/동시성 제어**: leader lock, scope lock
- **이벤트 큐**: append-only 이벤트 로그 생성/소비
- **검증 실행**: 테스트/린트/형식검사 명령 실행 및 결과 기록
- **체크포인트 게이트 평가**: CP0/CP1/CP2 등 Gate 충족 여부 판단
- **슈퍼바이저 대기/트리거**: 리뷰 필요 이벤트 생성 + 리뷰 번들(bundle) 생성
- **클라이언트 인터페이스 제공**: `c4` CLI 또는 Claude Code 플러그인이 c4d에 연결해 status/명령 수행

### 제외(What c4d does NOT do)

- LLM(Claude/Codex/Completion)을 직접 호출해 “코드 작성” 수행
- Claude Code UI에 직접 메시지 푸시(플랫폼 종속 기능)
- Git remote 관리(선택 기능으로 확장 가능)

---

## 1. 핵심 개념

### 1.1 프로젝트 상태머신

- 상태는 `.c4/state.json`에 저장되고, c4d는 이를 \*\*단일 진실 소스(Single Source of Truth)\*\*로 사용한다.
- 주요 상태:
  - `INIT` → `PLAN` → `EXECUTE` → `CHECKPOINT` → `COMPLETE`
- 하위 상태:
  - `checkpoint.current`: `CP0|CP1|CP2|...`
  - `execution.mode`: `leader|paused|awaiting_supervisor|repair`

### 1.2 역할

- **Leader(c4d)**: 작업 할당/검증/게이트 판단/이벤트 발행
- **Worker(Claude Code 세션)**: 할당받은 task 수행 후 결과 제출
- **Supervisor(Claude Code 세션)**: 체크포인트에서 승인/반려/재계획 결정

---

## 2. 디렉토리/파일 레이아웃

프로젝트 루트 아래 `.c4/`는 c4d가 관리한다.

```
.c4/
  state.json
  config.yaml
  locks/
    leader.lock
    scope.<scope_id>.lock
  events/
    000001-...json
    000002-...json
  bundles/
    cp-CP1-<timestamp>/
      diff.patch
      test_report.json
      gate_checklist.md
      todo_progress.md
      prompt_supervisor.md
  workers/
    <worker_id>.json
  runs/
    tests/<timestamp>.json
    logs/<timestamp>.log
```

---

## 3. 상태 스키마

### 3.1 `.c4/config.yaml` (예시)

- `project_id`: string
- `default_branch`: main
- `work_branch_prefix`: c4/w-
- `leader`:
  - `poll_interval_ms`: 1000
  - `max_idle_minutes`: 0  # 0=무제한
- `locks`:
  - `scope_lock_ttl_sec`: 3600
- `validation`:
  - `commands`:
    - `lint`: "npm run lint"
    - `unit`: "npm test"
    - `e2e`: "npm run e2e"
  - `required`: [lint, unit]
- `checkpoints`:
  - `CP0`: required\_tasks: ["T-001", "T-002"] required\_validations: [lint, unit]
  - `CP1`: required\_tasks: ["T-010", "T-011"] required\_validations: [lint, unit]
  - `CP2`: required\_tasks: [] required\_validations: [lint, unit, e2e]
- `budgets`:
  - `max_iterations_per_task`: 7
  - `max_failures_same_signature`: 3

### 3.2 `.c4/state.json` (예시)

```json
{
  "project_id": "my-awesome-project",
  "status": "EXECUTE",
  "checkpoint": {"current": "CP1", "state": "in_progress"},
  "queue": {
    "pending": ["T-013", "T-014"],
    "in_progress": {"T-012": "worker-1"},
    "done": ["T-001", "T-002"]
  },
  "workers": {
    "worker-1": {"state": "busy", "task_id": "T-012", "scope": "ui/login", "branch": "c4/w-T012"}
  },
  "locks": {
    "leader": {"owner": "c4d", "pid": 12345, "started_at": "2026-01-07T04:00:00Z"},
    "scopes": {"ui/login": {"owner": "worker-1", "expires_at": "2026-01-07T05:00:00Z"}}
  },
  "last_validation": {"lint": "pass", "unit": "fail", "timestamp": "..."},
  "metrics": {"events_emitted": 104, "validations_run": 21}
}
```

---

## 4. 이벤트 모델

c4d는 모든 상태 변화를 **이벤트로 기록**한다.

### 4.1 이벤트 파일 규칙

- 파일명: `NNNNNN-<ISO8601>-<type>.json` (6자리 증가)
- append-only, 수정 금지

### 4.2 이벤트 타입(필수)

- `LEADER_STARTED`
- `WORKER_JOINED`
- `TASK_ASSIGNED`
- `WORKER_SUBMITTED`
- `VALIDATION_STARTED`
- `VALIDATION_FINISHED`
- `CHECKPOINT_REQUIRED`
- `SUPERVISOR_DECISION`
- `STATE_CHANGED`
- `ERROR`

### 4.3 이벤트 스키마(공통)

```json
{
  "id": "000123",
  "ts": "2026-01-07T13:10:00+09:00",
  "type": "TASK_ASSIGNED",
  "actor": "c4d",
  "data": {"task_id": "T-012", "worker_id": "worker-1", "scope": "ui/login"}
}
```

---

## 5. 락/동시성 설계

### 5.1 Leader lock

- 목적: 프로젝트당 리더는 1개
- 구현: `.c4/locks/leader.lock` 파일에 pid, started\_at 기록
- 획득 실패 시: 현재 리더 정보 반환
- 크래시 복구: pid 존재 여부 확인 + heartbeat(선택)로 stale lock 해제

### 5.2 Scope lock

- 목적: 동일 scope에 중복 할당 방지
- 구현: `.c4/locks/scope.<hash>.lock` + state.json 기록
- TTL 기반 만료(네트워크/세션 끊김 대비)

---

## 6. 작업 큐/할당

### 6.1 todo.md → task registry

- `todo.md`는 사람이 보지만, c4d는 파싱하여 **task registry**로 변환해 `.c4/tasks.json`(선택) 캐시
- task 필드(권장):
  - `id`, `title`, `scope`, `priority`, `dod`(Definition of Done), `validations`, `dependencies`

### 6.2 할당 알고리즘(기본)

1. 워커 join
2. `pending` 중 dependencies 해결된 task 후보 중
3. priority 높은 순 + scope lock 확보 가능한 것 선택
4. 워커에 할당 및 이벤트 기록

### 6.3 원자적 태스크 할당 (Race Condition 방지)

멀티 워커 환경에서 동일 태스크가 여러 워커에게 중복 할당되는 것을 방지하기 위해 **2단계 원자적 할당**을 사용한다:

**Phase 1: Scope Lock 획득**
- SQLite 기반 `acquire_scope_lock()`으로 락 획득
- scope가 있는 태스크: scope 값을 락 키로 사용
- scope가 없는 태스크: `__task__:{task_id}`를 락 키로 사용
- 락 획득 실패 시 다음 태스크로 이동

**Phase 2: Atomic State Modification**
- `atomic_modify()` 블록 내에서 상태 변경
- Double-check: 태스크가 여전히 pending 상태인지 재확인
- pending에서 제거 → in_progress에 worker_id와 함께 추가
- 할당 실패 시 락 해제 후 다음 태스크 시도

```
[Worker A]                    [Worker B]
    |                             |
    +-- acquire_scope_lock() ---> OK
    |                             +-- acquire_scope_lock() ---> FAIL (skip)
    +-- atomic_modify() -------->
    |   (remove from pending)
    |   (add to in_progress)
    +-- return TaskAssignment
```

이 방식은 락 획득과 상태 변경 사이의 경쟁 상태를 방지하며, 워커가 불필요한 작업을 수행하는 것을 원천 차단한다.

---

## 7. 검증(Validation) 실행

### 7.1 실행 방식

- `child_process.spawn`(Node) 또는 subprocess(Python)로 실행
- stdout/stderr 저장: `.c4/runs/logs/*.log`
- 결과 저장: `.c4/runs/tests/<timestamp>.json`

### 7.2 결과 스키마(예시)

```json
{
  "ts": "...",
  "command": "npm test",
  "exit_code": 1,
  "duration_ms": 8321,
  "summary": "2 failed, 120 passed",
  "failure_signature": "TypeError: ... at src/..."
}
```

---

## 8. 체크포인트(Checkpoint) 게이트 평가

### 8.1 게이트 평가 입력

- `config.yaml`의 checkpoints 정의
- `state.json`의 task 진행률
- 최근 validation 결과

### 8.2 판단

- required tasks done AND required validations pass → `CHECKPOINT_REQUIRED`(리뷰 필요) 또는 자동 승인 정책

### 8.3 리뷰 번들(bundle) 생성

- 위치: `.c4/bundles/cp-<CP>-<ts>/`
- 포함:
  - `diff.patch`: 현재 work 브랜치 vs main
  - `test_report.json`: 최근 validation 결과
  - `gate_checklist.md`: Gate 체크리스트 자동 채움
  - `todo_progress.md`: 남은 todo 요약
  - `prompt_supervisor.md`: Supervisor에게 줄 프롬프트(출력 포맷 포함)

---

## 9. Supervisor 대기/결정 플로우

> **업데이트 (중요): Claude Code는 공식적으로 headless(비대화형) 실행을 지원한다.** 따라서 c4d는 **Supervisor를 완전 자동으로 호출**할 수 있으며, 반자동 단계를 기본값으로 둘 필요가 없다.

### 9.1 c4d의 역할

- `CHECKPOINT_REQUIRED` 이벤트 발행
- `awaiting_supervisor` 상태로 전환
- **Claude Code headless 실행(**``**)을 통해 Supervisor 리뷰 자동 수행**
- Supervisor 결정 결과를 이벤트로 기록하고 상태 전이

### 9.2 Supervisor 호출 방식 (Headless, 권장 기본값)

c4d는 체크포인트 도달 시 다음 절차로 Supervisor를 자동 호출한다.

1. 리뷰 번들(bundle) 생성
2. 번들 내 `prompt_supervisor.md`를 입력으로 사용
3. Claude Code를 **비대화형(headless) 모드**로 실행

```bash
claude -p "$(cat bundle/prompt_supervisor.md)" \
  --allowedTools "Read,Grep,Glob" \
  --permission-mode acceptEdits \
  --output-format json \
  --max-turns 10 \
  > bundle/supervisor_decision.json
```

- `--permission-mode acceptEdits` : 파일 수정 자동 승인
- `--output-format json` : Supervisor 결정 파싱 용이
- `--max-turns` : 리뷰 루프 상한

### 9.3 Supervisor 입력 번들 규칙

`prompt_supervisor.md`에는 반드시 아래 요소를 포함한다:

- 역할 선언: "You are the C4 Supervisor."
- 체크포인트 ID 및 Gate 조건
- `diff.patch` 요약 또는 참조 경로
- `test_report.json` 요약
- `todo_progress.md` 요약
- **출력 포맷 강제(JSON)**

출력 예시(JSON):

```json
{
  "decision": "APPROVE",
  "checkpoint": "CP1",
  "notes": "All gates satisfied",
  "required_changes": []
}
```

### 9.4 Supervisor 결정 처리

c4d는 `supervisor_decision.json`을 파싱하여 다음 중 하나를 수행한다:

- **APPROVE**

  - 체크포인트 통과 이벤트 기록
  - 다음 체크포인트 또는 EXECUTE 단계로 전환

- **REQUEST\_CHANGES**

  - `required_changes` 항목을 `todo.md`에 자동 추가
  - 상태를 `EXECUTE`로 되돌리고 워커 재개

- **REPLAN\_REQUIRED**

  - 상태를 `PLAN` 또는 `CHECKPOINT`로 전환
  - PLAN 문서 수정 요구 이벤트 기록

### 9.5 반자동 모드(옵션)

자동 Supervisor가 실패하거나 사람이 개입하길 원할 경우:

```bash
c4 supervise --bundle bundle/cp-CP1-<ts>
```

- 동일한 `prompt_supervisor.md`를 사용
- Claude Code 대화형 세션에서 실행 가능

---

## 10. 클라이언트 인터페이스

### 10.1 IPC 방식(추천)

- 로컬 HTTP(127.0.0.1) + Unix domain socket 중 택1
- 인증: 프로젝트 토큰 파일(`.c4/daemon.token`) 또는 OS user check

### 10.2 API (예시)

- `POST /start`
- `POST /stop`
- `GET /status`
- `POST /worker/join`
- `POST /worker/submit`
- `POST /checkpoint/request`
- `POST /supervisor/decision`

### 10.3 CLI 명령(예시)

- `c4d start`
- `c4d stop`
- `c4 status` (→ c4d status)
- `c4 worker join`
- `c4 worker submit --commit <id> --branch <name>`
- `c4 supervise --bundle <path>`

---

## 11. Git 연동 정책

### 11.1 워커 브랜치 규칙

- 브랜치: `c4/w-<task_id>`
- 워커는 자신의 브랜치에서만 작업

### 11.2 통합 정책(기본)

- Supervisor APPROVE 후에만 main으로 merge
- c4d는 merge 자체는 옵션(초기엔 사람이 merge해도 됨)

### 11.3 diff 생성

- `git diff main...<branch> > diff.patch`

---

## 12. 장애/복구

- 데몬 재시작 시:
  - leader.lock 확인
  - state.json 로드
  - events 재생(replay)로 일관성 확인(선택)
- 워커 세션 끊김:
  - scope lock TTL 만료로 회수
  - task를 pending으로 되돌림
- 무한 실패:
  - `failure_signature` 반복 횟수 초과 시 `repair` 상태로 전환 + Supervisor 요청

---

## 13. 보안/안전장치

- 위험 명령어 차단(옵션): `rm -rf`, `sudo` 등
- validation command allowlist
- 이벤트/번들에 민감정보 기록 금지

---

## 14. 테스트 계획

### 14.1 단위 테스트

- todo parser
- lock manager
- scheduler(할당) 로직
- gate evaluator
- event writer

### 14.2 통합 테스트

- 가짜 repo 생성 → c4d start → worker join/submit 시나리오
- 체크포인트 도달 → bundle 생성 → supervisor decision 처리

---

## 15. 구현 로드맵 (권장)

### v0 (2\~4일)

- leader.lock + state.json
- status API/CLI
- todo 파싱 + 단일 워커 할당
- validation 실행(단일 command)
- checkpoint\_required 이벤트 + bundle 생성

### v1 (1\~2주)

- 멀티 워커 + scope lock
- 이벤트 큐 + replay
- supervisor decision 반영(todo 자동 생성)

### v2 (2\~4주)

- 자동 merge(옵션)
- watch 모드(폴링 UI)
- failure signature 기반 repair 모드

---

## 16. 오픈 이슈

- Claude Code에서 ‘자동 supervisor 세션 실행’을 얼마나 자동화할지(반자동 권장)
- Windows 환경에서의 파일 락/소켓 선택
- Git merge 자동화 범위(보수적으로 시작)


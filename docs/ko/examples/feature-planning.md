# 기능 계획: /pi → /plan → /run

CQ의 전체 계획 워크플로우를 사용하여 대형 기능 — 재시도 로직이 있는 실시간 Webhook 전달 — 을 구축하는 완전한 예제입니다.

---

## 이 워크플로우를 사용하는 경우

다음과 같을 때 `/pi` → `/plan` → `/run`을 사용하세요:

- 기능이 10개 이상의 파일에 영향을 미침
- 아키텍처 결정이 필요함 (DB 스키마, 재시도 전략, 큐 설계)
- 여러 엔지니어(또는 Worker)가 병렬로 작업함
- 코드 작성 전에 스펙 문서가 필요함

더 작은 태스크에는 `/quick`을 사용하세요.

---

## 기능: Webhook 전달 시스템

**목표**: 이벤트가 발행될 때 (예: `job.completed`), 시스템이 등록된 모든 Webhook URL에 지수적 백오프 재시도(최대 5회, 재시도 간 2^n초)로 전달해야 합니다.

---

## 1단계: /pi로 아이디어 탐색

계획에 확정하기 전에 `/pi`로 브레인스토밍하고 설계 옵션을 검토합니다:

```
/pi "재시도가 있는 Webhook 전달 시스템"
```

CQ가 Play Idea 모드에 진입합니다 — 저비용 탐색 단계입니다. 명확화 질문을 하고 옵션을 제시합니다:

```
[PI] 탐색 중: 재시도가 있는 Webhook 전달 시스템

범위 명확화를 위한 질문:
  1. 전달 순서: 엄격한 순서(FIFO) 또는 최선형?
  2. 재시도 저장소: 메모리 큐 또는 영속적 (SQLite/Postgres)?
  3. 실패 알림: 최종 실패 시 호출자에게 알려야 하나요?
  4. 동시성: 엔드포인트당 최대 병렬 전달 수?

설계 옵션:
  옵션 A: 인프로세스 goroutine 풀 + SQLite 재시도 테이블
    장점: 인프라 의존성 없음, 간단한 운영
    단점: 큐가 플러시되지 않으면 충돌 시 손실

  옵션 B: 별도의 webhook-worker 프로세스 + Redis 큐
    장점: 내구성, 확장성
    단점: Redis 필요, 복잡한 구성

  옵션 C: 이벤트 디스패처에 내장된 재시도 루프
    장점: 최소한의 코드, 새 테이블 없음
    단점: 디스패처 goroutine 차단

추천: v1에는 옵션 A (현재 SQLite 아키텍처에 적합)
```

질문에 답합니다:

```
1. v1에서는 최선형으로 충분
2. 영속적 — SQLite, 나머지 시스템과 일관성 유지
3. 예, webhook.failed 이벤트 발생
4. 엔드포인트당 5개 동시
```

CQ가 합의된 설계를 요약하고 계획으로 이동할 것을 제안합니다:

```
[PI] 계획 준비됨. /plan을 실행하여 태스크 생성.
```

---

## 2단계: /plan으로 태스크 생성

```
/plan "Webhook 전달: SQLite 기반 재시도, 5회 시도, 지수 백오프, 소진 시 webhook.failed 이벤트"
```

CQ가 Discovery → Design → 태스크 생성을 실행합니다:

```
[DISCOVERY] 도메인: go backend
요구사항 수집 (EARS 형식):
  - job.completed 발생 시, 시스템이 등록된 모든 hook에 전달해야 함
  - 전달 실패 시, 시스템이 2^n초 지연으로 재시도해야 함 (최대 5회)
  - 모든 재시도 소진 시, 시스템이 webhook.failed 이벤트를 발생시켜야 함
  - 시스템이 엔드포인트당 최대 5개의 동시 전달을 지원해야 함

[DESIGN] 아키텍처 결정 기록 (ADR-014):
  저장소: webhooks 테이블 (id, url, event_type, created_at)
           webhook_deliveries 테이블 (id, webhook_id, payload, attempt, status, next_retry)
  Worker: WebhookWorker goroutine 풀 (풀 크기 = 엔드포인트당 5)
  재시도: 지수 백오프 1s, 2s, 4s, 8s, 16s

[TASKS] 6개 태스크 생성:
  T-WH-01  DB 스키마: webhooks + webhook_deliveries 테이블
  T-WH-02  WebhookStore: CRUD + pending-deliveries 쿼리
  T-WH-03  WebhookWorker: goroutine 풀 + HTTP 전달
  T-WH-04  RetryScheduler: 백오프 + 재큐 로직
  T-WH-05  EventBus 통합: job.completed 구독 → 큐에 추가
  T-WH-06  테스트: 단위 + 통합
  CP-WH    체크포인트: 모든 태스크 + 통합 테스트 통과

의존성:
  T-WH-02는 T-WH-01 필요
  T-WH-03는 T-WH-02 필요
  T-WH-04는 T-WH-03 필요
  T-WH-05는 T-WH-03 필요
  T-WH-06는 T-WH-03, T-WH-04, T-WH-05 필요
  CP-WH는 T-WH-06 필요
```

태스크 목록 검토:

```
/status
```

```
State: PLAN → EXECUTE
Queue: 6 pending | 0 in_progress | 0 done
지금 준비: T-WH-01 (의존성 없음)
차단됨: T-WH-02..06 (의존성 대기 중)
```

---

## 3단계: /run으로 Worker 실행

Worker를 시작합니다. T-WH-01에 의존성이 없으므로 하나의 Worker가 즉시 시작할 수 있습니다:

```
/run 3
```

```
3개의 Worker 스폰 중...

Worker-1 클레임: T-WH-01  DB 스키마: webhooks + webhook_deliveries 테이블
Worker-2: 준비된 태스크 없음, 대기 중...
Worker-3: 준비된 태스크 없음, 대기 중...
```

Worker-1이 마이그레이션을 생성합니다:

```sql
-- infra/supabase/migrations/00060_webhooks.sql
CREATE TABLE webhooks (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    url         TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    secret      TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id  TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    payload     JSONB NOT NULL,
    attempt     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | delivered | failed
    next_retry  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_deliveries_pending
    ON webhook_deliveries(status, next_retry)
    WHERE status = 'pending';
```

Worker-1이 T-WH-01을 제출합니다. T-WH-02가 준비됩니다.

```
Worker-1이 T-WH-01 제출
Worker-2 클레임: T-WH-02  WebhookStore: CRUD + pending-deliveries 쿼리
```

Worker들이 의존성이 해결되면서 계속 태스크를 가져갑니다. 진행 상황 확인:

```
/status
```

```
State: EXECUTE
Queue: 2 pending | 3 in_progress | 1 done
  T-WH-01  [done]         DB 스키마
  T-WH-02  [in_progress]  WebhookStore
  T-WH-03  [in_progress]  WebhookWorker
  T-WH-04  [pending]      RetryScheduler    (T-WH-03에 의해 차단됨)
  T-WH-05  [pending]      EventBus 통합 (T-WH-03에 의해 차단됨)
  T-WH-06  [in_progress]  테스트
  CP-WH    [pending]      체크포인트
```

---

## 4단계: 체크포인트 리뷰

모든 태스크가 완료되면 CQ가 자동으로 CHECKPOINT 상태에 진입합니다:

```
/status
```

```
State: CHECKPOINT
Checkpoint: CP-WH — 6개 태스크 완료, 리뷰 대기 중
```

리뷰 트리거:

```
/checkpoint
```

```
[CHECKPOINT] CP-WH 리뷰 중...

변경사항 (6개 파일, +842줄, -12줄):
  infra/supabase/migrations/00060_webhooks.sql  (+38)
  internal/store/webhook_store.go               (+180)
  internal/worker/webhook_worker.go             (+210)
  internal/scheduler/retry_scheduler.go         (+95)
  internal/eventbus/dispatcher.go               (+45, -12)
  internal/worker/webhook_worker_test.go        (+274)

유효성 검사:
  go-build:       PASS
  go-vet:         PASS
  go-test:        PASS  (87개 테스트, 1.4s)
  integration:    PASS  (3개 테스트, 2.1s)

리뷰 결과: APPROVE

메모:
  - WebhookWorker가 엔드포인트별 동시성에 세마포어 올바르게 사용 (좋음)
  - RetryScheduler가 time.AfterFunc 사용 — 대형 큐에서는 향후 ticker 고려
  - maxAttempts에서 webhook.failed 이벤트 올바르게 발생
```

---

## 5단계: 마무리

```
/finish
```

```
[FINISH] 다듬는 중...
  - 체인지로그 항목 생성
  - 최종 유효성 검사 실행
  - 모든 검사 통과

State: COMPLETE

요약: Webhook 전달 시스템
  - 6개 태스크 완료
  - 842줄 추가
  - 87개 단위 + 3개 통합 테스트 통과
  - 릴리즈 준비 완료
```

---

## 각 단계가 하는 일

| 단계 | 커맨드 | 목적 |
|------|--------|------|
| 탐색 | `/pi "아이디어"` | 확정 전 옵션 브레인스토밍, 제약사항 명확화 |
| 계획 | `/plan "스펙"` | 스펙, 설계 결정(ADR), 태스크 큐 생성 |
| 실행 | `/run N` | N개의 Worker 스폰; 의존성 해결되면 태스크 처리 |
| 리뷰 | `/checkpoint` | 관리자가 모든 변경사항 검토; 승인 또는 변경 요청 |
| 마무리 | `/finish` | 다듬기, 체인지로그, 최종 유효성 검사 |

---

## 팁

**Worker 수 조정**: `/run 3`이 좋은 기본값입니다. 독립적인 태스크가 많으면 Worker가 더 많을수록 도움이 됩니다. 선형 의존성 체인(A → B → C)에서는 추가 Worker가 유휴 상태가 됩니다.

**체크포인트에서 변경 요청 시**: CQ가 새 태스크를 생성하고 EXECUTE로 돌아갑니다. Worker들이 자동으로 처리합니다 — 기다리거나 `/run`을 다시 실행하세요.

**스펙은 보존됩니다**: `/plan`의 스펙과 설계는 `.c4/specs/`와 `.c4/designs/`에 저장됩니다. `c4_get_spec` 또는 `c4_get_design`으로 언제든지 참조할 수 있습니다.

---

## 다음 단계

- **GPU 워크로드**: [연구 루프](research-loop.md)
- **연구자 워크플로우**: [연구 루프](research-loop.md)
- **전체 워크플로우 레퍼런스**: [사용 가이드](../reference/commands.md)

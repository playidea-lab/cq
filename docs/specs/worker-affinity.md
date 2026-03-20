feature: worker-affinity
domain: go
description: |
  Hub 워커에 이력 기반 친화도(affinity) 자동 라우팅.
  job 성공 이력이 쌓이면 같은 종류의 job이 자동으로 해당 워커에 우선 할당.

requirements:
  - id: REQ-001
    pattern: event-driven
    text: "WHEN job이 성공적으로 완료되면 THEN worker_affinity 테이블에 이력이 갱신된다"
  - id: REQ-002
    pattern: event-driven
    text: "WHEN 새 job이 도착하면 THEN affinity_score가 가장 높은 가용 워커에 우선 할당된다"
  - id: REQ-003
    pattern: optional
    text: "IF 모든 가용 워커의 score가 0이면 THEN 기존 FIFO로 fallback"
  - id: REQ-004
    pattern: optional
    text: "IF --worker 플래그 지정 시 THEN 친화도 무시하고 직접 할당"

## 동작 시나리오

### S1: 첫 실행 (cold start)
- WHEN: 이력 없는 워커 2대에 HMR job 제출
- THEN: 아무 워커에 할당 (기존 FIFO)
- VERIFY:
  - affinity score 계산 결과 모두 0
  - job이 정상 할당됨

### S2: 이력 축적 후 우선 할당
- WHEN: 워커 A가 HMR job 3건 성공 후 새 HMR job 도착
- THEN: 워커 A에 우선 할당 (워커 B보다 score 높음)
- VERIFY:
  - worker_affinity에 (A, hmr, success=3) 레코드 존재
  - Score(A, hmr) > Score(B, hmr)
  - 할당된 worker_id = A

### S3: 실패 이력 반영
- WHEN: 워커 A가 HMR job 1건 실패 후 새 HMR job 도착
- THEN: 성공률이 반영되어 score 감소
- VERIFY:
  - worker_affinity에 fail_count >= 1
  - Score 계산 시 success_rate 반영

### S4: target_worker 수동 지정
- WHEN: --worker B 플래그로 job 제출 (A의 affinity가 더 높음)
- THEN: 워커 B에 직접 할당, A 무시
- VERIFY:
  - 할당된 worker_id = B
  - A는 이 job을 받지 않음

### S5: recency bonus
- WHEN: 워커 A가 30일 전 HMR 성공, 워커 B가 어제 HMR 성공 (둘 다 1건)
- THEN: 워커 B가 우선 (recency bonus)
- VERIFY:
  - Score(B) > Score(A)
  - B의 recency_bonus > 0, A의 recency_bonus = 0

### S6: cq hub workers 친화도 표시
- WHEN: cq hub workers 실행
- THEN: 각 워커 행에 AFFINITY 컬럼 표시
- VERIFY:
  - 출력에 "hmr(3✓)" 형식 포함
  - 이력 없는 워커는 "(none)" 표시

## 테스트 매핑
| 시나리오 | 테스트 | 상태 |
|---------|--------|------|
| S1 | (구현 후 자동 매핑) | ⏳ |
| S2 | (구현 후 자동 매핑) | ⏳ |
| S3 | (구현 후 자동 매핑) | ⏳ |
| S4 | (구현 후 자동 매핑) | ⏳ |
| S5 | (구현 후 자동 매핑) | ⏳ |
| S6 | (구현 후 자동 매핑) | ⏳ |

# C4 for Codex Playbook

이 디렉토리는 Codex CLI에서 C4를 안정적으로 운용하기 위한 실행 규약입니다.

## 기본 원칙

- 같은 태스크에 `Direct`와 `Worker` 프로토콜을 섞지 않습니다.
- Direct: `c4_claim -> c4_report`
- Worker: `c4_get_task -> c4_submit`
- 제출/보고 전에는 항상 `c4_run_validation`을 먼저 실행합니다.
- 최종 판단은 항상 `c4_status` 숫자 기준으로 합니다.

## 시나리오 선택

1. Direct 우선
- 조건: 다중 파일 결합, 리팩토링, 마이그레이션, 횡단 수정
- 절차: `c4_status -> c4_claim -> 구현 -> c4_run_validation -> commit -> c4_report -> c4_status`

2. Worker 병렬
- 조건: 독립 scope 태스크 다수
- 절차: `c4_status -> (필요 시 c4_start) -> c4_get_task 반복 -> c4_submit`

3. Hybrid
- 낮은 결합 구간은 Worker로 소모
- 결합 높은 구간은 Direct로 전환
- 게이트는 `c4_checkpoint`로 닫기

## 실행 리허설

1. Direct 리허설
- 시작 입력: `c4 status 보여주고 direct로 시작해`
- 기대 분기: `c4-status -> c4-direct -> c4-claim -> c4-report`

2. Worker 리허설
- 시작 입력: `독립 태스크 병렬로 run`
- 기대 분기: `c4-status -> c4-run -> c4-submit`

3. Hybrid 리허설
- 시작 입력: `먼저 병렬 처리하고 결합 구간은 direct로 전환`
- 기대 분기: `c4-run` 반복 후 `c4-direct` 전환, 마지막 `c4-checkpoint`

## 자주 발생하는 실패

- `Task ... is claimed by direct mode — use c4_report`
  - 원인: Direct 태스크를 submit함
  - 조치: `c4_report`로 전환

- `Task ... is ... (expected in_progress)`
  - 원인: 상태 불일치
  - 조치: `c4_status` 재조회 후 태스크 재선정

- `Task ... is owned by worker ...`
  - 원인: owner 불일치
  - 조치: 해당 worker만 submit, 현재 세션은 다른 태스크 진행

## 참고

- 상세 에이전트 규칙: `.codex/agents/README.md`
- Hosted Worker 서비스화 가이드: `.codex/hosted-worker/README.md`
- 가격 계산기: `.codex/tools/hosted_pricing.py`

# C4 Codex Agents

Codex CLI용 C4 에이전트 모음입니다. 기본 구현 경로는 Worker이며, Direct는 `execution_mode=direct` 태스크에 한해 사용합니다.

## 라우팅 규칙

1. `state == CHECKPOINT`
- `c4-checkpoint` 사용

2. `ready_tasks == 0`이고 `pending_tasks > 0`
- 의존성 대기 상태. `c4-status`로 대기 이유 보고

3. Direct 전용 태스크 (`execution_mode=direct`)
- `c4-direct` 사용
- 보조: `c4-claim`, `c4-report`

4. 구현 태스크 실행 (기본)
- `c4-run` 사용
- 보조: `c4-submit`, `c4-validate`

5. 외부 프로세스/서비스 워커
- `c4-hosted-worker` 사용

## 프로토콜 불변조건

- Direct 태스크는 `c4_submit` 금지
- Worker 태스크는 `c4_report` 금지
- `validation_results`는 `c4_submit` 스키마(`name/status/message`)로 정규화 후 전송

## 포함된 에이전트

- `c4-status`: 상태/큐/ready 태스크 리포트 + 라우팅
- `c4-plan`: Discovery -> Design -> Tasking
- `c4-run`: Worker 루프 실행
- `c4-hosted-worker`: Hosted Worker(외부 프로세스) 실행 루프
- `c4-direct`: Direct 모드 end-to-end
- `c4-claim`: Direct claim 전용
- `c4-report`: Direct report 전용
- `c4-submit`: Worker submit 전용
- `c4-validate`: 검증 실행/요약
- `c4-add-task`: 태스크 등록 (`worker`/`direct`)
- `c4-checkpoint`: 체크포인트 의사결정
- `c4-clear`: 상태 초기화

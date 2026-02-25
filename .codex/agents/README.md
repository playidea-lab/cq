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

6. 품질 수렴/마무리
- `c4-polish` -> `c4-finish` 순서 사용
- 구형 진입점 `c4-refine`은 `c4-polish`로 라우팅

7. 유틸리티/운영
- 빠른 시작: `c4-quick`
- 실행 중지: `c4-stop`
- 병렬 확장: `c4-swarm`, `c4-standby` (Hub 활성 + worker 도구 등록 시)
- 릴리스/가이드: `c4-release`, `c4-help`

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
- `c4-quick`: 생성+할당 빠른 시작
- `c4-polish`: build/test/review 반복 정제
- `c4-refine`: deprecated 라우터 (`c4-polish`로 전환)
- `c4-finish`: gate 확인 후 마무리 루틴
- `c4-stop`: EXECUTE -> HALTED 전환
- `c4-swarm`: 다중 워커 병렬 배치
- `c4-standby`: Hub 상주 워커 루프
- `c4-release`: 릴리스 노트/버전 판단
- `c4-review`: 3-pass 리뷰 실행
- `c4-interview`: 요구사항 심층 인터뷰
- `c4-init`: Codex 초기화 진입점
- `c4-reboot`: 세션 재시작 절차
- `c4-attach`: 세션 이름 연결
- `c4-help`: 상태 기반 명령 라우팅 도움말

# Codex-Claude 패리티 매트릭스

## 목적
- Claude Code Skill 기반 워크플로우와 Codex Agent 기반 워크플로우를 동일한 C4 프로토콜 관점에서 비교한다.
- 구현 우선순위를 `Parity Baseline`(필수), `Coverage`(범위 확장), `Codex Plus`(강화)로 분리한다.

## 용어
- `Parity Baseline`: 동일 상태 전이/도구 프로토콜을 반드시 만족해야 하는 항목
- `Coverage`: Claude에 있는 기능 의도를 Codex에도 제공해야 하는 항목
- `Codex Plus`: 동등화 이후 Codex에서 추가로 강화하는 항목

## 매핑표

| Claude Skill | Codex Agent | 구분 | 현재 상태 | 수용 기준(DoD) |
|---|---|---|---|---|
| `c4-plan` | `c4-plan` | Parity Baseline | 부분 | Discovery/Design/Tasking 흐름과 Worker-first 규칙 명시 |
| `c4-run` | `c4-run` | Parity Baseline | 부분 | 상태 분기 + worker loop + submit 가드 + checkpoint 종료 |
| `c4-status` | `c4-status` | Parity Baseline | 구현 | 상태/큐/ready 보고 + 라우팅 제공 |
| `c4-checkpoint` | `c4-checkpoint` | Parity Baseline | 구현 | checkpoint 의사결정 흐름 수행 |
| `c4-submit` | `c4-submit` | Parity Baseline | 구현 | worker submit 스키마/owner 규칙 준수 |
| `c4-validate` | `c4-validate` | Parity Baseline | 구현 | 검증 실행 및 요약 출력 |
| `c4-add-task` | `c4-add-task` | Parity Baseline | 구현 | todo 생성 입력/모드 분기 지원 |
| `c4-clear` | `c4-clear` | Parity Baseline | 구현 | 상태 초기화 가이드 제공 |
| `c4-stop` | `(없음)` | Coverage | 미구현 | EXECUTE -> HALTED 중지 흐름 Codex 제공 |
| `c4-init` | `(없음)` | Coverage | 미구현 | init/bootstrap 절차를 Codex 트리거로 제공 |
| `c4-help` | `(없음)` | Coverage | 미구현 | Codex 사용자용 명령 안내 엔트리 제공 |
| `c4-quick` | `(없음)` | Coverage | 미구현 | 빠른 시작/즉시 실행 템플릿 제공 |
| `c4-review` | `(없음)` | Coverage | 미구현 | 리뷰 중심 점검 루프 제공 |
| `c4-standby` | `(없음)` | Coverage | 미구현 | standby worker 흐름 제공 |
| `c4-swarm` | `(없음)` | Coverage | 미구현 | 병렬 작업 오케스트레이션 제공 |
| `c4-release` | `(없음)` | Coverage | 미구현 | 릴리스 점검/실행 흐름 제공 |
| `finish` | `(없음)` | Coverage | 미구현 | build/test/install/docs/commit 루틴 제공 |
| `research-loop` | `(없음)` | Coverage | 미구현 | 연구 반복 루프 Codex 대응 |
| `c2-paper-review` | `(없음)` | Coverage | 미구현 | C2 논문/문서 리뷰 흐름 Codex 대응 |
| `c4-interview` | `(없음)` | Coverage | 미구현 | 요구사항 인터뷰 보조 흐름 Codex 대응 여부 결정 |

## Parity Baseline 범위
- 대상: `plan`, `run`, `status`, `checkpoint`, `validate`, `submit`, `add-task`, `clear`
- 필수 정책:
  - 구현 태스크는 Worker-first (`get_task -> submit`)
  - Direct는 `execution_mode=direct`에서만 `claim/report`
  - `commit_sha` 없는 submit 금지
  - CHECKPOINT/COMPLETE 상태에서 구현 루프 중단

## Coverage 범위
- 대상: Claude에 존재하지만 Codex 미구현인 lifecycle/advanced/domain 의도
- 산출물:
  - 누락 의도별 agent 파일
  - 각 agent에 상태 전이/프로토콜/출력 체크리스트 포함

## Codex Plus 범위
- 예시:
  - pre-flight 자동 점검(상태/ready/dependency)
  - submit 전 커밋/변경 검증 가드
  - 작업 종료 시 운영 요약 자동 생성
- 원칙: Parity 동작을 깨지 않는 범위에서만 확장

## 의존성 기준
- `T-530-0` 완료 후 다음 구현 태스크(`T-531-0` ~)가 ready 되어야 한다.
- 의존성 큐에서 root task 완료 전 하위 task는 실행되지 않아야 한다.


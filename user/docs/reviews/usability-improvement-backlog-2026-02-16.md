# C4 사용성 개선 백로그 (일괄 반영용)

> 작성일: 2026-02-16  
> 상태: Draft (반영 대기)  
> 목적: 지금은 수정하지 않고, 추후 한 번에 반영할 수 있도록 개선 항목을 구조화한다.

## 1. 문서 목적

이 문서는 C4 프로젝트의 사용성 평가 결과를 "즉시 수정"이 아니라 "일괄 반영" 기준으로 정리한 실행 백로그다.

핵심 목표:
1. 사용자 혼란을 유발하는 문서/스킬/실제 동작 간 불일치를 명시한다.
2. 각 이슈를 우선순위, 영향, 근거, 개선안, 수용기준으로 정의한다.
3. 나중에 한 번에 반영할 때 바로 실행 가능한 순서와 검증 항목을 제공한다.

## 2. 평가 범위

검토 범위:
- 루트 문서: `README.md`, `AGENTS.md`
- 시작/가이드: `docs/getting-started/*`, `docs/usage-guide.md`, `docs/user-guide/*`
- 실행 스킬: `.claude/skills/run/SKILL.md`, `.claude/skills/submit/SKILL.md`, `.claude/skills/plan/SKILL.md`
- 실제 계약 참조: `c4-core/internal/store/types.go`, `c4_status` 실출력

검토 제외:
- 실제 기능 구현 변경
- DB 스키마 변경
- 런타임 로직 리팩토링

## 3. 기준 스냅샷 (2026-02-16)

현재 `c4_status` 출력(요약):
- `state`, `pending_tasks`, `ready_tasks`, `ready_task_ids`, `workers[]` 구조
- `queue`, `parallelism`, `status` 필드는 확인되지 않음

정책 기준(요약):
- Worker-first 구현 원칙
- Task tracking SSOT: `.c4/tasks.db` + MCP API
- direct DB 업데이트 금지
- `PLAN.md`/`TODO.md` 등 임의 생성 금지

## 4. 우선순위 기준

- `P0`: 실행 실패/잘못된 운영 유도 가능 (즉시 정합성 확보 필요)
- `P1`: 온보딩 실패/심각한 혼란 유발
- `P2`: 신뢰성 저하/학습 비용 증가
- `P3`: 유지보수성 저하/재발 방지 자동화 필요

## 5. 개선 백로그 상세

### UXR-001 (P0) `/run` 상태 스키마 계약 불일치

증상:
- 스킬 문서가 `status["parallelism"]`, `status["queue"]["pending"]`, `status["status"]`를 가정함.
- 실제 상태 구조는 `state`, `pending_tasks`, `workers[]` 중심임.

사용자 영향:
- 자동 실행/상태 분기 로직이 오작동하거나 예외를 유발할 수 있음.
- "왜 /run이 문서대로 안 되지?"라는 신뢰 하락을 야기함.

근거:
- `.claude/skills/run/SKILL.md:85`
- `.claude/skills/run/SKILL.md:260`
- `.claude/skills/run/SKILL.md:262`
- `c4-core/internal/store/types.go:107`
- `c4-core/internal/store/types.go:110`
- `c4-core/internal/store/types.go:117`

개선안:
1. `/run` 스킬의 상태 참조 키를 실제 `c4_status` 계약으로 교체.
2. 상태 파싱 실패 시 명시적 에러 메시지와 fallback 경로 제공.
3. 스킬 문서 예시 JSON을 실제 응답 스키마 기준으로 갱신.

수용기준:
1. `/run` 문서/스킬에서 `parallelism`, `queue.pending`, `status` 가정 제거.
2. `state` 기반 분기(`PLAN/HALTED/EXECUTE/CHECKPOINT/COMPLETE`)가 일관되게 문서화됨.
3. 최소 1개 상태 샘플이 실제 출력과 동일 필드명으로 제시됨.

---

### UXR-002 (P0) `/submit` 인자 계약 불일치

증상:
- 명령어 레퍼런스는 `/submit <task_id> <commit_sha> [validation_results]`를 안내.
- 스킬 문서는 `/submit [task-id]` 및 task 자동 탐지 흐름을 안내.

사용자 영향:
- 호출 방식 혼선으로 제출 실패 가능.
- 작업 완료 보고 절차(커밋 SHA 포함)에 대한 신뢰 저하.

근거:
- `docs/user-guide/명령어-레퍼런스.md:230`
- `.claude/skills/submit/SKILL.md:17`
- `.claude/skills/submit/SKILL.md:20`
- `.claude/skills/submit/SKILL.md:199`

개선안:
1. `/submit` 사용자 입력 계약을 단일 규격으로 통일.
2. "사용자 입력"과 "스킬 내부 처리(commit_sha 수집)"를 분리 표기.
3. 실패 케이스(인자 누락, in_progress 아님) 에러 예시 추가.

수용기준:
1. 명령 레퍼런스와 스킬 문서가 동일한 호출 계약을 사용.
2. 커밋 SHA 확보 책임 주체(사용자 vs 스킬)가 명확히 기재.
3. 제출 전 상태 확인 절차가 문서에 명시됨.

---

### UXR-003 (P1) 설치/트러블슈팅 경로 혼선 (Python 모듈 vs Go 바이너리)

증상:
- 일부 문서는 MCP 실행 경로를 `python -m c4.mcp_server`로 안내.
- 설치 가이드는 Go 바이너리(`c4-core/bin/cq` 또는 `~/.local/bin/cq`) 기준.

사용자 영향:
- 설치 후 "MCP server not found" 해결 과정에서 잘못된 경로를 따라갈 위험.
- 신규 사용자의 첫 실행 성공률 저하.

근거:
- `docs/user-guide/문제-해결.md:18`
- `docs/getting-started/설치-가이드.md:105`
- `docs/getting-started/설치-가이드.md:125`

개선안:
1. 공식 실행 경로를 Go 바이너리 기준으로 단일화.
2. Python 모듈 실행법은 "레거시/개발용" 섹션으로 분리하거나 제거.
3. 문제 해결 문서에 `.mcp.json`/`~/.claude.json` 점검 순서를 표준화.

수용기준:
1. 사용자 가이드 전체에서 기본 MCP 실행 경로가 하나로 통일.
2. 설치 문서와 트러블슈팅 문서의 샘플 설정이 동일.
3. "재시작 필요 시점" 안내가 모든 경로에서 일관됨.

---

### UXR-004 (P1) Task/Plan SSOT 충돌 (`tasks.json`, `docs/PLAN.md` vs `tasks.db`, `c4_add_todo`)

증상:
- 명령어 레퍼런스는 `tasks.json`과 `docs/PLAN.md` 파싱 중심으로 설명.
- 정책/실구현은 `.c4/tasks.db`와 `c4_add_todo` 중심.

사용자 영향:
- 사용자는 어떤 저장소/프로세스를 따라야 하는지 판단하기 어려움.
- 잘못된 파일 편집으로 운영 상태와 문서가 분리될 수 있음.

근거:
- `docs/user-guide/명령어-레퍼런스.md:39`
- `docs/user-guide/명령어-레퍼런스.md:60`
- `c4-core/cmd/c4/add_task.go:25`
- `AGENTS.md:135`
- `AGENTS.md:136`

개선안:
1. "실행 SSOT"와 "설명용 문서"를 분리 표기.
2. `tasks.json`, `docs/PLAN.md` 중심 문구를 최신 워크플로우 기준으로 정정.
3. `/plan` 설명을 c4_save_spec/design/add_todo 중심으로 업데이트.

수용기준:
1. Task tracking 저장소 설명이 전 문서에서 동일(`.c4/tasks.db`).
2. `PLAN.md` 생성/파싱이 필수인 것처럼 오해되는 문구 제거.
3. 실제 도구 호출 경로와 문서 절차가 1:1 매칭됨.

---

### UXR-005 (P1) 트러블슈팅에서 DB 직접 조작 유도

증상:
- 문서가 `sqlite3 ... DELETE FROM c4_locks` 같은 직접 수정 방법을 안내.
- 정책은 MCP API만 사용, direct DB 업데이트 금지.

사용자 영향:
- 데이터 무결성/운영 추적성 손상 가능.
- 운영 중 상태 꼬임 시 복구 난이도 상승.

근거:
- `docs/user-guide/문제-해결.md:124`
- `AGENTS.md:226`

개선안:
1. DB 직접 조작 항목을 "긴급/최후 수단"으로 격하하고 경고 강화.
2. 우선 순위를 MCP 복구 절차(`c4_status`, `c4_get_task`, `c4_mark_blocked` 등)로 재정렬.
3. 가능한 경우 전용 복구 도구를 추가하고 문서화.

수용기준:
1. 일반 사용자 경로에서 DB 직접 SQL 명령이 기본 해법으로 노출되지 않음.
2. 모든 복구 단계에서 MCP API 우선 원칙이 보장됨.
3. 위험 작업은 백업/롤백 절차와 함께 표기됨.

---

### UXR-006 (P2) 도구 수/구성 수치 불일치 (103/108/112/134/152)

증상:
- 문서마다 도구 수/분류 수치가 다르게 표기됨.

사용자 영향:
- 문서 신뢰도 하락.
- 버전/구성 상태에 대한 혼란 증가.

근거:
- `docs/usage-guide.md:3`
- `docs/getting-started/설치-가이드.md:5`
- `README.md:19`
- `AGENTS.md:246`

개선안:
1. 도구 수는 정적 하드코딩 대신 "버전/구성 조건" 표기.
2. 단일 생성 소스(예: 자동 스크립트)로 문서 수치 동기화.
3. Hub enabled/disabled 조건별 숫자 표를 표준 템플릿으로 통일.

수용기준:
1. 도구 수 표기가 문서 전체에서 모순 없이 설명됨.
2. 숫자 옆에 집계 기준(기본/Hub 포함/옵션 포함)이 명시됨.
3. 버전 갱신 시 자동 동기화 절차가 문서화됨.

---

### UXR-007 (P2) 워크플로우 상태 모델 표현 불일치

증상:
- 일부 문서는 `INIT -> DISCOVERY -> DESIGN -> PLAN` 흐름을 강조.
- 일부 문서는 `INIT -> PLAN` 전이로 설명.

사용자 영향:
- 현재 상태에서 가능한 명령을 판단하기 어려움.
- "내 상태가 왜 문서 다이어그램과 다르지?" 문제 발생.

근거:
- `README.md:41`
- `docs/getting-started/빠른-시작.md:121`
- `docs/user-guide/워크플로우-개요.md:13`

개선안:
1. 상태 머신 "논리 상태"와 "계획 단계(Discovery/Design)"를 분리 모델로 문서화.
2. 모든 문서의 다이어그램을 동일한 베이스 템플릿으로 교체.
3. 상태별 허용 명령표를 단일 참조로 연결.

수용기준:
1. 상태 다이어그램이 문서군에서 동일한 의미 체계를 사용.
2. Discovery/Design이 상태인지 단계인지 명확히 설명됨.
3. 명령어 레퍼런스와 상태 개요 문서 간 충돌이 없음.

---

### UXR-008 (P2) Worker-first 원칙과 Direct 사용 경계 설명 부족

증상:
- 일부 문서는 Direct 경로를 일반 옵션처럼 제시.
- 정책 문서는 구현 태스크는 Worker 우선 원칙을 강조.

사용자 영향:
- 상황별 실행 모드 선택 기준이 모호해짐.
- 팀 운영 시 프로세스 일관성 저하.

근거:
- `docs/usage-guide.md:22`
- `AGENTS.md:166`
- `docs/user-guide/Codex-가이드.md:38`

개선안:
1. "기본: Worker", "예외: direct execution_mode 태스크"를 표준 규칙으로 명시.
2. Direct 모드 사용 시 금지/허용 조건을 체크리스트로 추가.
3. `/run`, `/submit`, `c4_claim/c4_report` 선택 트리를 통합.

수용기준:
1. 신규 사용자가 문서만 보고도 모드 선택 오류 없이 실행 가능.
2. Direct 경로는 예외 조건과 함께만 제시됨.
3. Codex/Claude 가이드가 동일한 판단 기준을 공유.

---

### UXR-009 (P3) 계약 문서와 스킬 구현의 동기화 체계 부재

증상:
- 동일 명령의 계약이 문서와 스킬에서 독립 변경됨.
- 변경 후 드리프트를 사전에 감지하는 자동 검증이 없음.

사용자 영향:
- 시간이 지날수록 문서 정확도 악화.
- 유지보수 비용 누적.

개선안:
1. "명령 계약 SSOT" 파일 도입(예: YAML/JSON).
2. 문서/스킬 예시를 SSOT로부터 생성하거나 검증하는 스크립트 도입.
3. CI에 계약 드리프트 체크 추가.

수용기준:
1. 주요 명령(`/run`, `/submit`, `/plan`, `/status`)이 계약 파일 기반으로 검증됨.
2. PR에서 계약 불일치 시 자동 실패.
3. 사람이 수동으로 여러 문서를 동시에 편집하지 않아도 일관성이 유지됨.

---

### UXR-010 (P3) "무엇이 현재 지원되는가"를 한 번에 보여주는 단일 표 부재

증상:
- 사용자 관점의 실행 가능 기능이 여러 문서에 분산됨.
- Codex/Claude 차이, Hub on/off 차이를 빠르게 파악하기 어려움.

사용자 영향:
- 온보딩 시간 증가.
- 잘못된 기대치 설정.

개선안:
1. 단일 "지원 매트릭스" 문서 작성(명령, 상태, 클라이언트, 조건).
2. 설치 가이드/사용 가이드/문제 해결에서 해당 문서로 단일 링크.
3. 릴리스 시 매트릭스 업데이트를 체크리스트에 포함.

수용기준:
1. 사용자 질문("이 명령 여기서 되나요?")에 문서 1개로 답할 수 있음.
2. 지원 여부, 전제조건, 대체 경로가 한 화면에서 확인 가능.

## 6. 우선순위 요약표

| ID | Priority | 난이도 | 영향도 | 일괄 반영 권장 순서 |
|---|---|---:|---:|---:|
| UXR-001 | P0 | 중 | 매우 높음 | 1 |
| UXR-002 | P0 | 중 | 매우 높음 | 2 |
| UXR-003 | P1 | 중 | 높음 | 3 |
| UXR-004 | P1 | 중 | 높음 | 4 |
| UXR-005 | P1 | 중 | 높음 | 5 |
| UXR-006 | P2 | 하 | 중 | 6 |
| UXR-007 | P2 | 중 | 중 | 7 |
| UXR-008 | P2 | 하 | 중 | 8 |
| UXR-009 | P3 | 상 | 중 | 9 |
| UXR-010 | P3 | 중 | 중 | 10 |

## 7. 일괄 반영 대상 파일(초안)

문서:
- `README.md`
- `docs/getting-started/설치-가이드.md`
- `docs/getting-started/빠른-시작.md`
- `docs/usage-guide.md`
- `docs/user-guide/명령어-레퍼런스.md`
- `docs/user-guide/문제-해결.md`
- `docs/user-guide/워크플로우-개요.md`
- `docs/user-guide/Codex-가이드.md`
- `docs/user-guide/Codex-패리티-매트릭스.md`

스킬:
- `.claude/skills/run/SKILL.md`
- `.claude/skills/submit/SKILL.md`
- `.claude/skills/plan/SKILL.md` (필요 시 계약 링크 정리)

정책 문서 정합성 점검:
- `AGENTS.md`

## 8. 일괄 반영 실행 순서 (제안)

1단계: 계약 고정
1. `c4_status`, `/run`, `/submit` 계약을 단일 기준으로 확정.
2. 상태 키/인자/오류 코드/필수 전제조건을 표준 표로 정의.

2단계: 스킬 정합화
1. `.claude/skills` 문서를 기준 계약에 맞게 정리.
2. 예시 코드의 필드명/분기/에러 처리 갱신.

3단계: 사용자 문서 일괄 정리
1. 설치/빠른시작/명령어/FAQ를 순서대로 정합화.
2. 중복/레거시 경로를 하나의 권장 경로로 수렴.

4단계: 자동 검증 도입
1. 계약 드리프트 검사 스크립트 추가.
2. CI에서 문서-스킬-계약 정합성 체크.

## 9. 검증 체크리스트 (반영 시 사용)

문서 검증:
1. 동일 명령이 모든 문서에서 같은 인자 규칙을 갖는가?
2. 상태명/필드명이 실제 API와 일치하는가?
3. 설치 경로가 단일 표준으로 안내되는가?
4. 위험 작업(DB 직접 수정)이 기본 경로에서 제거되었는가?

워크플로우 검증:
1. 신규 사용자가 설치 문서만으로 `c4_status`까지 도달 가능한가?
2. `PLAN/HALTED/EXECUTE/CHECKPOINT/COMPLETE` 분기가 혼동 없이 설명되는가?
3. Worker-first와 Direct 예외 규칙이 명확한가?

운영 검증:
1. 문서 수치(도구 수/조건)가 버전/구성별로 설명되는가?
2. 변경 후 FAQ가 구버전 실행 경로를 재노출하지 않는가?

## 10. 반영 완료 정의 (Done Definition)

다음 조건을 모두 만족하면 본 백로그를 "완료"로 전환:
1. UXR-001~UXR-005 해결 완료(P0/P1 클로즈).
2. 핵심 명령 계약(`run/submit/status/plan`) 정합성 검증 자동화 도입.
3. 사용자 문서 주요 8개 파일 정합성 리뷰 통과.
4. 샘플 온보딩 시나리오 2개(신규/기존 사용자) 수동 점검 완료.

## 11. 참고 메모

- 본 문서는 구현 반영 문서가 아니라 "일괄 반영 준비 문서"다.
- 실제 반영 시에는 본 문서를 기준으로 체크리스트 기반 PR 묶음으로 진행한다.
- 반영 중 정책 변경이 발생하면, 먼저 본 문서의 우선순위/수용기준을 갱신한 뒤 구현을 진행한다.

## 12. 추가 리뷰 로그 (누적)

### 12.1 라운드 1 (2026-02-16) - 코드 리뷰 + 구조 리뷰

이번 라운드는 핵심 워크플로우 경로를 우선 검토했다:
- 상태 조회: `c4_status` (`state.go`, `store_status.go`)
- 제출/할당: `c4_submit`, `c4_get_task` (`tasks.go`)
- 등록 구조: `register.go`

---

### CR-001 (P1) `GetStatus`의 실패 은닉(부분 성공 반환)

증상:
- `c4_tasks` 집계 쿼리 실패 시 에러를 반환하지 않고 기본/부분 상태를 그대로 반환한다.

사용자 영향:
- 실제 DB 문제를 사용자가 감지하지 못한 채 정상으로 오인할 수 있다.
- 운영 중 "작업이 없는 것처럼 보이는" 허위 정상 상태가 발생할 수 있다.

근거:
- `c4-core/internal/mcp/handlers/store_status.go:33`

개선안:
1. `GetStatus` 핵심 쿼리 실패 시 명시적 에러 반환으로 전환.
2. 부분 성공이 필요하면 `warnings` 필드로 노출하고 실패를 숨기지 않음.
3. 상태 조회 실패를 감지하는 테스트 케이스 추가.

수용기준:
1. 핵심 집계 실패 시 호출자(`c4_status`)가 실패를 인지 가능.
2. 장애 상황에서 "정상 상태처럼 보이는" 응답이 제거됨.

---

### CR-002 (P1) 핸들러 간 에러 전달 방식 불일치 (`c4_start` vs 기타)

증상:
- `c4_start`는 실패 시 `error` 반환 대신 `{success:false}` payload를 반환한다.
- `c4_submit` 등 다수 핸들러는 실패 시 Go error를 반환한다.

사용자 영향:
- 클라이언트 구현이 도구별로 분기 로직을 다르게 가져가야 한다.
- 일반적인 "error channel 우선" 처리기에서 실패를 놓칠 수 있다.

근거:
- `c4-core/internal/mcp/handlers/state.go:82`
- `c4-core/internal/mcp/handlers/state.go:85`
- `c4-core/internal/mcp/handlers/tasks.go:229`
- `c4-core/internal/mcp/handlers/tasks.go:244`

개선안:
1. 에러 전달 규약을 통일(권장: transport error + structured payload 보조).
2. 최소한 `c4_start` 문서에 예외 규칙을 명확히 명시.
3. 주요 도구 공통 에러 규약 테스트 도입.

수용기준:
1. 핵심 도구(`start/status/get_task/submit`)의 에러 처리 규약이 일관됨.
2. 문서와 실제 에러 전달 방식이 일치.

---

### CR-003 (P2) `c4_status` 설명-응답 불일치 (queue/workers)

증상:
- 툴 설명은 state/queue/workers를 포함한다고 안내한다.
- 실제 응답은 `queue` 객체가 없고, `workers`도 기본 경로에서 채워지지 않는 형태다.

사용자 영향:
- 문서/스킬이 응답 스키마를 추정하면서 드리프트가 발생하기 쉽다.
- 클라이언트가 존재하지 않는 키(`queue.pending`)를 참조할 가능성이 높다.

근거:
- `c4-core/internal/mcp/handlers/state.go:27`
- `c4-core/internal/mcp/handlers/store_status.go:12`
- `c4-core/internal/mcp/handlers/store_status.go` (workers 채움 로직 부재)
- `c4-core/internal/store/types.go:117`

개선안:
1. 설명을 실제 응답 필드 기준으로 즉시 정정.
2. 필요 시 `workers`/`queue`를 실제로 채우는 구현 추가.
3. 상태 응답 JSON 예시를 단일 SSOT에서 생성.

수용기준:
1. `c4_status` 설명/테스트/문서가 동일한 응답 키를 사용.
2. 스킬 문서에서 존재하지 않는 필드 참조가 사라짐.

---

### CR-004 (P3) `c4_get_task`의 무할당 응답이 모호함

증상:
- 할당 가능한 태스크가 없을 때 빈 객체 `{}`를 반환한다.

사용자 영향:
- 타입 분기(객체 비어있음 vs 할당 객체)를 호출자마다 별도 구현해야 한다.
- 로깅/디버깅 시 상태 해석이 어려워진다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:221`
- `c4-core/internal/mcp/handlers/tasks.go:223`

개선안:
1. 무할당 시 명시적 형태(`{"assigned":false,"reason":"no_ready_task"}`)로 통일.
2. 스키마/문서/테스트에 no-task 응답을 명시.

수용기준:
1. no-task 응답이 모든 클라이언트에서 동일하게 해석 가능.
2. 빈 map 의미를 추측하지 않아도 됨.

---

### AR-001 (P1) 등록 주석의 수동 카운트 드리프트

증상:
- `register.go` 주석의 core tool 수/목록이 최신 등록 목록과 어긋난다.
- 예: tasks 핸들러에 `c4_request_changes`, `c4_task_list`가 존재하지만 core tools 주석 목록에 누락.

사용자 영향:
- 내부 문서/설명값이 빠르게 낡아 신뢰를 떨어뜨린다.
- 이후 외부 문서 숫자 드리프트를 가속한다.

근거:
- `c4-core/internal/mcp/handlers/register.go:13`
- `c4-core/internal/mcp/handlers/tasks.go:154`
- `c4-core/internal/mcp/handlers/tasks.go:190`

개선안:
1. 수동 카운트 주석 제거 또는 자동 생성으로 전환.
2. 등록 툴 수를 런타임/테스트에서 계산해 출력하도록 변경.

수용기준:
1. 소스 내 고정 숫자와 실제 등록 결과 간 불일치가 재발하지 않음.

---

### AR-002 (P1) `InputSchema` 정의의 광범위 중복 (SSOT 부재)

증상:
- 핸들러 패키지에서 `InputSchema: map[string]any{...}` 패턴이 대량 반복된다.
- 이번 스캔 기준 160개 정의가 분산되어 있다.

사용자 영향:
- 스키마 변경 시 누락/불일치 위험이 높다.
- 문서 자동화 없이 수동 관리 비용이 증가한다.

근거:
- `c4-core/internal/mcp/handlers` 내 `InputSchema: map[string]any{` 검색 결과 `count=160`

개선안:
1. 공통 schema builder/정적 계약 파일(JSON/YAML) 도입.
2. 도구 정의/문서/스킬에서 동일 계약을 참조하도록 빌드 파이프라인 구성.
3. 계약 변경 시 CI에서 diff 검증.

수용기준:
1. 스키마 변경이 단일 소스에서 전파됨.
2. 계약 드리프트를 CI가 자동으로 차단함.

---

### AR-003 (P2) 상태 계약 테스트 범위 부족

증상:
- `TestStatusSuccess`는 상태 응답의 일부 필드(`State`, `TotalTasks`)만 검증한다.
- 스키마 키 일관성(`state` vs `status`, queue/workers 존재성) 관련 회귀를 잡지 못한다.

사용자 영향:
- 문서/스킬/코드 간 계약 드리프트가 테스트에서 조기에 검출되지 않는다.

근거:
- `c4-core/internal/mcp/handlers/handlers_test.go:206`
- `c4-core/internal/mcp/handlers/handlers_test.go:220`
- `c4-core/internal/mcp/handlers/handlers_test.go:223`

개선안:
1. `c4_status` 계약 테스트를 스냅샷/필드검증 형태로 확장.
2. 핵심 필드 존재/부재 정책을 테스트로 고정.
3. no-task, start-failure 등 경계 케이스를 계약 테스트에 포함.

수용기준:
1. 응답 키 변경이 발생하면 테스트가 즉시 실패.
2. 문서 계약과 테스트 계약이 동일 파일/생성물에서 파생됨.

---

### 12.2 라운드 1 추가 요약

이번 라운드에서 새롭게 누적된 항목:
1. 코드 레벨: 4건 (`CR-001` ~ `CR-004`)
2. 구조 레벨: 3건 (`AR-001` ~ `AR-003`)
3. 우선 조치 권장: `CR-001`, `CR-002`, `AR-001`, `AR-002`

다음 라운드 추천 범위:
1. `worker_standby`/Hub 경로의 상태 계약 일관성 리뷰
2. `docs/user-guide/*`와 `.claude/skills/*` 간 명령별 계약 diff 리뷰
3. `c4_lighthouse` 기반 계약-문서 자동화 가능성 검토

---

### 12.3 라운드 2 (2026-02-16) - `worker_standby`/Hub 계약 리뷰 (1번)

검토 범위:
1. `c4_worker_standby`, `c4_worker_complete`, `c4_worker_shutdown` 핸들러 계약
2. Hub 클라이언트(lease/register/complete)와 MCP 노출 계층의 일치성
3. 워커 가이드/스킬과 런타임 동작 간 계약 일치성

이번 라운드 신규 누적:
1. 코드 레벨: 3건 (`CR-005` ~ `CR-007`)
2. 구조 레벨: 2건 (`AR-004` ~ `AR-005`)
3. 즉시 반영 권장: `CR-005`, `CR-006`, `CR-007`

### CR-005 (P1) `WorkerDeps` nil 방어 부재로 런타임 panic 가능

현상:
- `handleWorkerStandby`, `handleWorkerComplete`, `handleWorkerShutdown`는 `deps.HubClient`/`deps.ShutdownStore`를 직접 역참조한다.
- `deps` 또는 필수 의존성이 nil일 때 명시적 에러 반환 대신 panic 경로로 진입할 수 있다.

사용자 영향:
- 초기화/연결 실패가 사용자 친화적 오류 메시지로 노출되지 않고 세션 단위 비정상 종료로 이어질 수 있다.
- 복구 지점이 불명확해 워커 운용 안정성이 떨어진다.

근거:
- `c4-core/internal/mcp/handlers/worker_standby.go:77`
- `c4-core/internal/mcp/handlers/worker_standby.go:103`
- `c4-core/internal/mcp/handlers/worker_standby.go:191`
- `c4-core/internal/mcp/handlers/worker_standby.go:254`
- `c4-core/internal/mcp/handlers/worker_standby_test.go:42`

개선안:
1. 각 handler 진입부에 `deps`, `HubClient`, `ShutdownStore` nil guard 추가.
2. panic 대신 계약형 에러(`hub client not configured`, `shutdown store not configured`)를 반환.
3. nil-deps 경계 테스트를 추가해 회귀 방지.

수용기준:
1. 의존성 누락 시 tool call이 panic 없이 JSON-RPC 에러로 종료.
2. nil 관련 경계 테스트 3종(standby/complete/shutdown)이 모두 통과.

### CR-006 (P1) `c4_worker_complete.status` enum의 런타임 강제 검증 부재

현상:
- 스키마에는 `SUCCEEDED | FAILED` enum이 선언되어 있으나, 핸들러는 빈 문자열 여부만 검증한다.
- MCP 서버는 `tools/call` 시 `inputSchema` 기반 서버측 검증을 수행하지 않고 handler에 바로 전달한다.

사용자 영향:
- 클라이언트가 스키마 검증을 우회/미구현한 경우 잘못된 status 값이 들어올 수 있다.
- `FAILED`가 아닌 모든 문자열이 `exit_code=0` 경로로 처리되어 완료 의미가 왜곡될 수 있다.

근거:
- `c4-core/internal/mcp/handlers/worker_standby.go:159`
- `c4-core/internal/mcp/handlers/worker_standby.go:183`
- `c4-core/internal/mcp/mcp.go:73`
- `c4-core/internal/mcp/mcp.go:82`
- `c4-core/cmd/c4/mcp.go:595`
- `c4-core/cmd/c4/mcp.go:608`

개선안:
1. `handleWorkerComplete`에서 `switch`로 status를 명시 검증.
2. 허용 외 status는 즉시 에러 반환.
3. invalid status 테스트 케이스를 추가.

수용기준:
1. `status=UNKNOWN` 요청 시 `invalid status` 에러가 반환.
2. enum 허용값 외에는 `CompleteJob` 호출이 절대 발생하지 않음.

### CR-007 (P1) Lease 만료 모델 대비 갱신 경로 공백

현상:
- 워커 가이드는 lease 만료(5분, 갱신 없으면 재큐)를 명시한다.
- Hub 클라이언트에는 `RenewLease`가 구현되어 있으나, worker MCP 도구 집합/standby 스킬 루프에는 갱신 단계가 없다.

사용자 영향:
- 장시간 작업에서 lease 만료 후 잡 재큐/중복 실행 가능성이 증가한다.
- 완료 보고 시점과 Hub의 실제 lease 상태가 어긋날 수 있다.

근거:
- `docs/user-guide/워커-가이드.md:213`
- `docs/user-guide/워커-가이드.md:248`
- `c4-core/internal/hub/worker.go:51`
- `c4-core/internal/mcp/handlers/hub_jobs.go:14`
- `.claude/skills/c4-standby/SKILL.md:32`
- `.claude/skills/c4-standby/SKILL.md:57`

개선안:
1. 선택지 A: `c4_worker_standby`가 job 반환 후 별도 renew 루프를 관리하도록 확장.
2. 선택지 B: `c4_hub_lease_renew`(또는 동등 도구)를 MCP에 노출하고 `/c4-standby` 루프에 통합.
3. lease 만료 임계(예: 60초 전) 기준으로 주기적 renew 정책 명문화.

수용기준:
1. 5분 초과 장기 작업에서도 lease 만료 없이 완료 가능.
2. renew 실패 시 재시도/중단 정책이 문서와 코드에서 동일.

### AR-004 (P2) Hub capability 표면과 worker 운영면의 단절

현상:
- Hub Client는 renew capability를 보유하지만 worker 운영 도구(standby/complete/shutdown)에는 노출되지 않는다.
- 결과적으로 Hub API 역량과 MCP 운영 계약이 비대칭이다.

사용자 영향:
- 워커 운영 시나리오가 내부 구현 세부를 전제로 하게 되어 유지보수 비용이 증가한다.

근거:
- `c4-core/internal/hub/worker.go:51`
- `c4-core/internal/mcp/handlers/hub_jobs.go:14`
- `c4-core/internal/mcp/handlers/hub_jobs.go:167`

개선안:
1. worker 운영 계약(필수/선택 도구)을 별도 표준 문서로 정의.
2. Hub 기능이 추가되면 worker 운영 계약 반영 여부를 CI에서 강제.

수용기준:
1. Hub worker lifecycle 필수 API(register/claim/heartbeat/renew/complete)가 문서/도구에서 일관되게 확인됨.

### AR-005 (P2) Worker 계약 테스트의 경계/부정 케이스 부족

현상:
- 현재 테스트는 정상 흐름 중심이며 nil deps, invalid status, lease 만료 시나리오를 다루지 않는다.

사용자 영향:
- 계약 드리프트가 회귀 테스트에서 늦게 발견된다.

근거:
- `c4-core/internal/mcp/handlers/worker_standby_test.go:69`
- `c4-core/internal/mcp/handlers/worker_standby_test.go:97`
- `c4-core/internal/mcp/handlers/worker_standby_test.go:201`

개선안:
1. 부정 케이스(의존성 누락/invalid status/lease renew fail)를 table-driven으로 추가.
2. 문서의 worker timing/lease 규칙을 테스트 fixture에 반영.

수용기준:
1. worker 계약 관련 실패 모드가 테스트 목록으로 명시되고 CI에서 상시 검증.

---

### 12.4 라운드 3 (2026-02-16) - `docs/user-guide` vs `.claude/skills` 계약 diff 리뷰 (2번)

검토 범위:
1. 슬래시 명령 사용법/인자 표기
2. 실행 경로(MCP vs CLI) 설명
3. 사용자 가이드 내부 문서 간 자기일관성

이번 라운드 신규 누적:
1. 코드/계약 레벨: 4건 (`CR-008` ~ `CR-011`)
2. 구조 레벨: 1건 (`AR-006`)
3. 즉시 반영 권장: `CR-008`, `CR-009`, `CR-010`

### CR-008 (P0) `/submit` 인자 계약 충돌 재확인

현상:
- 명령어 레퍼런스는 `/submit <task_id> <commit_sha> [validation_results]`를 강제한다.
- 실제 스킬은 `/submit [task-id]`이며 task-id 생략 시 자동 탐지/대화형 제출을 안내한다.

사용자 영향:
- 문서대로 호출한 사용자와 스킬 안내를 따르는 사용자가 서로 다른 행동을 기대하게 된다.
- 자동화 스크립트/휴먼 사용 패턴이 분리되어 운영 혼선을 만든다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:230`
- `.claude/skills/submit/SKILL.md:17`
- `.claude/skills/submit/SKILL.md:38`

개선안:
1. `/submit` 계약을 단일 포맷으로 확정(필수 인자형 vs 자동탐지형).
2. 선택하지 않은 방식은 deprecated 경로로 명시.
3. 예시/파라미터 표를 모든 문서에서 동기화.

수용기준:
1. `/submit` 시그니처가 레퍼런스/스킬/헬프에서 동일.
2. 혼합 시그니처가 남아있으면 CI 실패.

### CR-009 (P1) `/add-task` 사용법이 문서 내부에서도 상충

현상:
- 명령어 레퍼런스는 positional 인자(`<task_id> <title> [options]`)를 제시.
- add-task 스킬과 사용 시나리오 문서는 대화형/설명형 입력(`/add-task [설명]`)을 제시.

사용자 영향:
- 동일 명령의 입력 스타일이 문서마다 달라 신규 사용자의 첫 성공률이 저하된다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:296`
- `.claude/skills/add-task/SKILL.md:18`
- `docs/user-guide/사용-시나리오.md:71`
- `docs/user-guide/사용-시나리오.md:79`

개선안:
1. canonical 입력 방식을 하나로 고정하고, 다른 방식은 "허용 별칭"으로만 기술.
2. 레퍼런스 문서에 대화형 fallback을 명시해 실제 동작과 합치.

수용기준:
1. `/add-task` 문서 예시가 모든 문서에서 동일한 계약 계층(정규/별칭)으로 정렬.

### CR-010 (P1) `/run` 인자 표면의 문서 불일치

현상:
- 명령어 레퍼런스는 `/run` 단일 형태만 제시한다.
- run/help 스킬은 `/run N`, `--max`, `--continuous`를 공식 사용법으로 제시한다.

사용자 영향:
- 병렬도 제어와 연속 실행 기능의 발견 가능성이 문서 선택에 따라 달라진다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:103`
- `.claude/skills/run/SKILL.md:18`
- `.claude/skills/run/SKILL.md:22`
- `.claude/skills/c4-help/SKILL.md:77`

개선안:
1. `/run` 인자 행렬을 명령어 레퍼런스에 승격.
2. Smart Auto Mode 문서와 레퍼런스 사이를 교차 참조로 고정.

수용기준:
1. `/run`의 인자/옵션 표가 레퍼런스/스킬에서 동일.

### CR-011 (P2) `/c4-stop` 실행 경로 설명 불투명

현상:
- 명령어 레퍼런스는 상태 전환 동작만 설명한다.
- stop 스킬은 내부 실행으로 `uv run c4 stop`을 명시한다.

사용자 영향:
- MCP 중심 운영을 기대한 사용자에게 런타임 의존성(uv/python/cli) 차이가 숨겨진다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:128`
- `.claude/skills/c4-stop/SKILL.md:18`

개선안:
1. `/c4-stop` 문서에 실제 실행 경로를 명시(MCP/CLI 중 무엇을 호출하는지).
2. 경로별 선행 조건(uv 설치 등)과 실패 시 fallback 절차를 추가.

수용기준:
1. stop 명령의 실행 경로/의존성이 사용자 문서에서 명확히 확인됨.

### AR-006 (P1) 명령 계약의 다중 소스 운영으로 드리프트 지속

현상:
- 동일 명령 계약이 `명령어-레퍼런스`, `skills`, `help`, `시나리오`에 중복 서술된다.
- 단일 소스가 없어 문서 수정 시 동기화 누락이 반복된다.

사용자 영향:
- 문서 신뢰도 하락, 온보딩 비용 증가, 지원/리뷰 비용 증가.

근거:
- `docs/user-guide/명령어-레퍼런스.md:8`
- `.claude/skills/c4-help/SKILL.md:77`
- `docs/user-guide/사용-시나리오.md:71`

개선안:
1. 명령 계약을 기계판독 가능한 manifest로 분리.
2. 나머지 문서는 manifest에서 생성(또는 검증)하도록 전환.

수용기준:
1. 명령 계약 수정 시 단일 파일만 변경하고, 나머지는 자동 갱신/검증됨.

---

### 12.5 라운드 4 (2026-02-16) - 계약 SSOT/CI 설계 리뷰 (3번)

목표:
- "코드 계약 ↔ 스킬 ↔ 문서"의 수동 동기화를 제거하고, 드리프트를 CI에서 조기 차단.

### AR-007 (P1) Registry 기반 계약 SSOT 부재

현상:
- 런타임 registry는 도구 이름/설명/`inputSchema`를 이미 보유한다.
- Lighthouse는 이를 `register_all`, `export_llms_txt`로 구조화할 수 있지만, 명령어 레퍼런스/skills 계약과 직접 연결되어 있지 않다.

근거:
- `c4-core/cmd/c4/mcp.go:586`
- `c4-core/internal/mcp/handlers/lighthouse.go:24`
- `c4-core/internal/mcp/handlers/lighthouse.go:426`

설계안:
1. `docs/contracts/command-contracts.json`(신규) 도입:
   - 필드: `command`, `aliases`, `args`, `execution_path`, `required_state`, `examples`, `source_tool`
2. 생성 경로:
   - 1차: MCP `tools/list` + Lighthouse 메타를 병합
   - 2차: skill frontmatter(`Usage`)를 파싱해 매핑
3. 소비 경로:
   - `명령어-레퍼런스.md`, `c4-help`, 스킬 Usage 섹션을 manifest 검증 대상으로 전환

수용기준:
1. 명령 계약의 단일 canonical JSON이 저장소에 존재.
2. 문서/스킬은 canonical 대비 차이(diff)로 검증 가능.

### AR-008 (P1) CI에 계약 일치성 게이트 부재

현상:
- 현재 CI는 Python lint/typecheck/test/build만 수행하고, docs/skills 계약 일치 검증이 없다.

근거:
- `.github/workflows/test.yml:1`
- `.github/workflows/test.yml:121`
- `scripts/` 하위에 계약 검증 스크립트 부재(`scripts` 검색 결과 기준)

설계안:
1. CI job `contract-consistency` 추가:
   - Step A: `tools/list` 또는 생성 스냅샷으로 canonical 로드
   - Step B: `docs/user-guide/명령어-레퍼런스.md` 파싱
   - Step C: `.claude/skills/*/SKILL.md` Usage 파싱
   - Step D: 시그니처/옵션/필수 상태 diff 검사
2. 실패 메시지는 "어느 파일의 어떤 명령이 어긋났는지"를 라인 단위로 출력.

수용기준:
1. 계약 드리프트 PR에서 CI가 명확한 diff와 함께 실패.
2. 문서/스킬만 수정해도 계약 위반이면 머지 차단.

### AR-009 (P2) 템플릿 계층(AGENTS/CLAUDE 템플릿/가이드) 간 용어 드리프트

현상:
- 템플릿 계층에서 완료 명령 표기가 `/finish`로 남아 있고, 프로젝트 지침은 `/finish`를 사용한다.
- init 템플릿과 프로젝트별 AGENTS/가이드가 독립 진화하면서 용어 동기화가 깨진다.

근거:
- `c4-core/cmd/c4/templates/claude_md.tmpl:37`
- `c4-core/cmd/c4/templates/claude_md.tmpl:87`
- `AGENTS.md:42`
- `AGENTS.md:64`

설계안:
1. "명령 명명 규칙"도 manifest에 포함해 템플릿 생성 입력으로 사용.
2. `c4 init` 시 생성되는 AGENTS/CLAUDE와 user-guide 용어를 동일 소스에서 렌더링.
3. 릴리스 전 체크리스트에 "command naming parity" 추가.

수용기준:
1. `/finish` vs `/finish`류의 명령 표기 차이가 자동 검출됨.
2. 템플릿 갱신 후 프로젝트 문서 재생성 시 용어가 일관됨.

---

### 12.6 라운드 2-4 종합 요약

이번 요청(1→2→3)으로 누적된 신규 항목:
1. 코드/계약 레벨: 7건 (`CR-005` ~ `CR-011`)
2. 구조/설계 레벨: 6건 (`AR-004` ~ `AR-009`)
3. 우선 반영 권장 Top 5: `CR-005`, `CR-006`, `CR-007`, `CR-008`, `AR-008`

다음 라운드 권장:
1. `c4_status`/`c4_get_task`/`c4_submit` 3종 응답 스키마 계약 테스트 심화
2. `명령어-레퍼런스` 파서/검증 스크립트 PoC 작성(문서는 유지, 코드 변경은 추후 일괄)
3. 계약 manifest 초안(JSON) 스키마 설계 및 필수 필드 합의

---

### 12.7 라운드 5 (2026-02-16) - `c4_status`/`c4_get_task`/`c4_submit` 심화 리뷰

검토 범위:
1. `c4_submit` 입력 계약과 소유권 검증 경계
2. `c4_get_task` 할당/추적 로직의 정확성
3. `/status` 문서 계약과 실제 응답 계약 정합성

이번 라운드 신규 누적:
1. 코드/계약 레벨: 5건 (`CR-012` ~ `CR-016`)
2. 구조 레벨: 2건 (`AR-010` ~ `AR-011`)
3. 즉시 반영 권장: `CR-012`, `CR-013`, `CR-014`

### CR-012 (P0) `c4_submit` 소유권 검증 우회 가능 (`worker_id` 생략)

현상:
- `c4_submit` 스키마는 `worker_id`를 선택 파라미터로 두고, required 목록에는 포함하지 않는다.
- 스토어의 owner guard는 `worker_id != ""`일 때만 실행된다.

사용자 영향:
- `in_progress` 태스크의 `task_id`를 아는 다른 워커/클라이언트가 `worker_id`를 비워 제출하면, 소유권 검증을 우회해 완료 처리할 수 있다.
- 멀티 워커 환경에서 완료 무결성이 깨질 수 있다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:22`
- `c4-core/internal/mcp/handlers/tasks.go:113`
- `c4-core/internal/mcp/handlers/sqlite_store.go:761`
- `docs/user-guide/Codex-가이드.md:37`
- `docs/user-guide/명령어-레퍼런스.md:230`

개선안:
1. `worker_id`를 required로 승격하고 handler/store에서 강제 검증.
2. 가능하면 클라이언트 입력 `worker_id` 신뢰 대신 세션/채널 identity 기반 검증으로 전환.
3. 소유권 우회(omit worker_id) 부정 테스트 추가.

수용기준:
1. `worker_id` 누락 제출은 명시적 에러로 거절.
2. 소유자 불일치 및 누락 케이스 모두 회귀 테스트에서 차단.

### CR-013 (P1) `validation_results` 계약 3중 불일치 (스키마 required, 문서 optional, 런타임 무검증)

현상:
- 도구 스키마는 `validation_results`를 required로 선언.
- 명령어 레퍼런스는 `validation_results`를 선택으로 표기.
- handler는 `task_id`, `commit_sha`만 검사하고 `validation_results` 존재 여부를 검사하지 않는다.

사용자 영향:
- 검증 결과 없이 제출이 가능해져 "검증 후 제출" 정책이 느슨해진다.
- 사용자 문서/실행 동작이 달라 자동화 및 운영 혼선이 생긴다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:91`
- `c4-core/internal/mcp/handlers/tasks.go:113`
- `c4-core/internal/mcp/handlers/tasks.go:235`
- `c4-core/internal/mcp/handlers/tasks.go:238`
- `docs/user-guide/명령어-레퍼런스.md:236`

개선안:
1. 정책 선택: `validation_results` 필수(비어있지 않음) 또는 명시적 optional.
2. 선택한 정책에 맞게 schema, handler, 문서를 일괄 정렬.
3. optional 정책이면 `validation_skipped` 같은 상태 필드를 명시적으로 남김.

수용기준:
1. `validation_results`에 대한 필수/선택 정책이 단일 계약으로 고정.
2. 문서/스키마/핸들러에서 동일하게 동작.

### CR-014 (P1) `validation_results.status` enum 서버측 미검증으로 우회 가능

현상:
- 스키마는 `status`를 `pass|fail` enum으로 선언.
- 런타임은 `fail`만 검사하고, 그 외 값(예: `unknown`)은 사실상 pass처럼 처리된다.

사용자 영향:
- 클라이언트가 enum 외 값을 보내도 제출이 진행될 수 있어 검증 품질이 훼손된다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:97`
- `c4-core/internal/mcp/handlers/sqlite_store.go:732`
- `c4-core/internal/mcp/handlers/sqlite_store.go:738`

개선안:
1. handler 또는 store에서 `status` 값 서버측 강제 검증 추가.
2. enum 외 값은 즉시 에러 반환.
3. `status=unknown` 케이스 테스트 추가.

수용기준:
1. enum 외 status는 제출이 거절됨.
2. 검증 상태 파싱 실패는 명시적 에러 메시지로 노출.

### CR-015 (P2) `AssignTask`의 `stale_reassign` 추적 이벤트 오표기 가능성

현상:
- stale 재할당 여부 플래그가 "최종 쿼리 성공 + taskID 존재" 조건으로 계산되어, 일반 pending 할당도 stale로 분류될 수 있다.
- 해당 플래그는 `stale_reassign` trace 이벤트 발생에 직접 사용된다.

사용자 영향:
- 운영 지표에서 stale 복구 빈도가 과대 계상되어 진단/튜닝 판단을 왜곡할 수 있다.

근거:
- `c4-core/internal/mcp/handlers/sqlite_store.go:627`
- `c4-core/internal/mcp/handlers/sqlite_store.go:537`

개선안:
1. stale 경로 진입 여부를 별도 boolean으로 분리해 명확히 추적.
2. fallback pending 할당과 stale 재할당 이벤트를 분리 기록.

수용기준:
1. 정상 pending 할당에서 `stale_reassign` 이벤트가 발생하지 않음.
2. stale 재할당 케이스에서만 해당 trace가 기록됨.

### CR-016 (P2) `/status` 출력 예시와 실제 응답 계약 괴리

현상:
- 명령어 레퍼런스는 `Execution Mode`, `Current Task`, `Checkpoints`, `Recent Events`를 상태 출력 예시로 제시한다.
- 현재 핸들러는 `ProjectStatus` 구조체를 그대로 반환하며, 핵심 필드는 task count/ready/deps 중심이다.

사용자 영향:
- 문서 예시를 기준으로 자동 파서/운영 절차를 만든 경우 실제 응답과 맞지 않아 파싱 실패 또는 오해가 발생한다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:159`
- `docs/user-guide/명령어-레퍼런스.md:167`
- `docs/user-guide/명령어-레퍼런스.md:173`
- `docs/user-guide/명령어-레퍼런스.md:177`
- `c4-core/internal/mcp/handlers/state.go:74`
- `c4-core/internal/store/types.go:106`

개선안:
1. 문서 예시를 실제 `ProjectStatus` 계약으로 업데이트.
2. 별도 포맷터(요약 텍스트)를 제공할 계획이면 해당 출력 계약을 명시적으로 분리 문서화.

수용기준:
1. `/status` 문서 예시가 현재 응답 스키마와 1:1로 매핑됨.

### AR-010 (P1) `c4_submit` 제출자 식별 정책의 SSOT 부재

현상:
- 코드에서는 `worker_id`가 선택 입력이지만, 운영 가이드 일부는 필수처럼 사용하고 일부는 생략 가능 형태를 안내한다.
- 결과적으로 "누가 제출했는가" 정책이 코드/문서에 일관되게 고정되어 있지 않다.

사용자 영향:
- 보안/무결성 관련 판단이 클라이언트 구현마다 달라지는 구조적 리스크가 생긴다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:22`
- `c4-core/internal/mcp/handlers/tasks.go:113`
- `docs/user-guide/Codex-가이드.md:37`
- `docs/user-guide/명령어-레퍼런스.md:230`

개선안:
1. 제출자 식별 정책(필수 필드, 신뢰 원천, 검증 규칙)을 단일 계약 문서로 고정.
2. 스키마/핸들러/가이드가 해당 정책을 자동 검증하도록 연결.

수용기준:
1. 제출자 식별 관련 계약이 모든 사용자 표면에서 동일.

### AR-011 (P2) `submit` 계약의 공격/경계 테스트 커버리지 부족

현상:
- 현재 테스트는 정상 제출, missing task_id/commit_sha, wrong-owner(명시 worker_id) 위주다.
- `worker_id` 누락 제출, `validation_results` 누락, enum 외 status 제출 경로를 다루지 않는다.

사용자 영향:
- 계약 우회 시나리오가 회귀 테스트에서 조기에 검출되지 않는다.

근거:
- `c4-core/internal/mcp/handlers/tasks_test.go:249`
- `c4-core/internal/mcp/handlers/tasks_test.go:256`
- `c4-core/internal/mcp/handlers/sqlite_store_direct_test.go:28`
- `c4-core/internal/mcp/handlers/sqlite_store_direct_test.go:50`

개선안:
1. 부정 케이스 3종(누락 worker_id, 누락 validation_results, invalid validation status)을 table-driven으로 추가.
2. owner guard 우회 방지 여부를 단위테스트로 고정.

수용기준:
1. 제출 계약 우회 시나리오가 CI에서 항상 실패로 잡힘.

---

### 12.8 라운드 5 종합 요약

이번 라운드 신규 누적:
1. 코드/계약 레벨: 5건 (`CR-012` ~ `CR-016`)
2. 구조 레벨: 2건 (`AR-010` ~ `AR-011`)
3. 우선 반영 권장 Top 3: `CR-012`, `CR-013`, `CR-014`

다음 라운드 권장:
1. `c4_submit` 보안/무결성 계약(제출자 식별, 검증결과 필수성) 확정안 리뷰
2. `submit/get_task/status` 3종 계약 테스트 명세서 초안 작성
3. docs 예시 출력과 실제 응답 스키마 자동 diff 규칙 설계

### 12.9 라운드 6 (claim/report/request_changes)

### CR-017 (P1) Direct 완료 경로가 `commit_sha/branch` 의미를 오염

현상:
- `c4_report`는 `summary`, `files_changed`를 받지만 저장 시 `commit_sha=summary`, `branch=files_changed(csv)`로 기록한다.
- 반면 worker 경로(`c4_submit`)는 `commit_sha`를 실제 커밋 증빙으로 사용한다.
- 동일 컬럼이 경로별로 다른 의미를 가지면서 리뷰 컨텍스트 해석까지 혼탁해진다.

사용자 영향:
- `commit_sha`를 기계적으로 신뢰하는 자동화/리포트에서 Direct 완료 이력이 오탐 또는 누락된다.
- 경로별 데이터 의미를 별도로 알아야 해 운영/분석 복잡도가 상승한다.

근거:
- `c4-core/internal/mcp/handlers/tracking.go:56`
- `c4-core/internal/mcp/handlers/tracking.go:65`
- `c4-core/internal/mcp/handlers/sqlite_store.go:774`
- `c4-core/internal/mcp/handlers/sqlite_store.go:898`
- `c4-core/internal/mcp/handlers/sqlite_store.go:718`
- `c4-core/internal/store/types.go:20`
- `c4-core/internal/store/types.go:21`

개선안:
1. `c4_tasks`에 `files_changed`(또는 `report_files`) 같은 명시 컬럼을 추가하고 `branch` 재사용을 중단.
2. Direct 경로도 선택형 `commit_sha`를 받거나, 별도 `summary/handoff` 컬럼으로 저장 분리.
3. `enrichWithReviewContext`가 의미 고정된 필드만 읽도록 정리.

수용기준:
1. `commit_sha` 컬럼에는 SHA 형태 값(또는 빈 값)만 존재한다.
2. 리뷰 컨텍스트의 `files_changed`가 전용 필드에서 일관되게 채워진다.

### CR-018 (P1) `max_revision` 경계 비교가 정책 문구와 충돌 가능

현상:
- `RequestChanges`에서 `nextVersion >= MaxRevision`이면 즉시 차단한다.
- 문서에는 `max_revision`을 "최대 수정 횟수(초과 시 BLOCKED)"로 설명하고 있어, 정확히 상한값까지 허용된다고 해석될 여지가 크다.

사용자 영향:
- 운영자가 설정값을 기준으로 기대한 반복 횟수보다 1회 적게 재시도가 허용될 수 있다.
- 장기적으로는 "정책 문구 vs 런타임 동작" 불일치로 디버깅 비용이 증가한다.

근거:
- `c4-core/internal/mcp/handlers/store_review.go:108`
- `docs/user-guide/명령어-레퍼런스.md:345`

개선안:
1. 정책을 명시적으로 결정: "최대 버전" 기준인지 "최대 재시도 횟수" 기준인지 SSOT에 고정.
2. 결정된 정책에 맞춰 비교식(`>=` 또는 `>`)과 오류 메시지를 동시에 정렬.
3. 경계값 테스트(`max_revision=3`일 때 허용/차단 버전)를 명시 추가.

수용기준:
1. `max_revision` 설명 문구와 실제 허용 버전이 1:1로 일치한다.
2. 경계값 테스트가 CI에서 회귀를 차단한다.

### CR-019 (P2) `required_changes` 정규화 부재로 후속 DoD 품질 저하

현상:
- 핸들러는 `required_changes` 비어있지 않음만 검사하고, 항목 trim/빈문자열 필터링은 수행하지 않는다.
- 저장 시 `strings.Join(requiredChanges, "\n- ")`로 바로 결합해 빈 항목/공백 항목이 그대로 DoD/리뷰 DoD에 반영된다.

사용자 영향:
- 생성된 후속 태스크 DoD가 깨진 bullet 또는 의미 없는 항목을 포함할 수 있다.
- 리뷰어/작업자가 실제 수정 요구를 빠르게 파악하기 어려워진다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:161`
- `c4-core/internal/mcp/handlers/tasks.go:326`
- `c4-core/internal/mcp/handlers/store_review.go:142`
- `c4-core/internal/mcp/handlers/store_review.go:164`

개선안:
1. `required_changes`를 저장 전에 `trim + 빈값 제거 + 중복 제거` 정규화.
2. 정규화 후 항목이 0개면 에러 반환.
3. DoD 생성 직전 항목 수를 검증해 포맷 깨짐 방지.

수용기준:
1. 공백/빈 문자열이 포함된 입력에서도 생성 DoD bullet이 항상 유효한 문장 목록으로 유지된다.

### CR-020 (P2) `REQUEST_CHANGES` 사유를 `commit_sha`에 저장

현상:
- `RequestChanges`는 리뷰 태스크를 완료 처리하면서 `commit_sha="REQUEST_CHANGES: <comments>"`를 기록한다.
- 리뷰/제출 경로별로 `commit_sha`의 의미가 더욱 다중화된다.

사용자 영향:
- 완료 이력 분석 시 `commit_sha`를 기준으로 경로를 통합 조회하기 어렵다.
- 관측 데이터 정합성이 낮아져 리포트/자동화 품질이 저하된다.

근거:
- `c4-core/internal/mcp/handlers/store_review.go:113`

개선안:
1. 리뷰 판정 사유는 별도 컬럼(예: `decision_note`) 또는 `handoff`로 저장.
2. `commit_sha`에는 실제 커밋 증빙만 기록하도록 정책 고정.

수용기준:
1. `REQUEST_CHANGES` 사유 조회가 `commit_sha` 없이 가능한 저장 구조를 가진다.

### AR-012 (P1) 완료/리뷰 증빙 데이터 모델 SSOT 부재

현상:
- Worker 완료(`submit`), Direct 완료(`report`), Review 거절(`request_changes`)이 서로 다른 필드 매핑으로 증빙 데이터를 기록한다.
- `commit_sha/branch/handoff/summary/files_changed/comments`의 의미 경계가 코드 레벨에서 일관되지 않다.

사용자 영향:
- 계약 해석이 구현 맥락 의존적이 되어 신규 도구/리포트 개발 시 오류 유입 가능성이 높다.
- 운영 문서와 실제 데이터 모델을 동시에 추적해야 해 학습/운영 비용이 증가한다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:87`
- `c4-core/internal/mcp/handlers/tracking.go:56`
- `c4-core/internal/mcp/handlers/store_review.go:113`
- `c4-core/internal/mcp/handlers/sqlite_store.go:774`
- `c4-core/internal/mcp/handlers/sqlite_store.go:898`
- `c4-core/internal/store/types.go:41`

개선안:
1. 완료/리뷰 증빙 필드를 도메인 모델로 분리(`completion_evidence`, `review_decision` 등).
2. 스키마/핸들러/문서를 단일 계약 생성 소스에서 파생.
3. 경로별 필드 매핑 표를 문서화하고 계약 테스트와 연결.

수용기준:
1. 동일 의미 데이터가 경로와 무관하게 동일 필드로 저장된다.
2. 문서 계약과 저장 모델 간 수동 동기화 지점이 제거된다.

### AR-013 (P2) claim/report/request_changes 경계 테스트 커버리지 부족

현상:
- 현재 테스트는 주로 필수값 누락/정상 경로를 검증한다.
- `max_revision` 경계, `required_changes` 정규화, Direct 완료 저장 필드 의미 검증이 빠져 있다.

사용자 영향:
- 정책 변경이나 리팩토링 시 계약 회귀가 CI에서 조기 검출되지 않는다.

근거:
- `c4-core/internal/mcp/handlers/tasks_test.go:537`
- `c4-core/internal/mcp/handlers/sqlite_store_direct_test.go:367`
- `c4-core/internal/mcp/handlers/sqlite_store_direct_test.go:143`

개선안:
1. `RequestChanges` 경계값 table-driven 테스트(`max_revision` 1/2/3 케이스) 추가.
2. `required_changes` 공백/중복 입력 정규화 테스트 추가.
3. Direct 완료 후 DB 필드 매핑 검증 테스트 추가.

수용기준:
1. 위 3개 축의 회귀 테스트가 CI에 포함되고, 계약 변경 시 실패로 즉시 드러난다.

---

### 12.10 라운드 6 종합 요약

이번 라운드 신규 누적:
1. 코드/계약 레벨: 4건 (`CR-017` ~ `CR-020`)
2. 구조 레벨: 2건 (`AR-012` ~ `AR-013`)
3. 우선 반영 권장 Top 3: `CR-017`, `CR-018`, `AR-012`

다음 라운드 권장:
1. 완료/리뷰 증빙 데이터 모델(필드 의미 고정) RFC 초안 작성
2. `max_revision` 정책 문구와 런타임 비교식 단일화
3. claim/report/request_changes 경계 회귀 테스트 명세 확정

### 12.11 라운드 7 (checkpoint/mark_blocked)

### CR-021 (P1) `c4_mark_blocked`가 대상 태스크 미존재여도 성공으로 응답

현상:
- 저장소 구현은 `UPDATE ... WHERE task_id = ?` 실행 후 영향 행 수를 검사하지 않는다.
- 결과적으로 존재하지 않는 `task_id`에 대해서도 handler는 성공 메시지를 반환하고 `task.blocked` 이벤트를 발행한다.

사용자 영향:
- 운영자가 차단 처리 성공으로 오인할 수 있고, 이벤트 소비자는 실제 DB 상태와 다른 시그널을 받는다.

근거:
- `c4-core/internal/mcp/handlers/sqlite_store.go:832`
- `c4-core/internal/mcp/handlers/sqlite_store.go:840`
- `c4-core/internal/mcp/handlers/tasks.go:361`

개선안:
1. `RowsAffected()`가 0이면 `task not found` 오류 반환.
2. 실제 상태 전이가 일어난 경우에만 `task.blocked` 이벤트 발행.

수용기준:
1. 존재하지 않는 태스크 ID에 대한 `c4_mark_blocked` 호출은 오류를 반환한다.
2. DB 상태와 이벤트 발행 결과가 항상 일치한다.

### CR-022 (P1) `c4_mark_blocked` 진단 파라미터가 저장되지 않음

현상:
- API 계약은 `failure_signature`, `attempts`, `last_error`를 받도록 설계되어 있다.
- 그러나 저장소 구현은 status 변경만 수행하고 해당 진단 정보를 DB/이벤트 어디에도 기록하지 않는다.

사용자 영향:
- 차단 사유 추적(왜 막혔는지, 몇 회 시도했는지)이 불가능해 운영 가시성이 떨어진다.
- 재현/회귀 방지 데이터가 누락되어 자동화 규칙 고도화가 어렵다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:178`
- `c4-core/internal/mcp/handlers/tasks.go:179`
- `c4-core/internal/mcp/handlers/sqlite_store.go:830`
- `c4-core/internal/mcp/handlers/sqlite_store.go:832`
- `c4-core/internal/mcp/handlers/sqlite_store.go:840`

개선안:
1. `c4_tasks` 또는 별도 `c4_task_failures` 테이블에 `failure_signature/attempts/last_error`를 저장.
2. `task.blocked` 이벤트 payload에도 동일 필드를 포함해 downstream 분석 가능하게 유지.

수용기준:
1. 차단 이벤트 1건당 실패 시그니처와 시도 횟수를 재구성할 수 있다.

### CR-023 (P1) 체크포인트 저장 실패가 은닉되고 `success=true`가 반환됨

현상:
- `c4_checkpoints` 저장 실패 시 stderr 로그만 남기고 함수는 계속 진행한다.
- 이후 응답 객체는 항상 `Success: true`로 구성되어 호출자는 실패를 감지할 수 없다.

사용자 영향:
- 체크포인트 이력이 실제로는 유실되었는데도 성공으로 보이므로 감사 추적 신뢰도가 낮아진다.

근거:
- `c4-core/internal/mcp/handlers/store_review.go:46`
- `c4-core/internal/mcp/handlers/store_review.go:50`
- `c4-core/internal/mcp/handlers/store_review.go:67`
- `c4-core/internal/mcp/handlers/store_review.go:92`

개선안:
1. 저장 실패 시 즉시 에러 반환(최소한 `success=false` + 명시 메시지).
2. EventBus 발행/후속 로직은 저장 성공 이후에만 실행.

수용기준:
1. 체크포인트 저장 실패를 호출자가 API 레벨에서 확실히 감지할 수 있다.

### CR-024 (P1) `REQUEST_CHANGES`의 거절 귀속이 "최신 in_progress 태스크" 휴리스틱에 의존

현상:
- 체크포인트 인자/저장 모델에 대상 태스크 식별자가 없어, 구현은 최신 `in_progress` 태스크를 조회해 거절 통계를 기록한다.
- 병렬 실행 환경에서는 실제 리뷰 대상과 무관한 태스크/작업자에 거절 통계가 기록될 수 있다.

사용자 영향:
- persona 통계와 품질 지표가 오염되어 의사결정(재할당/학습/평가)에 왜곡이 발생한다.

근거:
- `c4-core/internal/mcp/handlers/tracking.go:24`
- `c4-core/internal/mcp/handlers/sqlite_store.go:147`
- `c4-core/internal/mcp/handlers/store_review.go:79`
- `c4-core/internal/mcp/handlers/store_review.go:86`

개선안:
1. 체크포인트 계약에 대상 `task_id`(또는 `review_task_id`)를 명시 필수화.
2. 거절 통계는 명시된 대상에만 기록하고, 휴리스틱 경로는 제거.

수용기준:
1. 병렬 작업 상황에서도 거절 통계가 정확한 대상 태스크/작업자에만 기록된다.

### CR-025 (P0) 문서화된 `APPROVE_FINAL` 결정값이 런타임에서 거부됨

현상:
- 사용자 문서는 `/checkpoint` 결정값으로 `APPROVE_FINAL`을 제시한다.
- 실제 런타임 검증은 `APPROVE`, `REQUEST_CHANGES`, `REPLAN`만 허용한다.

사용자 영향:
- 문서대로 실행하면 즉시 에러가 발생해 체크포인트 흐름이 중단된다.

근거:
- `docs/user-guide/명령어-레퍼런스.md:284`
- `c4-core/internal/mcp/handlers/tracking.go:87`
- `c4-core/internal/mcp/handlers/tracking.go:208`
- `c4-core/internal/mcp/handlers/tracking.go:212`

개선안:
1. 정책 결정: `APPROVE_FINAL`을 실제 지원할지, 문서에서 제거할지 확정.
2. 확정안에 맞춰 스키마 enum/런타임 검증/문서를 일괄 정렬.

수용기준:
1. 문서의 결정값 목록과 런타임 허용값이 완전히 일치한다.

### AR-014 (P1) 체크포인트 도메인 모델에 대상 엔터티 연결(태스크/리뷰)이 없음

현상:
- 체크포인트 테이블은 `checkpoint_id/decision/notes/required_changes`만 저장한다.
- 이 설계 때문에 구현이 대상 식별을 런타임 휴리스틱으로 보완하고 있다.

사용자 영향:
- 데이터 모델이 의사결정 맥락을 보존하지 못해, 통계/감사/재현성 품질이 구조적으로 낮아진다.

근거:
- `c4-core/internal/mcp/handlers/sqlite_store.go:147`
- `c4-core/internal/mcp/handlers/tracking.go:23`
- `c4-core/internal/mcp/handlers/store_review.go:79`

개선안:
1. 체크포인트 엔터티에 `task_id` 또는 `review_task_id`를 추가해 맥락을 1차 저장.
2. 기존 데이터 마이그레이션 후 휴리스틱 기반 귀속 로직 제거.

수용기준:
1. 체크포인트 레코드 단건만으로 어떤 작업 맥락의 결정인지 판별 가능하다.

### AR-015 (P2) checkpoint/mark_blocked 테스트가 핸들러 중심으로 편향됨

현상:
- 관련 테스트 대부분이 mock store 기반 handler 검증이다.
- 저장소 레벨에서의 실패 경로(0 rows update, checkpoint insert 실패, 병렬 귀속 오염)는 고정되지 않았다.

사용자 영향:
- 리팩토링 시 핵심 런타임 보장(정확한 저장/귀속/이벤트 일치)이 회귀될 가능성이 높다.

근거:
- `c4-core/internal/mcp/handlers/handlers_test.go:507`
- `c4-core/internal/mcp/handlers/handlers_test.go:647`
- `c4-core/internal/mcp/handlers/tasks_test.go:448`
- `c4-core/internal/mcp/handlers/sqlite_store_direct_test.go:383`

개선안:
1. SQLite store 통합 테스트에 `mark_blocked` 실패/무효 ID 케이스 추가.
2. checkpoint 저장 실패와 다중 in_progress 귀속 케이스를 store 테스트로 고정.

수용기준:
1. 위 경계 시나리오가 CI 회귀 테스트로 자동 검증된다.

---

### 12.12 라운드 7 종합 요약

이번 라운드 신규 누적:
1. 코드/계약 레벨: 5건 (`CR-021` ~ `CR-025`)
2. 구조 레벨: 2건 (`AR-014` ~ `AR-015`)
3. 우선 반영 권장 Top 3: `CR-025`, `CR-023`, `CR-024`

다음 라운드 권장:
1. checkpoint 결정 enum 및 문서 계약 단일화
2. mark_blocked 진단 데이터 저장 모델 추가
3. checkpoint/mark_blocked 저장소 통합 테스트 확장

### 12.13 라운드 8 (add_todo/task_list)

### CR-026 (P1) `c4_add_todo.execution_mode` 입력이 런타임에서 무시됨

현상:
- 스키마/인자에는 `execution_mode(worker/direct/auto)`가 정의되어 있다.
- 그러나 `handleAddTodo` 및 `AddTask` 저장 경로에서 이 값을 읽거나 저장하지 않는다.
- 즉, API 표면에 노출된 모드 제어가 실제 실행 모델에 반영되지 않는다.

사용자 영향:
- 호출자는 `direct`를 지정해도 태스크 생성 결과에서 모드가 보존/강제되지 않아 오동작을 유발할 수 있다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:36`
- `c4-core/internal/mcp/handlers/tasks.go:134`
- `c4-core/internal/mcp/handlers/tasks.go:250`
- `c4-core/internal/mcp/handlers/sqlite_store.go:458`
- `c4-core/internal/store/types.go:9`

개선안:
1. `execution_mode`를 저장 모델에 추가하고 생성 시 영속화.
2. `direct` 태스크의 허용 경로(`claim/report`)와 `worker` 태스크의 허용 경로(`get_task/submit`)를 모드 기반으로 강제.

수용기준:
1. `execution_mode`를 지정해 생성한 태스크가 조회 결과와 실행 가드에서 동일하게 반영된다.

### CR-027 (P1) 리뷰 태스크 ID 생성이 하이픈 포함 Task ID에서 충돌/오귀속 가능

현상:
- 리뷰 ID 생성은 `ParseTaskID` 결과의 `baseID/version`에 의존한다.
- `ParseTaskID`는 `SplitN("-", 3)` 후 `baseID=parts[1]` 규칙을 사용하므로, 하이픈이 많은 task ID에서 베이스가 과도 축약된다.
- 축약된 베이스로 `ReviewID`를 만들면 서로 다른 태스크가 동일 리뷰 ID로 수렴할 수 있다.

사용자 영향:
- 리뷰 태스크 연결이 잘못되거나 중복 충돌로 생성 실패할 수 있다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:291`
- `c4-core/internal/mcp/handlers/tasks.go:292`
- `c4-core/internal/task/models.go:62`
- `c4-core/internal/task/models.go:77`
- `c4-core/internal/task/models.go:81`
- `c4-core/internal/task/models.go:91`

개선안:
1. Task ID 문법을 명확히 강제(정규식)하거나, 리뷰 ID 생성을 full ID 기반의 비가역 변환이 아닌 명시 규칙으로 재설계.
2. 파싱 실패/비표준 ID는 조기 에러 처리.

수용기준:
1. 하이픈 포함/비표준 케이스에서도 리뷰 ID 충돌 없이 안정적으로 생성된다.

### CR-028 (P1) `review_required=true`에서도 리뷰 생성 실패가 성공으로 은닉됨

현상:
- 메인 태스크 추가 후 리뷰 태스크 생성 실패 시 warning 로그만 출력하고 성공 응답을 반환한다.
- 기본값이 `review_required=true`인 도구 계약과 실제 보장이 다르다.

사용자 영향:
- 운영자는 리뷰 태스크가 생성된 것으로 오인할 수 있고, Review-as-Task 워크플로우가 조용히 깨진다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:288`
- `c4-core/internal/mcp/handlers/tasks.go:304`

개선안:
1. `review_required=true`에서 리뷰 생성 실패 시 전체 호출을 실패 처리.
2. `review_required=false`에서만 best-effort 동작 허용.

수용기준:
1. `review_required=true` 호출은 메인/리뷰 태스크가 모두 생성될 때만 성공한다.

### CR-029 (P2) `task.created` 이벤트 payload 필드명이 의미와 불일치 (`mode` <- domain)

현상:
- 태스크 생성 이벤트에서 `mode` 키에 `task.Domain` 값을 담아 발행한다.
- 도메인 정보를 모드로 표기해 이벤트 계약 해석이 혼란스러워진다.

사용자 영향:
- 이벤트 소비자가 필드 의미를 오해해 잘못된 라우팅/분석을 수행할 수 있다.

근거:
- `c4-core/internal/mcp/handlers/sqlite_store.go:480`
- `c4-core/internal/mcp/handlers/sqlite_store.go:483`
- `c4-core/internal/mcp/handlers/tasks.go:131`

개선안:
1. 이벤트 키를 `domain`으로 정정하거나, 실제 모드를 별도 필드로 명시 분리.

수용기준:
1. 이벤트 payload 필드명과 값 의미가 1:1로 일치한다.

### CR-030 (P1) `c4_task_list`가 SQLite 백엔드에 하드 종속됨

현상:
- `handleTaskList`는 내부적으로 `*SQLiteStore` 타입 단언(또는 Local unwrap) 실패 시 즉시 오류를 반환한다.
- 도구 설명은 일반 task list 기능으로 노출되지만, 구현은 특정 저장소 구현에 종속되어 있다.

사용자 영향:
- 비-SQLite 백엔드에서 동일 도구를 호출하면 기능이 갑자기 비활성화되어 운영 일관성이 깨진다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:378`
- `c4-core/internal/mcp/handlers/tasks.go:392`
- `c4-core/internal/cloud/hybrid.go:40`

개선안:
1. `Store` 인터페이스에 목록 조회 계약을 승격하거나, 백엔드별 adapter를 등록해 동일 도구 계약 보장.
2. 불가피하면 도구 설명에 백엔드 제한을 명시.

수용기준:
1. 지원 백엔드 범위와 런타임 동작이 문서/스키마와 일치한다.

### AR-016 (P1) Task ID 문법 계약 부재로 파서/생성 로직이 암묵 규칙에 의존

현상:
- `c4_add_todo`는 자유 입력형 task ID를 받지만, 내부 리뷰 생성은 특정 ID 문법을 전제한 파서 규칙에 의존한다.
- ID 문법이 계약으로 명시/검증되지 않아 구현 세부가 사용자 계약을 사실상 결정하고 있다.

사용자 영향:
- 호출자마다 ID 스타일이 달라지면 리뷰 연동 품질이 비결정적으로 흔들린다.

근거:
- `c4-core/internal/mcp/handlers/tasks.go:126`
- `c4-core/internal/mcp/handlers/tasks.go:291`
- `c4-core/internal/task/models.go:61`

개선안:
1. task ID grammar를 명시 문서+런타임 검증으로 고정.
2. 파서와 생성기(`ReviewID`, `NextVersionID`)를 해당 grammar 기반 단일 모듈로 통합.

수용기준:
1. ID 문법 위반 입력은 일관된 오류로 거부되고, 유효 입력은 항상 결정적 ID를 생성한다.

### AR-017 (P2) add_todo/task_list 핸들러 계약 테스트 공백

현상:
- `handleAddTodo` 테스트는 기본 happy/missing 케이스 중심이며 `execution_mode` 반영/ID 파싱 경계를 다루지 않는다.
- `handleTaskList` 자체의 단위 테스트는 부재하고, 현재는 store 레벨 테스트 위주다.

사용자 영향:
- 계약 드리프트(`execution_mode` dead field, backend 제한)가 테스트에서 조기 탐지되지 않는다.

근거:
- `c4-core/internal/mcp/handlers/tasks_test.go:12`
- `c4-core/internal/mcp/handlers/task_list_test.go:37`
- `c4-core/internal/mcp/handlers/handlers_test.go:715`

개선안:
1. `execution_mode` 반영 여부, ID 파싱 경계, review 생성 실패 처리에 대한 handler 테스트 추가.
2. `handleTaskList`의 백엔드 분기(SQLite/Hybrid/비지원) 테스트 추가.

수용기준:
1. 핵심 계약 분기(모드, ID 파싱, 백엔드 제한)가 테스트로 고정된다.

---

### 12.14 라운드 8 종합 요약

이번 라운드 신규 누적:
1. 코드/계약 레벨: 5건 (`CR-026` ~ `CR-030`)
2. 구조 레벨: 2건 (`AR-016` ~ `AR-017`)
3. 우선 반영 권장 Top 3: `CR-026`, `CR-027`, `CR-028`

다음 라운드 권장:
1. `execution_mode` 저장/가드 설계 확정
2. task ID grammar 확정 + 리뷰 ID 생성 규칙 재정의
3. add_todo/task_list 핸들러 계약 테스트 확장

### 12.15 문서-계획 현행화 (2026-02-17)

#### 12.15.1 반영 완료 상태 (라운드 8 이후)
1. `T-U026-0` (`CR-026`) 구현 완료 (`status=done`)
   - commit: `e0849bf81c1cd5020026dcac33d12f5e85c45367`
2. `R-U026-0` 리뷰 태스크 완료 (`status=done`)
3. 라운드 8 우선 반영 Top 3 중 `CR-026`은 종료되었고, 잔여 핵심은 `CR-027`, `CR-028`이다.

#### 12.15.2 현재 C4 Plan 스냅샷 (2026-02-17 기준)
1. 전체 집계
   - `pending 22`
   - `ready 6`
   - `blocked_by_dependencies 16`
   - `in_progress 0`
2. 즉시 실행 가능(Ready) 태스크
   - `T-U027-0`, `T-U024-0`, `T-U020-0`, `T-U022-0`, `T-U030-0`, `T-U029-0`
3. 의존성으로 대기(Blocked) 중인 핵심 태스크
   - `T-U028-0` <- `T-U027-0`
   - `T-UA013-0` <- `T-U017-0`, `T-U018-0`, `T-U019-0`, `T-U020-0`
   - `T-UA015-0` <- `T-U021-0`, `T-U022-0`, `T-U023-0`, `T-U024-0`, `T-U025-0`
   - `T-UA017-0` <- `T-U026-0`, `T-U027-0`, `T-U028-0`, `T-U029-0`, `T-U030-0`
   - `T-U900-0` <- `T-UA013-0`, `T-UA015-0`, `T-UA017-0`
4. 리뷰 태스크(`R-*`)는 모두 대응 구현 태스크(`T-*`) 완료 후 실행되는 1:1 의존 구조다.

#### 12.15.3 의존성 태스크 그래프 (요약)
1. Stream A (ID/Review 경로)
   - `T-U027-0 -> T-U028-0 -> T-UA017-0 -> T-U900-0`
2. Stream B (Checkpoint/Blocked 경로)
   - `T-U024-0 -> T-UA015-0 -> T-U900-0`
3. Stream C (Evidence/TaskList 경로)
   - `T-U020-0 -> T-UA013-0 -> T-U900-0`
   - `T-U022-0 -> T-UA015-0 -> T-U900-0`
   - `T-U030-0 -> T-UA017-0 -> T-U900-0`
   - `T-U029-0 -> T-UA017-0 -> T-U900-0`
4. 리뷰 흐름
   - 각 `R-*`는 대응 `T-*` 직후 배치 (구현 완료 검증 게이트 역할)

#### 12.15.4 라운드 후속 실행 순서 (일괄 c4plan 반영안)
1. Wave 1 (병렬 실행)
   - `T-U027-0`, `T-U024-0`, `T-U020-0`, `T-U022-0`, `T-U030-0`, `T-U029-0`
2. Wave 2
   - `T-U028-0`
3. Wave 3 (구조/테스트 잠금)
   - `T-UA013-0`, `T-UA015-0`, `T-UA017-0`
4. Wave 4 (최종 수렴)
   - `T-U900-0`
5. 운영 원칙
   - 각 Wave 완료 직후 대응 `R-*` 태스크를 처리해 회귀 범위를 단계적으로 잠근다.

#### 12.15.5 운영 메모 (머지 가드)
1. Codex 진단 유지: dependency 저장 포맷(`JSON 배열 문자열`) 파싱 보강이 필요하다.
2. `T-UA012` SSOT 문서는 코드 정합 없이 단독 머지하지 않는다.
3. 위 항목은 `T-U900-0` 완료 조건 점검 시 다시 확인한다.

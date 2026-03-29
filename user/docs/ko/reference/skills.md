# 스킬 레퍼런스

스킬은 Claude Code 내에서 호출하는 슬래시 명령입니다. **42개의 스킬** 전부가 CQ 바이너리에 내장되어 있습니다(`skills_embed` 빌드 태그) — 설치 후 인터넷 연결 불필요.

## 아이디어 탐색

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/pi` | play idea, 아이디어, ideation, /pi | 계획 전 아이디어를 발산하고 구체화합니다. 발산/수렴/조사/토론 모드 지원. `idea.md` 작성 후 `/c4-plan` 자동 실행. |

## 핵심 워크플로우

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-plan` | plan, 계획, 설계, 기획 | Discovery → Design → Lighthouse 계약 → 태스크 생성. 기능의 완전한 구조화 계획. |
| `/c4-run` | run, 실행, ㄱㄱ | 대기 중인 모든 태스크에 Worker를 병렬로 스폰. 연속 모드 — 큐가 비워질 때까지 자동 재스폰. |
| `/c4-finish` | finish, 마무리, 완료 | 빌드 → 테스트 → 문서 → 커밋. 구현 후 완료 루틴. |
| `/c4-status` | status, 상태 | 진행 상황, 의존성 그래프, 큐 요약, Worker 상태가 포함된 시각적 태스크 그래프. |
| `/c4-quick` | quick, 빠르게 | 즉시 태스크 하나를 생성하고 할당, 계획 단계 생략. 작고 집중된 변경에 적합. |

## 품질 루프

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-polish` | polish | *(폐기됨 — polish 루프는 `/c4-finish`에 통합되었습니다. 별도 호출 불필요.)* |
| `/c4-refine` | refine | *(폐기됨 — 품질 루프는 `/c4-finish`에 통합되었습니다. 별도 호출 불필요.)* |
| `/c4-checkpoint` | (체크포인트 도달 시 자동 실행) | 페이즈 게이트: 4가지 관점 리뷰 (전체적 / 사용자 흐름 / 연쇄 영향 / 출시 준비). 승인, 변경 요청, 재계획, 재설계 중 선택. |
| `/c4-validate` | validate, 검증 | 심각도 기반 처리로 lint + 테스트 실행. CRITICAL은 커밋 차단, HIGH는 리뷰 필요, MEDIUM은 권장 사항. |
| `/c4-review` | review | 6축 평가로 3단계 코드 또는 논문 종합 리뷰. 공식 리뷰 문서 생성. |
| `/company-review` | company review, PR 리뷰, diff 리뷰 | PI Lab 표준 코드 리뷰. PR/MR diff 기반 6축 평가. |
| `/c4-submit` | submit, 제출 | 자동 검증과 함께 완료된 태스크 제출. 커밋 SHA 검증, 필요 시 체크포인트 트리거. |
| `/simplify` | simplify, 단순화 | 변경된 코드의 재사용성, 품질, 효율성을 검토하고 발견된 문제 수정. |

## 태스크 관리

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-add-task` | add task, 태스크 추가 | DoD, 범위, 도메인 가이드를 포함하여 대화형으로 태스크 추가. 기존 패턴에서 ID 추론. |
| `/c4-stop` | stop, 중단 | 실행 중단, HALTED 상태로 전환. 이후 재개를 위해 진행 상황 보존. |
| `/c4-swarm` | swarm | 코디네이터 주도 에이전트 팀 스폰. 모드: standard(구현), review(읽기 전용 감사), investigate(가설 경쟁). |

## 세션

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/done` | done, 세션 종료, session done | 현재 세션을 완전 캡처와 함께 완료로 표시 — 작업 요약, 지식 저장, 상태 정리. |
| `/c4-attach` | 세션 이름, attach, name this session | 나중에 `cq claude -t <name>`으로 재개할 수 있도록 현재 세션에 이름 부여. 메모 선택적 추가 가능. |
| `/c4-reboot` | reboot, 재시작 | 현재 이름 붙인 세션 재시작. `cq`가 동일한 세션 UUID로 자동 재개. |
| `/session-distill` | session distill, 세션 요약, distill | 현재 세션을 지속적 지식으로 정제. 결정, 패턴, 인사이트를 지식 베이스에 추출. |

## 연구 & 문서

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c9-init` | c9-init, c9 초기화 | 새 C9 연구 프로젝트 초기화. 메트릭, 수렴 조건, Hub URL이 포함된 `state.yaml` 생성. |
| `/c9-loop` | c9-loop | 메인 루프 드라이버 — `state.yaml`에서 현재 페이즈를 읽어 다음 단계를 자동 실행. |
| `/c9-survey` | c9-survey | Gemini Google Search 그라운딩을 사용하여 최신 arXiv 논문 + SOTA 벤치마크 조사. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) 토론 모드 — 연구 컨퍼런스 시뮬레이션. |
| `/c9-steer` | c9-steer | `state.yaml`을 직접 편집하지 않고 페이즈 변경 및 사유 업데이트. |
| `/c9-report` | c9-report | 분산 Worker를 통해 원격 서버에서 실험 결과 수집. |
| `/c9-finish` | c9-finish | Research Loop 완료 시 최고 모델 저장 + 결과 문서화. |
| `/c9-deploy` | c9-deploy | 최고 모델을 엣지 서버에 배포. `/c9-finish`와 독립적으로 실행 가능. |
| `/research-loop` | research loop | 논문-실험 개선 루프. 목표 품질에 도달할 때까지 리뷰 → 계획 → 실험 → 재리뷰 반복. |
| `/experiment-workflow` | experiment workflow, 실험 워크플로우 | 엔드투엔드 실험 생명주기 관리: 데이터 준비 → 학습 → 평가 → 기록. |
| `/c2-paper-review` | 논문 리뷰, paper review | *(폐기됨 — `/c4-review` 사용 권장.)* |

## 개발

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/tdd-cycle` | TDD, test-driven, RED-GREEN-REFACTOR | TDD 사이클 가이드. RED-GREEN-REFACTOR 순서로 테스트 기반 구현. |
| `/debugging` | debugging, 디버깅, 버그 추적 | 체계적 디버깅. 재현 → 가설 수립 → 고립 → 수정 → 검증 순서. |
| `/spec-first` | spec-first, 스펙 먼저, 설계 문서 | Spec-First 개발 가이드. 코드 작성 전 스펙 문서 먼저 작성. |
| `/incident-response` | incident, 장애, 서버 다운, 에러율 급증 | 프로덕션 장애 대응 워크플로우. 분류 → 진단 → 완화 → 사후 검토. |

## 메타 & 유틸리티

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/craft` | craft, 스킬 만들어줘, rule 만들어줘 | 대화형으로 스킬, 에이전트, 규칙, CLAUDE.md 커스터마이징 생성. |
| `/c4-help` | help | 스킬, 에이전트, MCP 도구 빠른 참조. 42개 스킬 전체에서 결정 트리 + 키워드 검색. |
| `/c4-clear` | clear | 디버깅을 위한 C4 상태 초기화. 설정 보존 옵션과 함께 `.c4/`의 태스크, 이벤트, 락 초기화. |
| `/init` | init, 초기화 | 현재 프로젝트에 C4 초기화. 설치 경로 감지 후 `cq claude/cursor/codex` 실행. |
| `/claude-md-improver` | CLAUDE.md 개선, claude-md, improve instructions | 프로젝트의 CLAUDE.md 분석 및 개선. 구조 점검, 빌드/테스트 명령, 에이전트 규칙 확인. |
| `/skill-tester` | skill tester, 스킬 테스트, eval | 스킬 품질 테스트 및 평가. eval 케이스 생성, 분류 시험 실행, 트리거 정확도 측정. |
| `/pr-review` | PR 만들어, PR 체크리스트, pull request | PR/MR 생성 체크리스트 및 리뷰 가이드. 병합 전 자동 검증. |
| `/c4-release` | release | git 이력에서 CHANGELOG 생성. Conventional Commits 분석, 시맨틱 버전 제안, 태그 생성. |
| `/c4-standby` | standby, 대기, worker mode | Supabase를 통해 세션을 지속적인 분산 Worker로 전환. 작업 대기, 실행, 결과 보고. *full 티어 전용* |
| `/c4-interview` | interview | 심층 탐색 요구사항 인터뷰. 시니어 PM/아키텍트 역할로 숨겨진 요구사항과 엣지 케이스 발견. |

---

## 스킬 상태 확인

> `connected` 또는 `full` 티어 필요 (haiku 분류에 LLM Gateway 필요).

스킬이 올바르게 트리거되는지 측정하고 모니터링합니다 — 변경 전후로 Claude가 사용자 프롬프트를 정확하게 분류하는지 보장합니다.

| MCP 도구 | 설명 |
|----------|------|
| `c4_skill_eval_run` | 스킬의 EVAL.md 테스트 케이스에 대해 k-trial haiku 분류 실행. `trigger_accuracy` 반환. |
| `c4_skill_eval_generate` | haiku를 사용하여 스킬의 EVAL.md 테스트 케이스 생성 (긍정 + 부정 프롬프트). |
| `c4_skill_eval_status` | 평가된 전체 스킬의 트리거 정확도 요약 표시. `ok` = ≥ 0.90. |

`cq doctor`는 스킬이 0.90 임계값 아래로 떨어지면 경고하는 `skill-health` 검사를 포함합니다.

---

## 기계 가독 형식

프로그래밍 방식으로 사용하기 위한 JSONL 다운로드:

```sh
curl https://playidea-lab.github.io/cq/api/skills.jsonl
```

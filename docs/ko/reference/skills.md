# Skill 레퍼런스

Skill은 Claude Code 안에서 호출하는 슬래시 커맨드입니다. 42개의 모든 Skill이 CQ 바이너리에 내장되어 있습니다(`skills_embed` 빌드 태그) — 설치 후 인터넷이 필요 없습니다.

---

## 아이디어 탐색

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/pi` | play idea, ideation, /pi | 계획 전 아이디어를 브레인스토밍하고 다듬습니다. 발산/수렴/조사/토론 모드. `idea.md`를 작성하고 자동으로 `/c4-plan`을 실행합니다. |

---

## 핵심 워크플로우

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-plan` | plan, design, spec | Discovery -> Design -> Lighthouse 계약 -> 태스크 생성. 기능을 위한 완전한 구조화된 계획. |
| `/c4-run` | run, execute | 모든 대기 중인 태스크를 위한 Worker 병렬 스폰. 지속 모드 — 큐가 빌 때까지 자동 재스폰. |
| `/c4-finish` | finish, complete | 빌드 -> 테스트 -> 문서 -> 커밋. 구현 후 완료 루틴. |
| `/c4-status` | status | 진행률, 의존성 그래프, 큐 요약, Worker 상태가 있는 시각적 태스크 그래프. |
| `/c4-quick` | quick | 태스크 즉시 생성 + 할당, 계획 건너뜀. 작고 집중된 변경사항용. |

---

## 품질 루프

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-checkpoint` | (체크포인트에서 자동) | 단계 게이트: 4렌즈 리뷰 (전체적 / 사용자 흐름 / 파급 효과 / 출시 준비). 승인, 변경 요청, 재계획, 재설계. |
| `/c4-validate` | validate | lint + 테스트 실행. CRITICAL은 커밋 차단, HIGH는 리뷰 필요, MEDIUM은 권장. |
| `/c4-review` | review | 6축 평가를 통한 종합적 3-pass 코드 또는 논문 리뷰. 공식 리뷰 문서 생성. |
| `/c4-polish` | polish | *(Deprecated — polish 루프가 `/c4-finish`에 내장됨)* |
| `/c4-refine` | refine | *(Deprecated — 품질 루프가 `/c4-finish`에 내장됨)* |

---

## 태스크 관리

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-add-task` | add task | DoD, 범위, 도메인 가이던스와 함께 태스크 대화형 추가. 기존 패턴에서 ID를 추론. |
| `/c4-submit` | submit | 자동 유효성 검사와 함께 완료된 태스크 제출. 커밋 SHA 확인, 필요 시 체크포인트 트리거. |
| `/c4-interview` | interview | 깊이 있는 탐색적 요구사항 인터뷰. 시니어 PM/아키텍트로서 숨겨진 요구사항과 엣지 케이스 발견. |
| `/c4-stop` | stop | 실행 중지, HALTED 상태로 전환. 나중에 재개할 수 있도록 진행 상황 보존. |
| `/c4-clear` | clear | 디버깅용 C4 상태 초기화. 선택적 설정 보존으로 `.c4/`의 태스크, 이벤트, 잠금 지우기. |

---

## 협업 및 확장

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c4-swarm` | swarm | 코디네이터 주도 Agent 팀 스폰. 모드: standard(구현), review(읽기 전용 감사), investigate(가설 경쟁). |
| `/c4-standby` | standby, worker mode | Supabase를 통해 세션을 영속적 분산 Worker로 전환. 작업을 기다리고, 실행하고, 보고합니다. *full 티어만* |

---

## 연구 및 문서

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/research-loop` | research loop | 논문-실험 개선 루프. 목표 품질 도달할 때까지 리뷰 -> 계획 -> 실험 -> 재리뷰를 반복. |
| `/c2-paper-review` | paper review | *(Deprecated — 대신 `/c4-review` 사용)* |

---

## C9 Research Loop (ML)

> `connected` 또는 `full` 티어 필요 (Hub 작업 제출용).

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/c9-init` | c9-init | 새 C9 연구 프로젝트 초기화. 메트릭, 수렴 조건, Hub URL이 있는 `state.yaml` 생성. |
| `/c9-loop` | c9-loop | 메인 루프 드라이버 — `state.yaml`에서 현재 단계를 읽고 다음 단계 자동 실행. |
| `/c9-run` | c9-run | 현재 라운드의 실험 YAML을 Supabase Worker 큐에 제출. |
| `/c9-check` | c9-check | 실험 결과 파싱 + 수렴 확인. C4의 체크포인트와 동일. |
| `/c9-standby` | c9-standby | RUN 단계에서 대기; 메일을 통해 훈련 완료 시 CHECK 자동 트리거. |
| `/c9-finish` | c9-finish | Research Loop 완료 시 최적 모델 저장 + 결과 문서화. |
| `/c9-steer` | c9-steer | `state.yaml`을 직접 수정하지 않고 단계와 이유 변경. |
| `/c9-survey` | c9-survey | Gemini Google Search grounding으로 최신 arXiv 논문 + SOTA 벤치마크 탐색. |
| `/c9-report` | c9-report | 분산 Worker를 통해 원격 서버에서 실험 결과 수집. |
| `/c9-conference` | c9-conference | Claude (Opus) + Gemini (Pro) 토론 모드 — 연구 컨퍼런스 시뮬레이션. |
| `/c9-deploy` | c9-deploy | 엣지 서버에 최적 모델 배포. `/c9-finish`와 독립적으로 실행 가능. |

---

## 유틸리티

| Skill | 트리거 | 설명 |
|-------|--------|------|
| `/init` | init | 현재 프로젝트에 C4 초기화. 설치 경로 감지, `cq claude/cursor/codex` 실행. |
| `/c4-release` | release | git 이력에서 CHANGELOG 생성. Conventional Commits 분석, 시맨틱 버전 제안, 태그 생성. |
| `/c4-help` | help | Skill, Agent, MCP 도구 빠른 참조. 결정 트리 + 키워드 검색. |
| `/c4-attach` | attach, name this session | `cq claude -t <name>`으로 나중에 재개하기 위해 현재 세션에 이름 붙이기. 선택적으로 메모 추가. |
| `/c4-reboot` | reboot | 현재 이름 있는 세션 재부팅. `cq`가 같은 세션 UUID로 자동 재개. |

---

## Skill 상태 확인

> `connected` 또는 `full` 티어 필요 (haiku 분류에 LLM Gateway 필요).

Skill이 올바르게 트리거되는지 측정하고 모니터링 — Claude가 사용자 프롬프트를 정확하게 분류하는지 확인.

| MCP 도구 | 설명 |
|---------|------|
| `c4_skill_eval_run` | Skill의 EVAL.md 테스트 케이스에 k-trial haiku 분류 실행. `trigger_accuracy` 반환. |
| `c4_skill_eval_generate` | haiku를 사용하여 Skill의 EVAL.md 테스트 케이스 생성 (긍정 + 부정 프롬프트). |
| `c4_skill_eval_status` | 평가된 모든 Skill의 트리거 정확도 요약 표시. `ok` = >= 0.90. |

`cq doctor`에 Skill이 0.90 임계값 아래로 떨어지면 경고하는 `skill-health` 확인이 포함됩니다.

---

## Skill 카탈로그 (42개)

카테고리별 전체 목록:

| Skill | 카테고리 |
|-------|---------|
| `/pi` | 아이디어 탐색 |
| `/c4-plan` | 핵심 워크플로우 |
| `/c4-run` | 핵심 워크플로우 |
| `/c4-finish` | 핵심 워크플로우 |
| `/c4-status` | 핵심 워크플로우 |
| `/c4-quick` | 핵심 워크플로우 |
| `/c4-checkpoint` | 품질 루프 |
| `/c4-validate` | 품질 루프 |
| `/c4-review` | 품질 루프 |
| `/c4-polish` | 품질 루프 (deprecated) |
| `/c4-refine` | 품질 루프 (deprecated) |
| `/c4-add-task` | 태스크 관리 |
| `/c4-submit` | 태스크 관리 |
| `/c4-interview` | 태스크 관리 |
| `/c4-stop` | 태스크 관리 |
| `/c4-clear` | 태스크 관리 |
| `/c4-swarm` | 협업 |
| `/c4-standby` | 협업 |
| `/research-loop` | 연구 |
| `/c2-paper-review` | 연구 (deprecated) |
| `/c9-init` | C9 연구 |
| `/c9-loop` | C9 연구 |
| `/c9-run` | C9 연구 |
| `/c9-check` | C9 연구 |
| `/c9-standby` | C9 연구 |
| `/c9-finish` | C9 연구 |
| `/c9-steer` | C9 연구 |
| `/c9-survey` | C9 연구 |
| `/c9-report` | C9 연구 |
| `/c9-conference` | C9 연구 |
| `/c9-deploy` | C9 연구 |
| `/init` | 유틸리티 |
| `/c4-release` | 유틸리티 |
| `/c4-help` | 유틸리티 |
| `/c4-attach` | 유틸리티 |
| `/c4-reboot` | 유틸리티 |

---

## 머신 가독 형식

프로그래밍 방식으로 사용하려면 JSONL 다운로드:

```sh
curl https://playidea-lab.github.io/cq/api/skills.jsonl
```

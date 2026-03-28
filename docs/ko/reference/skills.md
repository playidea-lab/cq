# 스킬 레퍼런스

스킬은 Claude Code 내에서 호출하는 슬래시 명령입니다. 36개의 스킬이 모두 CQ 바이너리에 내장되어 있습니다(`skills_embed` 빌드 태그) — 설치 후 인터넷 연결 불필요.

## 아이디어 탐색

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/pi` | play idea, 아이디어, ideation, /pi | 계획 전 아이디어 발산·수렴. 발산/수렴/리서치/토론 모드. `idea.md` 생성 후 자동으로 `/c4-plan` 실행. |

## 핵심 워크플로우

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-plan` | plan, 계획, 설계, 기획 | Discovery → Design → Lighthouse 계약 → 태스크 생성. 기능에 대한 전체 구조화 계획. |
| `/c4-run` | run, 실행, ㄱㄱ | 모든 pending 태스크에 대해 워커를 병렬로 스폰. 연속 모드 — 큐가 빌 때까지 자동 respawn. |
| `/c4-finish` | finish, 마무리, 완료 | 빌드 → 테스트 → 문서 → 커밋. 구현 완료 루틴. |
| `/c4-status` | status, 상태 | 진행 상황, 의존성 그래프, 큐 요약, 워커 상태를 포함한 시각적 태스크 그래프. |
| `/c4-quick` | quick, 빠르게 | 즉시 하나의 태스크를 생성+할당, 계획 생략. 작고 집중된 변경에 적합. |

## 품질 루프

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-polish` | polish | *(Deprecated — polish 루프가 이제 `/c4-finish`에 내장됨. 별도 호출 불필요.)* |
| `/c4-refine` | refine | *(Deprecated — 품질 루프가 이제 `/c4-finish`에 내장됨. 별도 호출 불필요.)* |
| `/c4-checkpoint` | (체크포인트 시 자동) | Phase gate: 4-lens 리뷰 (holistic / user-flow / cascade / ship-ready). 승인, 변경 요청, 재계획, 재설계. |
| `/c4-validate` | validate, 검증 | 심각도 기반 처리로 lint + 테스트 실행. CRITICAL은 커밋 차단, HIGH는 리뷰 필요, MEDIUM은 권장. |
| `/c4-review` | review | 6축 평가로 3-pass 코드 또는 논문 리뷰. 공식 리뷰 문서 생성. |

## 태스크 관리

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-add-task` | add task, 태스크 추가 | DoD, 범위, 도메인 가이던스로 대화형 태스크 추가. 기존 패턴에서 ID 추론. |
| `/c4-submit` | submit, 제출 | 자동 검증으로 완료된 태스크 제출. commit SHA 검증, 필요 시 체크포인트 트리거. |
| `/c4-interview` | interview | 심층 탐색적 요구사항 인터뷰. 시니어 PM/아키텍트 역할로 숨겨진 요구사항과 엣지 케이스를 발굴. |
| `/c4-stop` | stop, 중단 | 실행 중단, HALTED 상태로 전환. 나중에 재개할 수 있도록 진행 상황 보존. |
| `/c4-clear` | clear | 디버깅용 C4 상태 초기화. 설정 보존 옵션으로 `.c4/`의 태스크, 이벤트, 잠금 삭제. |

## 협업 및 확장

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c4-swarm` | swarm | 코디네이터 주도 에이전트 팀 스폰. 모드: standard (구현), review (읽기 전용 감사), investigate (가설 경쟁). |
| `/c4-standby` | standby, 대기, worker mode | 세션을 Supabase 기반 분산 워커로 변환. 잡을 기다리고, 실행하고, 결과를 보고. *full 티어 전용* |

## 리서치 및 문서

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c2-paper-review` | 논문 리뷰, paper review | *(Deprecated — `/c4-review` 사용 권장.)* |
| `/research-loop` | research loop | 논문-실험 개선 루프. 목표 품질 달성까지 리뷰 → 계획 → 실험 → 재리뷰를 반복. |

## C9 연구 루프 (ML)

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/c9-init` | c9-init, c9 초기화 | C9 연구 프로젝트 초기화. `state.yaml` 생성, 메트릭/수렴 조건 설정. |
| `/c9-loop` | c9-loop | 메인 루프 드라이버 — `state.yaml`의 현재 phase를 읽어 다음 단계 자동 실행. |
| `/c9-run` | c9-run | 현재 라운드 실험 YAML을 Supabase 워커 큐에 제출. |
| `/c9-check` | c9-check | 실험 결과 파싱 + 수렴 판정. C4의 checkpoint에 해당. |
| `/c9-standby` | c9-standby | RUN phase 중 대기. mail 알림 수신 시 자동으로 CHECK 실행. |
| `/c9-finish` | c9-finish | 연구 루프 완료 시 best model 저장 + 결과 문서화. |
| `/c9-steer` | c9-steer | `state.yaml` 직접 편집 없이 phase 전환 및 reason 업데이트. |
| `/c9-survey` | c9-survey | Gemini Google Search grounding으로 최신 arXiv 논문·SOTA 수집. |
| `/c9-report` | c9-report | 분산 워커를 통해 원격 서버 실험 결과 수집 및 보고서 생성. |
| `/c9-conference` | c9-conference | Claude(Opus) + Gemini(Pro) 합의 토론 — 연구 컨퍼런스 시뮬레이션. |
| `/c9-deploy` | c9-deploy | best model을 edge 서버에 배포. `/c9-finish`와 독립 실행 가능. |

## 스킬 헬스

> `connected` 또는 `full` 티어 필요 (haiku 분류에 LLM Gateway 필요).

스킬 트리거 정확도를 측정하고 모니터링합니다 — 변경 전후로 Claude가 사용자 프롬프트를 올바르게 분류하는지 확인.

| MCP 도구 | 설명 |
|---------|------|
| `c4_skill_eval_run` | 스킬의 EVAL.md 테스트 케이스에 k번 haiku 분류 실행. `trigger_accuracy` 반환. |
| `c4_skill_eval_generate` | haiku로 스킬의 EVAL.md 테스트 케이스(긍정/부정 프롬프트) 자동 생성. |
| `c4_skill_eval_status` | 평가된 모든 스킬의 트리거 정확도 요약. `ok` = ≥ 0.90. |

`cq doctor`의 `skill-health` 체크가 임계값(0.90) 미달 스킬을 자동으로 경고합니다.

## 유틸리티

| 스킬 | 트리거 | 설명 |
|------|--------|------|
| `/init` | init, 초기화 | 현재 프로젝트에 C4 초기화. 설치 경로 감지 후 `cq claude/cursor/codex` 실행. |
| `/c4-release` | release | git 히스토리에서 CHANGELOG 생성. Conventional Commits 분석, 시맨틱 버전 제안, 태그 생성. |
| `/c4-help` | help | 스킬, 에이전트, MCP 도구의 빠른 레퍼런스. 전체 36개 스킬에 대한 결정 트리 + 키워드 검색. |
| `/c4-attach` | 세션 이름, attach, name this session | `cq claude -t <name>`으로 나중에 재개할 수 있도록 현재 세션에 이름 붙이기. 선택적 메모 추가 가능. |
| `/c4-reboot` | reboot, 재시작 | 현재 이름 붙은 세션 재부팅. `cq`가 같은 세션 UUID로 자동 재개. |

---

## 기계가 읽을 수 있는 형식

JSONL 형식으로 다운로드:

```sh
curl https://playidea-lab.github.io/cq/api/skills.jsonl
```

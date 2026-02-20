# C4 Usage Guide

> 103개 도구, 16개 커맨드, 37개 에이전트 — 상황에 맞는 최적 경로를 30초 안에 선택하기

---

## 1. 의사결정 트리

작업 크기만 판단하면 경로가 정해집니다.

```
작업이 뭔가?
│
├─ 1줄 수정 (타이포, 로그, 주석)
│  → 그냥 고쳐. C4 불필요.
│
├─ 소규모 (1-5파일, 요구사항 명확)
│  → /c4-quick "설명" → 작업 → /c4-submit
│
├─ 중규모 (5-15파일, 아키텍처 확정)
│  → /c4-add-task 반복 → /c4-run → /c4-polish → /c4-finish
│    또는 Direct: c4_claim → c4_report
│
├─ 대규모 (15+파일, 새 도메인)
│  → /c4-interview → /c4-plan → /c4-run N → /c4-polish → /c4-finish
│    또는 /c4-swarm N → /c4-polish → /c4-finish
│
├─ 연구/실험
│  → /c4-research 또는 /c4-swarm --investigate
│
└─ 문서 작업 (논문 리뷰, HWP/DOCX 분석)
   → /c4-review 또는 c4_parse_document
```

**확신이 없으면 작은 모드부터 시작**하세요. 병렬성이 보이면 올려도 됩니다.

---

## 2. 실행 모드 선택

### Direct vs Worker vs Swarm

| 기준 | Direct | Worker (`/c4-run`) | Swarm (`/c4-swarm`) |
|------|--------|-------------------|---------------------|
| 태스크 수 | 1-2개 | 3개+ 독립적 | 3개+ 상호의존적 |
| 파일 겹침 | 높음 | 낮음 (worktree 격리) | 중간 |
| 에이전트 간 소통 | 불필요 | 불필요 | 필요 |
| 비용 | 최저 | 중간 | 최고 |
| 사용법 | `c4_claim` → `c4_report` | `/c4-run N` | `/c4-swarm N` |
| 최적 상황 | 버그 수정, 리팩토링 | 독립 기능 구현 | 리뷰, 조사, 교차 기능 |

### 판단 기준

- **파일이 겹치면** → Direct (충돌 방지)
- **독립적이면** → Worker (병렬 속도)
- **결과를 교차 참조해야 하면** → Swarm (에이전트 협업)

### Swarm 모드 상세

| 변형 | 용도 | 팀 구성 |
|------|------|---------|
| `/c4-swarm N` | 병렬 구현 | N명 워커, 도메인 기반 에이전트 자동 매핑 |
| `/c4-swarm --review` | 다각도 리뷰 | Security + Performance + Test 3명 (읽기 전용) |
| `/c4-swarm --investigate` | 가설 경쟁 | 3명 조사관, 각자 독립 가설 검증 후 토론 |

---

## 3. 시나리오별 워크플로우

### A. 버그 수정 (가장 빈번)

**상황**: 특정 버그를 찾아 고치는 단일 작업

```bash
/c4-quick "fix: 타임아웃 버그 in auth middleware"
# → 자동으로 태스크 생성 + claim

# 코드 수정 후
/c4-validate               # lint + test 실행
/c4-submit                 # 완료 제출
```

**주의사항**: 수정 후 반드시 `/c4-validate`로 검증. 검증 실패 시 submit 금지.

### B. 명확한 기능 추가 (요구사항 확정)

**상황**: "JWT 인증 추가" 같은 범위가 명확한 기능

```bash
/c4-add-task "JWT 인증 미들웨어 추가"
# → DoD(Definition of Done) 대화형 생성

/c4-run 1                  # Worker 1개 스폰 실행
# 또는 Direct 모드:
# c4_claim(task_id) → 작업 → c4_report(task_id, summary, files_changed)

/c4-checkpoint             # 완료 시 리뷰 요청
```

**주의사항**: 태스크가 3개 이상이면 `/c4-run N`으로 병렬 실행을 고려하세요.

### C. 대규모 신기능 (아키텍처 결정 필요)

**상황**: "실시간 동기화 시스템" 같은 설계가 필요한 기능

```bash
/c4-interview "실시간 동기화"  # 심층 요구사항 탐색 (선택)
/c4-plan                      # Discovery → Design → Tasks 자동 생성
/c4-run 3                     # 3개 Worker 병렬 실행
/c4-checkpoint                # 단계별 리뷰
# → 리뷰 결과에 따라 수정 후 반복
```

**주의사항**: `/c4-interview`는 선택 사항이지만, 요구사항이 모호할 때 큰 도움이 됩니다.

### D. 코드 리뷰

**상황**: 구현 완료 후 다각도 품질 검증

```bash
/c4-swarm --review            # 보안/성능/테스트 3개 리뷰어 스폰
# 또는 간단히:
/c4-checkpoint                # 단일 리뷰
```

**리뷰 우선순위** (SOUL 기준):
1. 데이터 무결성 / 보안 / 권한
2. 장애 복구 가능성 (rollback, idempotency)
3. 관측 가능성 (logging, metrics)
4. 테스트 커버리지 / 회귀 위험
5. 가독성 / 스타일

### E. 복잡한 디버깅

**상황**: 원인 불명의 버그, 여러 가설 탐색 필요

```bash
/c4-swarm --investigate       # 3개 에이전트가 각각 다른 가설 추적
# → 결과 종합 → 원인 특정 → 수정

# 해결 후 지식으로 기록
c4_knowledge_record(type="insight", content="원인: ... 해결: ...")
```

**주의사항**: 장시간 디버깅 세션 종료 시 반드시 발견 사항을 기록하세요.

### F. ML 실험 파이프라인

**상황**: 모델 학습, GPU 작업 제출, DAG 파이프라인 실행

```bash
# 단일 작업 제출
c4_hub_submit(name="train-resnet", workdir="/git/ml", command="python train.py")
c4_hub_watch(job_id="...")     # 로그 실시간 확인
c4_hub_summary(job_id="...")   # 결과 + 메트릭 종합

# 다단계 파이프라인 (전처리 → 학습 → 평가)
c4_hub_dag_from_yaml(yaml_content="""
name: train-pipeline
nodes:
  - name: preprocess
    command: python preprocess.py
  - name: train
    command: python train.py --epochs 100
    gpu_count: 1
  - name: evaluate
    command: python evaluate.py
dependencies:
  - source: preprocess
    target: train
  - source: train
    target: evaluate
""")
c4_hub_dag_execute(dag_id="...")
```

### G. 논문 연구 루프

**상황**: 논문 품질을 반복 개선 (리뷰 → 실험 → 수정 → 재리뷰)

```bash
/c4-research start "PPAD Paper 1" --paper paper.pdf --target 7.0
/c4-research next            # 자동 단계 감지: 리뷰 → 실험 계획 → 실행 → 완료
# 세션을 넘겨도 상태가 유지됩니다
/c4-research status          # 진행 상황 확인
/c4-research approve complete  # 목표 달성 시 종료
```

### H. 문서 분석 & 리뷰

**상황**: HWP/DOCX/PDF/PPTX 문서 분석 또는 학술 논문 리뷰

```bash
# 문서 텍스트 추출
c4_extract_text(file_path="report.hwp")

# 구조화된 블록 파싱 (테이블, 이미지 포함)
c4_parse_document(file_path="thesis.docx")

# 학술 논문 6축 리뷰
/c4-review paper.pdf
```

---

## 4. 커맨드 전체 레퍼런스

### 매일 쓰는 커맨드 (필수 6개)

| 커맨드 | 용도 | 인자 형식 | 사용 시점 |
|--------|------|----------|-----------|
| `/c4-status` | 현황 파악 | (없음) | 세션 시작, 진행 중 확인 |
| `/c4-quick` | 즉시 작업 시작 | `"설명"` `[scope=경로]` | 소규모 작업 |
| `/c4-run` | Worker 병렬 실행 | `[N]` `[--continuous]` `[--max N]` | 독립 태스크 3개+ |
| `/c4-submit` | 완료 제출 | `[task-id]` | 작업 완료 시 |
| `/c4-validate` | 검증 실행 | (없음) | submit 전 필수 |
| `c4_claim` / `c4_report` | Direct 모드 | `task_id` (MCP 도구) | 파일 겹침 높은 작업 |

### 주간 쓰는 커맨드 (상황별 5개)

| 커맨드 | 용도 | 인자 형식 | 사용 시점 |
|--------|------|----------|-----------|
| `/c4-plan` | 대규모 기획 | (없음) | 새 기능, 15+파일 |
| `/c4-add-task` | 태스크 수동 추가 | `"설명"` `[--domain D]` | plan 없이 태스크 생성 |
| `/c4-checkpoint` | 단계별 리뷰 | (없음) | 구현 완료 후 |
| `/c4-swarm` | 팀 협업 | `[N]` `[--review]` `[--investigate]` | 리뷰, 조사, 교차 기능 |
| `/c4-stop` | 실행 중단 | (없음) | EXECUTE → HALTED 전환 (`/c4-run`으로 재개) |

### 가끔 쓰는 커맨드 (파워유저 5개)

| 커맨드 | 용도 | 인자 형식 | 사용 시점 |
|--------|------|----------|-----------|
| `/c4-interview` | 심층 인터뷰 | `"주제"` | 모호한 요구사항 탐색 |
| `/c4-release` | 체인지로그 생성 | (없음) | 릴리즈 전 |
| `/c4-research` | 연구 반복 | `[start\|status\|next\|record\|approve]` | 논문 품질 반복 개선 (alias: `/research-loop`) |
| `/c4-review` | 논문 리뷰 | `<pdf_path>` | 학술 논문 6축 분석 (alias: `/c2-review`) |
| `/c4-init` | 프로젝트 초기화 | (없음) | 새 프로젝트 시작 (또는 터미널에서 `c4`) |

### 메타 커맨드

| 커맨드 | 용도 | 인자 형식 |
|--------|------|----------|
| `/c4-help` | 커맨드/에이전트/도구 참조 | `[commands\|agents\|tools\|keyword]` |

### 운영 커맨드 (드물게)

| 커맨드 | 용도 | 인자 형식 | 주의 |
|--------|------|----------|------|
| `/c4-clear` | 상태 완전 초기화 | (없음) | **되돌릴 수 없음**. `.c4/` 전체 삭제 |

---

## 5. 에이전트 가이드

37개 에이전트는 `/c4-swarm`이나 `/c4-run`이 태스크 도메인에 따라 자동 매핑합니다. 직접 선택할 필요는 거의 없지만, 알아두면 도메인 지정 시 유리합니다.

### 도메인 → 에이전트 자동 매핑

`/c4-add-task` 시 `domain` 필드를 지정하면 Swarm이 최적 에이전트를 선택합니다.

| 도메인 | 에이전트 | 전문 영역 |
|--------|----------|-----------|
| `go` | `golang-pro` | goroutine, channel, interface, 에러 처리 |
| `python` | `python-pro` | decorator, async, 성능 최적화 |
| `frontend` | `frontend-developer` | React, 상태 관리, 접근성 |
| `backend` | `backend-architect` | REST API, 마이크로서비스, DB 스키마 |
| `database` | `database-optimizer` | 쿼리 튜닝, 인덱스 설계, 캐싱 |
| `security` | `security-auditor` | 취약점 탐지, 인증/인가 검증 |
| `ml` | `ml-engineer` | 모델 서빙, 피처 엔지니어링 |
| `devops` | `deployment-engineer` | CI/CD, Docker, K8s |
| `performance` | `performance-engineer` | 프로파일링, 병목 최적화 |
| `infra` | `cloud-architect` | AWS/GCP/Azure, Terraform |
| `testing` | `test-automator` | unit/integration/e2e 테스트 |
| (지정 없음) | `general-purpose` | 범용 |

### 특수 목적 에이전트

| 에이전트 | 용도 | 호출 방법 |
|----------|------|-----------|
| `code-reviewer` | 리뷰 태스크 자동 할당 | `/c4-swarm --review` 또는 자동 R-task |
| `debugger` | 에러/실패 조사 | `/c4-swarm --investigate` |
| `architect-review` | PRD/아키텍처 검토 | `/c4-plan` 내부에서 사용 |
| `c4-scout` | 코드베이스 빠른 탐색 | 내부 사용 (500토큰 압축 컨텍스트) |
| `content-writer` | 기술 문서/블로그 작성 | 직접 호출 |
| `prompt-engineer` | LLM 프롬프트 최적화 | AI 기능 개발 시 |

### 전체 에이전트 카테고리 (37개 → 9개 그룹)

| 카테고리 | 에이전트 | 수 |
|----------|---------|:--:|
| **Backend** | backend-architect, database-optimizer, graphql-architect | 3 |
| **Frontend** | frontend-developer, frontend-designer, react-pro | 3 |
| **DevOps/Infra** | deployment-engineer, devops-troubleshooter, cloud-architect | 3 |
| **Quality** | code-reviewer, code-refactorer, security-auditor, test-automator | 4 |
| **Data/ML** | data-engineer, data-scientist, ml-engineer, ai-engineer | 4 |
| **Languages** | golang-pro, python-pro, rust-pro, tauri-developer | 4 |
| **Documentation** | api-documenter, content-writer, prd-writer, prompt-engineer | 4 |
| **Project** | workflow-orchestrator, project-task-planner, context-manager, memory-bank | 4 |
| **Specialty** | debugger, incident-responder, performance-engineer, legacy-modernizer, mobile-developer, dx-optimizer, payment-integration, math-teacher, vibe-coding-coach, architect-review, c4-scout | 11 |

#### 혼동하기 쉬운 에이전트 구분

| 쌍 | 구분 |
|----|------|
| debugger vs incident-responder | 개발 중 버그 vs 프로덕션 장애 |
| frontend-designer vs frontend-developer | 디자인→스펙 vs 스펙→코드 |
| backend-architect vs cloud-architect | API/DB 설계 vs 인프라/IaC |
| code-reviewer vs code-refactorer | 읽기 전용 리뷰 vs 코드 변경 실행 |
| deployment-engineer vs devops-troubleshooter | CI/CD 구축 vs 장애 디버깅 |

### 에이전트를 직접 쓸 필요가 없는 이유

C4 Swarm이 태스크의 `domain` 필드를 보고 자동 매핑합니다:

```bash
/c4-add-task "Redis 캐시 레이어 추가" --domain database
/c4-swarm 2
# → database-optimizer 에이전트가 자동 할당됨
```

---

## 6. 인프라 기능

사용자가 직접 호출할 일은 드물지만, 시스템이 내부적으로 쓰거나 특정 시나리오에서 필요한 기능들입니다.

### Daemon — 로컬 작업 스케줄링 (Hub 불필요)

Hub 서버 없이 로컬에서 바로 작업을 실행합니다. `.c4/daemon.db`에 저장.

| 도구 | 용도 |
|------|------|
| `c4_job_submit` | 작업 제출 (command, exp_id, tags, env, timeout_sec, memo) |
| `c4_job_list` | 작업 목록 조회 (status 필터 지원) |
| `c4_job_status` | 특정 작업 상세 (로그 + 메트릭 포함) |
| `c4_job_cancel` | 실행 중 작업 취소 |
| `c4_job_summary` | 큐 전체 통계 (QUEUED/RUNNING/SUCCEEDED/FAILED 수) |
| `c4_gpu_status` | GPU 현황 조회 |

**특징**: `metrics.json` 자동 로드 (workdir 기준), `exp_id`/`tags`로 실험 분류 가능.

```
c4_job_submit(command="python train.py", exp_id="exp-001", tags=["gpu", "resnet"])
c4_job_status(job_id="j-...")   # 로그 + 메트릭 확인
c4_job_summary()                # 전체 큐 현황
```

### Hub — 원격/분산 작업 스케줄링 (C5 Hub 서버 필요)

원격 GPU 서버 또는 다중 워커가 필요한 경우 사용합니다.

| 도구 | 용도 |
|------|------|
| `c4_hub_submit` | 작업 제출 (command, GPU 요청) |
| `c4_hub_watch` | 실행 중 로그 확인 |
| `c4_hub_summary` | 완료 작업 종합 (상태, 메트릭, 로그) |
| `c4_hub_dag_from_yaml` | YAML로 다단계 파이프라인 정의 |
| `c4_hub_dag_execute` | DAG 실행 (의존성 순서, 병렬 가능 시 병렬) |
| `c4_hub_metrics` | 학습 메트릭 조회 (loss, accuracy 등) |

**언제 씀**: ML 실험, 장시간 빌드, GPU 필요 작업. 시나리오 F 참조.

### Code Intelligence — 심볼 단위 코드 조작

LSP(Multilspy/Jedi/Tree-sitter) 기반. 워커/에이전트가 내부적으로 사용.

| 도구 | 용도 |
|------|------|
| `c4_find_symbol` | 프로젝트 전체에서 심볼 정의 검색 |
| `c4_get_symbols_overview` | 파일 내 모든 심볼 개요 |
| `c4_replace_symbol_body` | 함수/클래스 본문 교체 |
| `c4_rename_symbol` | 모든 참조를 포함한 심볼 이름 변경 |
| `c4_find_referencing_symbols` | 심볼을 참조하는 모든 코드 검색 |

**언제 씀**: 리팩토링, 대규모 이름 변경. 보통 에이전트가 자동 사용.

### Cloud — 팀 협업 & 동기화

Supabase 기반. 여러 사람이 같은 프로젝트를 관리할 때.

- 인증, 프로젝트 동기화, 팀 대시보드
- C1 Desktop App의 Team 뷰에서 시각화
- 설정: `.c4/config.yaml`에서 `cloud.enabled: true`

### CDP Runner — 브라우저 자동화

Chromium 앱을 DevTools Protocol로 제어합니다.

```bash
c4_cdp_run(script="document.title")     # JS 실행
c4_cdp_list()                            # 열린 탭 목록
```

**언제 씀**: 웹 앱 테스트, UI 스크린샷, 데이터 스크래핑.

### C2 Document Lifecycle — 문서 처리

HWP, DOCX, PDF, XLSX, PPTX 파일을 파싱/분석합니다.

| 도구 | 용도 |
|------|------|
| `c4_parse_document` | 구조화된 블록 파싱 (테이블, 이미지 포함) |
| `c4_extract_text` | 평문 텍스트 추출 |
| `c4_workspace_create/load/save` | 글쓰기 워크스페이스 관리 |
| `c4_persona_learn` | AI 초안 vs 사용자 최종본 비교 → 문체 학습 |
| `c4_profile_load/save` | 사용자 글쓰기 프로필 관리 |

**언제 씀**: 논문 리뷰(`/c4-review`), 보고서 분석, 문체 커스터마이징.

### Digital Twin — 성찰 & 성장 추적

```bash
c4_reflect(focus="all")   # 패턴 분석, 성장 추적, 도전 과제 식별
```

**언제 씀**: 프로젝트 회고, 작업 패턴 파악, 약점 진단.

### LLM Gateway — 다중 모델 라우팅

```bash
c4_llm_call(prompt="...", provider="anthropic")  # Claude/GPT/Gemini/Ollama
c4_llm_providers()                                 # 사용 가능 프로바이더 목록
c4_llm_costs()                                     # 누적 비용 추적
```

**언제 씀**: 모델 비교, 비용 모니터링. Economic Mode 프리셋이 자동으로 라우팅.

---

## 7. 도구 우선순위 종합

103개 도구를 다 외울 필요 없습니다.

### 3계층 분류

```
━━━ 매일 (6개) ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  /c4-status, /c4-quick, /c4-run, /c4-submit,
  /c4-validate, c4_claim/c4_report

━━━ 주간 (8개) ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  /c4-plan, /c4-add-task, /c4-checkpoint, /c4-swarm,
  /c4-stop, c4_knowledge_record, c4_knowledge_search,
  c4_reflect

━━━ 가끔 (8개) ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  /c4-interview, /c4-release, /c4-research,
  /c4-review, c4_lighthouse, c4_hub_submit,
  c4_hub_dag_from_yaml, c4_llm_costs

━━━ 나머지 80+개 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  에이전트/워커가 내부적으로 사용
```

---

## 8. 지식 & 세션 관리

### 기록해야 할 때

| 상황 | 방법 |
|------|------|
| 비자명한 패턴 발견 | `c4_knowledge_record(type="insight", content="...")` |
| 까다로운 버그 해결 | 원인 + 해결법을 insight로 기록 |
| 장시간 세션 종료 | 발견 사항 + 미해결 이슈 + 다음 시작점 기록 |
| 실험 결과 | `c4_experiment_record(title="...", content="...")` |
| 반복되는 패턴 | `c4_knowledge_record(type="pattern", content="...")` |

### 기록하지 말아야 할 때

- 문서에 이미 있는 내용
- 일회성 수정 (타이포 등)
- 자명한 패턴 (공식 문서에 나오는 것)

### 지식 활용

```bash
c4_knowledge_search(query="SQLite deadlock")  # 과거 해결 사례 검색
c4_pattern_suggest(context="...")              # 현재 상황에 맞는 패턴 제안
```

---

## 9. 파워유저 패턴

### Lighthouse TDD — API 계약 선행 개발

새 MCP 도구를 만들 때 스펙부터 정의하고 구현합니다.

```bash
c4_lighthouse(action="register", name="my_tool",
  description="...", input_schema='...', spec="...")
# → 스텁 도구 즉시 사용 가능 + 구현 태스크 자동 생성

# 구현 후
c4_lighthouse(action="promote", name="my_tool")
# → 스키마 검증 → 스텁 제거 → 태스크 완료
```

### Economic Mode — 비용 최적화

`.c4/config.yaml`에서 프리셋을 바꿔 모델 비용을 조절합니다.

| 프리셋 | 구현 | 리뷰 | 체크포인트 | Scout |
|--------|:----:|:----:|:----------:|:-----:|
| **standard** | Sonnet | Opus | Opus | Haiku |
| **economic** | Sonnet | Sonnet | Sonnet | Haiku |
| **ultra-economic** | Haiku | Sonnet | Sonnet | Haiku |
| **quality** | Opus | Opus | Opus | Sonnet |

```yaml
# .c4/config.yaml
economic_mode:
  preset: economic    # 비용 민감한 반복 작업에 적합
```

### Soul 커스터마이징 — 리뷰 기준 맞춤

`.c4/SOUL.md`를 편집하면 C4의 리뷰 판단 기준이 바뀝니다.

- **비타협 규칙** 수정 → 테스트/DoD 강도 조절
- **리뷰 우선순위** 변경 → 보안 vs 성능 vs 가독성 가중치
- **작업 패킷 표준** 수정 → 태스크 생성 시 요구 항목 변경

### Discovery 워크플로우 — 설계 문서화

`/c4-plan`이 내부적으로 사용하는 Spec/Design 관리를 직접 다룰 수도 있습니다.

```bash
# Discovery 단계: 요구사항 정리
c4_save_spec(name="auth-v2", content="# Auth v2\n...")
c4_discovery_complete()

# Design 단계: 설계 결정 문서화
c4_save_design(name="auth-v2-design", content="# Design\n...")
c4_design_complete()

# → PLAN 단계로 자동 전환
```

### Persona Evolution — 에이전트 행동 학습

에이전트의 리뷰 결과(승인/거부)를 분석해서 행동을 자동 조정합니다.

```bash
c4_persona_stats(persona_id="code-reviewer")   # 승인/거부 비율 확인
c4_persona_evolve(persona_id="code-reviewer")   # 학습된 패턴을 Soul에 반영
```

### Config Reference — `.c4/config.yaml` 전체 키

```yaml
# === 프로젝트 ===
project_id: string          # 프로젝트 식별자 (기본: 폴더 이름)
default_branch: string      # 기본 브랜치 (기본: main)
work_branch_prefix: string  # Worker 브랜치 프리픽스 (기본: c4/w-)
domain: string              # 프로젝트 도메인 (에이전트 선택에 영향)

# === 태스크 ===
review_as_task: bool        # 자동 R-task 생성 (기본: true)
checkpoint_as_task: bool    # 자동 CP-task 생성 (기본: true)
max_revision: int           # REQUEST_CHANGES 최대 횟수 (기본: 3)

# === 모델 라우팅 ===
economic_mode:
  preset: standard|economic|ultra-economic|quality
  model_routing:            # preset 오버라이드
    implementation: sonnet|opus|haiku
    review: sonnet|opus|haiku
    checkpoint: sonnet|opus|haiku
    scout: sonnet|opus|haiku

# === 검증 ===
validation:
  <name>: <command>         # /c4-validate에서 실행할 커맨드
  # 예: go-test: "cd c4-core && go test ./..."
  # 예: pytest: "uv run pytest tests/"

# === 통합 ===
cloud:
  enabled: bool             # Supabase 동기화 (기본: false)
  project_id: string        # Supabase 프로젝트 ID

hub:
  enabled: bool             # PiQ Hub 연결 (기본: false)
  url: string               # Hub API URL
  api_prefix: string        # API 경로 프리픽스
  team_id: string           # 팀 식별자

worktree:
  enabled: bool             # Git worktree 사용 (기본: true)
  auto_cleanup: bool        # 완료 후 자동 정리 (기본: true)
```

---

## 10. 체크리스트

### 세션 시작

```
☐ /c4-status 로 현재 상태 확인
☐ 진행 중인 태스크가 있으면 이어서 작업
☐ 없으면 의사결정 트리(§1) 따라 시작
```

### 작업 중

```
☐ 수정 후 반드시 검증 (Go: go build/vet, Python: py_compile/pytest)
☐ c4_submit 전에 c4_status로 태스크 상태 확인
☐ pending 상태면 submit 금지 → c4_get_task로 먼저 할당
```

### 세션 종료

```
☐ 진행 중인 태스크 상태 정리
☐ 비자명한 발견 → c4_knowledge_record
☐ 미해결 이슈 있으면 기록
☐ (선택) c4_reflect로 패턴 분석
```

### 프로젝트 완료

```
☐ /c4-release 로 체인지로그 생성
☐ c4_reflect(focus="all")로 회고
☐ 학습 사항을 knowledge에 기록
```

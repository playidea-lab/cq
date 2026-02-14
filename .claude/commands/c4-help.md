# C4 Help

커맨드, 에이전트, 도구에 대한 빠른 참조를 제공합니다.

## Usage

```
/c4-help              → 전체 요약
/c4-help commands      → 커맨드 전체 레퍼런스
/c4-help agents        → 에이전트 카테고리별 목록
/c4-help tools         → MCP 도구 3계층 분류
/c4-help <keyword>     → 키워드 검색
```

## Instructions

`$ARGUMENTS`를 파싱해서 분기합니다. MCP 호출 불필요 (정적 텍스트 출력).

---

### 분기 1: 인자 없음 (`$ARGUMENTS`가 비어있을 때) → 전체 요약

다음 내용을 그대로 출력하세요:

```
## C4 Quick Reference

### 의사결정 트리

작업이 뭔가?
├─ 1줄 수정 → 그냥 고쳐 (C4 불필요)
├─ 소규모 (1-5파일) → /c4-quick "설명" → /c4-submit
├─ 중규모 (5-15파일) → /c4-add-task → /c4-run  또는  c4_claim → c4_report
├─ 대규모 (15+파일) → /c4-plan → /c4-run N  또는  /c4-swarm N
├─ 연구/실험 → /c4-research
└─ 문서 작업 → /c4-review (논문) 또는 c4_parse_document

### 핵심 커맨드 (매일)

| 커맨드 | 용도 | 인자 |
|--------|------|------|
| /c4-status | 현황 파악 | (없음) |
| /c4-quick | 즉시 작업 시작 | "설명" |
| /c4-run | Worker 병렬 실행 | [N] [--continuous] |
| /c4-submit | 완료 제출 | [task-id] |
| /c4-validate | 검증 실행 | (없음) |

### 자세히 보기

- /c4-help commands  → 16개 커맨드 전체 레퍼런스
- /c4-help agents   → 37개 에이전트 카테고리별 목록
- /c4-help tools    → 103개 MCP 도구 분류
- /c4-help <keyword> → 키워드 검색 (예: /c4-help review)
```

---

### 분기 2: `$ARGUMENTS` = "commands" → 커맨드 전체 레퍼런스

다음 내용을 출력하세요:

```
## C4 Commands (16개)

### 매일 (6개)

| 커맨드 | 용도 | 인자 형식 | 예시 |
|--------|------|----------|------|
| /c4-status | 현황 파악 | (없음) | /c4-status |
| /c4-quick | 즉시 작업 시작 | "설명" [scope=경로] | /c4-quick "fix: timeout bug" |
| /c4-run | Worker 병렬 실행 | [N] [--continuous] [--max N] | /c4-run 3 |
| /c4-submit | 완료 제출 | [task-id] | /c4-submit T-001 |
| /c4-validate | 검증 실행 | (없음) | /c4-validate |
| c4_claim/report | Direct 모드 | task_id (MCP 도구) | c4_claim("T-001-0") |

### 주간 (5개)

| 커맨드 | 용도 | 인자 형식 | 예시 |
|--------|------|----------|------|
| /c4-plan | 대규모 기획 | (없음) | /c4-plan |
| /c4-add-task | 태스크 수동 추가 | "설명" [--domain D] | /c4-add-task "JWT 인증" |
| /c4-checkpoint | 단계별 리뷰 | (없음) | /c4-checkpoint |
| /c4-swarm | 팀 협업 | [N] [--review] [--investigate] | /c4-swarm --review |
| /c4-stop | 실행 중단 | (없음) | /c4-stop |

### 가끔 (5개)

| 커맨드 | 용도 | 인자 형식 | 예시 |
|--------|------|----------|------|
| /c4-interview | 심층 인터뷰 | "주제" | /c4-interview "실시간 동기화" |
| /c4-release | 체인지로그 생성 | (없음) | /c4-release |
| /c4-research | 연구 반복 | [start\|status\|next\|record\|approve] | /c4-research next |
| /c4-review | 논문 리뷰 | <pdf_path> | /c4-review paper.pdf |
| /c4-init | 프로젝트 초기화 | (없음) | /c4-init |

### 운영 (1개)

| 커맨드 | 용도 | 인자 형식 | 주의 |
|--------|------|----------|------|
| /c4-clear | 상태 완전 초기화 | (없음) | 되돌릴 수 없음 |
```

---

### 분기 3: `$ARGUMENTS` = "agents" → 에이전트 카테고리별 목록

다음 내용을 출력하세요:

```
## C4 Agents (37개, 9개 카테고리)

### Backend (3)
| 에이전트 | 전문 영역 |
|----------|-----------|
| backend-architect | REST API, 마이크로서비스, DB 스키마 설계 |
| database-optimizer | 쿼리 튜닝, 인덱스 설계, 캐싱 전략 |
| graphql-architect | GraphQL 스키마, N+1 해결, 서브스크립션 |

### Frontend (3)
| 에이전트 | 전문 영역 |
|----------|-----------|
| frontend-developer | React 컴포넌트, 상태 관리, 접근성 (스펙→코드) |
| frontend-designer | 디자인→스펙 변환, 디자인 시스템 생성 |
| react-pro | React 19+, Server Components, 렌더링 최적화 |

### DevOps/Infra (3)
| 에이전트 | 전문 영역 |
|----------|-----------|
| deployment-engineer | CI/CD 파이프라인 구축, Docker, K8s |
| devops-troubleshooter | 프로덕션 장애 디버깅, 로그 분석 |
| cloud-architect | AWS/GCP/Azure 인프라, Terraform IaC, 비용 최적화 |

### Quality (4)
| 에이전트 | 전문 영역 |
|----------|-----------|
| code-reviewer | 읽기 전용 코드 리뷰, 테스트 커버리지 검증 |
| code-refactorer | 코드 구조 개선, 리팩토링 실행 |
| security-auditor | 취약점 탐지, 인증/인가 검증 |
| test-automator | unit/integration/e2e 테스트, CI 설정 |

### Data/ML (4)
| 에이전트 | 전문 영역 |
|----------|-----------|
| data-engineer | ETL, 데이터 웨어하우스, Spark/Airflow/Kafka |
| data-scientist | SQL, BigQuery, 데이터 인사이트 |
| ml-engineer | 모델 서빙, 피처 엔지니어링, A/B 테스트 |
| ai-engineer | LLM 앱, RAG, 벡터 검색, 에이전트 오케스트레이션 |

### Languages (4)
| 에이전트 | 전문 영역 |
|----------|-----------|
| golang-pro | goroutine, channel, 에러 처리, 성능 최적화 |
| python-pro | decorator, async, 테스트, 성능 최적화 |
| rust-pro | ownership, lifetime, trait, async, unsafe |
| tauri-developer | Tauri 2.x, Rust-JS bridge, 데스크톱 앱 |

### Documentation (4)
| 에이전트 | 전문 영역 |
|----------|-----------|
| api-documenter | OpenAPI/Swagger, SDK 생성, 개발자 문서 |
| content-writer | 기술 블로그, 문서 작성 |
| prd-writer | PRD 작성, 스토리 분해, 검증 체크포인트 |
| prompt-engineer | LLM 프롬프트 최적화, 시스템 프롬프트 설계 |

### Project (4)
| 에이전트 | 전문 영역 |
|----------|-----------|
| workflow-orchestrator | 개발 라이프사이클 조율, 에이전트 협업 |
| project-task-planner | PRD→태스크 리스트 생성 |
| context-manager | 다중 에이전트 컨텍스트 관리 |
| memory-bank | 프로젝트 지식 저장/검색 |

### Specialty (8)
| 에이전트 | 전문 영역 |
|----------|-----------|
| debugger | 개발 중 버그, 테스트 실패 디버깅 |
| incident-responder | 프로덕션 장애 대응, 포스트모템 |
| performance-engineer | 프로파일링, 병목 최적화, 캐싱 |
| legacy-modernizer | 레거시 리팩토링, 프레임워크 마이그레이션 |
| mobile-developer | React Native/Flutter, 네이티브 통합 |
| dx-optimizer | 개발자 경험 개선, 도구/워크플로우 |
| payment-integration | Stripe/PayPal, 구독/웹훅, PCI 준수 |
| math-teacher | 수학 증명, 엄밀한 풀이 |

### 내부 전용 (3)
| 에이전트 | 전문 영역 |
|----------|-----------|
| architect-review | PRD/아키텍처 검토 (/c4-plan 내부) |
| c4-scout | 코드베이스 빠른 탐색 (500토큰 압축) |
| vibe-coding-coach | 비전→앱 구현, 대화형 앱 빌딩 |

### 혼동하기 쉬운 에이전트 구분

| 쌍 | 구분 |
|----|------|
| debugger vs incident-responder | 개발 중 버그 vs 프로덕션 장애 |
| frontend-designer vs frontend-developer | 디자인→스펙 vs 스펙→코드 |
| backend-architect vs cloud-architect | API/DB 설계 vs 인프라/IaC |
| code-reviewer vs code-refactorer | 읽기 전용 리뷰 vs 코드 변경 실행 |
| deployment-engineer vs devops-troubleshooter | CI/CD 구축 vs 장애 디버깅 |
```

---

### 분기 4: `$ARGUMENTS` = "tools" → MCP 도구 3계층 분류

다음 내용을 출력하세요:

```
## C4 MCP Tools (103개, 3계층)

### Layer 1: 매일 쓰는 도구 (6개)
직접 호출하거나 커맨드를 통해 사용.

| 도구 | 용도 |
|------|------|
| /c4-status | 프로젝트 상태 확인 |
| /c4-quick "desc" | 즉시 작업 시작 |
| /c4-run [N] | Worker 병렬 실행 |
| /c4-submit | 완료 제출 |
| /c4-validate | lint + test 실행 |
| c4_claim / c4_report | Direct 모드 작업 |

### Layer 2: 주간/상황별 도구 (16개)
특정 시나리오에서 직접 호출.

| 카테고리 | 도구 |
|----------|------|
| 계획 | /c4-plan, /c4-add-task, /c4-interview |
| 리뷰 | /c4-checkpoint, /c4-swarm --review |
| 연구 | /c4-research, /c4-review |
| 지식 | c4_knowledge_record, c4_knowledge_search, c4_pattern_suggest |
| 성찰 | c4_reflect |
| Hub | c4_hub_submit, c4_hub_watch, c4_hub_summary |
| Lighthouse | c4_lighthouse |
| 비용 | c4_llm_costs |

### Layer 3: 내부 도구 (80+개)
에이전트/워커가 자동 사용. 직접 호출 거의 불필요.

| 카테고리 | 도구 수 | 예시 |
|----------|:-------:|------|
| 태스크 관리 | 6 | c4_add_todo, c4_get_task, c4_submit |
| 파일/검색 | 6 | c4_find_file, c4_read_file, c4_search_for_pattern |
| Git | 4 | c4_worktree_status, c4_analyze_history |
| LSP/심볼 | 7 | c4_find_symbol, c4_replace_symbol_body |
| Discovery | 8 | c4_save_spec, c4_save_design |
| Artifact | 3 | c4_artifact_save, c4_artifact_get |
| Soul/Persona | 7 | c4_soul_get, c4_persona_evolve |
| LLM Gateway | 3 | c4_llm_call, c4_llm_providers |
| CDP | 2 | c4_cdp_run, c4_cdp_list |
| C2 문서 | 8 | c4_parse_document, c4_extract_text |
| Hub (전체) | 26 | Job, DAG, Edge, Deploy, Artifact |
| 기타 | 5 | c4_onboard, c4_run_validation, c4_clear |
```

---

### 분기 5: 그 외 `$ARGUMENTS` → 키워드 검색

`$ARGUMENTS`를 키워드로 취급하여, 아래 데이터에서 키워드가 포함된 항목을 찾아 출력하세요.

**검색 대상 데이터**:

커맨드:
- c4-status: 현황 파악, 상태 확인
- c4-quick: 즉시 작업, 빠른 시작, 소규모
- c4-run: Worker 병렬 실행, 독립 태스크
- c4-submit: 완료 제출, 작업 완료
- c4-validate: 검증, lint, test
- c4-plan: 대규모 기획, 설계, Discovery
- c4-add-task: 태스크 추가, 수동 생성
- c4-checkpoint: 리뷰, 검토, 단계별
- c4-swarm: 팀 협업, 병렬, review, investigate
- c4-stop: 중단, 일시정지, HALTED
- c4-interview: 인터뷰, 요구사항 탐색
- c4-release: 릴리즈, 체인지로그
- c4-research: 연구, 논문, 실험, 반복 개선
- c4-review: 논문 리뷰, 학술, 6축 분석, PDF
- c4-init: 초기화, 프로젝트 시작
- c4-clear: 초기화, 상태 삭제, 리셋

에이전트:
- backend-architect: API, DB, 마이크로서비스, REST, 스키마
- frontend-developer: React, 컴포넌트, 상태관리, UI, 접근성
- frontend-designer: 디자인, 목업, 와이어프레임, 디자인시스템
- golang-pro: Go, goroutine, channel, 동시성
- python-pro: Python, decorator, async, 성능
- rust-pro: Rust, ownership, lifetime, trait
- debugger: 버그, 디버깅, 테스트 실패, 에러
- incident-responder: 프로덕션 장애, 긴급, 다운타임
- code-reviewer: 코드 리뷰, 품질, 보안
- code-refactorer: 리팩토링, 구조개선, 중복제거
- security-auditor: 보안, 취약점, 인증, OWASP
- test-automator: 테스트, CI, 자동화, 커버리지
- deployment-engineer: CI/CD, Docker, K8s, 배포
- devops-troubleshooter: 장애, 로그, 모니터링
- cloud-architect: AWS, GCP, Azure, Terraform, 인프라
- database-optimizer: 쿼리, 인덱스, 캐싱, DB 성능
- ml-engineer: ML, 모델, 피처, 서빙
- ai-engineer: LLM, RAG, 벡터, 프롬프트
- data-engineer: ETL, 파이프라인, Spark, Kafka
- data-scientist: 데이터 분석, SQL, BigQuery
- performance-engineer: 성능, 프로파일링, 병목, 캐싱
- mobile-developer: React Native, Flutter, 앱
- tauri-developer: Tauri, 데스크톱, Rust-JS bridge

도구:
- c4_knowledge_record: 지식 기록, 인사이트, 패턴
- c4_knowledge_search: 지식 검색, 과거 사례
- c4_reflect: 성찰, 패턴 분석, 회고
- c4_hub_submit: GPU 작업, 학습, 원격 실행
- c4_hub_dag_from_yaml: 파이프라인, DAG, 다단계
- c4_lighthouse: API 계약, 스텁, TDD, 스펙 선행
- c4_llm_costs: 비용, 모델, 라우팅
- c4_parse_document: 문서, HWP, DOCX, PDF, PPTX
- c4_extract_text: 텍스트 추출
- c4_cdp_run: 브라우저, 자동화, CDP

**출력 형식**:

```
## /c4-help "{keyword}" 검색 결과

### 관련 커맨드
- /c4-xxx: 설명

### 관련 에이전트
- agent-name: 설명

### 관련 도구
- c4_xxx: 설명
```

매칭 결과가 없으면:
```
"{keyword}"에 대한 결과가 없습니다.
/c4-help 으로 전체 목록을 확인하세요.
```

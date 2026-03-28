# MCP 도구 레퍼런스

CQ는 AI Agent(Claude Code, Cursor, Codex CLI, Gemini, External Brain 경유 ChatGPT)에 **169개의 MCP 도구**를 노출합니다. 도구는 기능에 따라 카테고리로 구성됩니다.

**도구 티어:**
- **Core** (40개 도구) — 항상 로드됨, 즉시 사용 가능
- **Extended** (129개 도구) — 요청 시 로드됨, MCP 초기화 후 사용 가능

---

## 프로젝트 및 상태 관리 (Core)

C4 프로젝트 생명주기, 상태 머신, 태스크 큐를 관리하는 도구.

| 도구 | 설명 |
|------|------|
| `c4_status` | 현재 프로젝트 상태, 태스크 수, 활성 Worker 표시 |
| `c4_start` | C4 프로젝트 초기화 (`.c4/` 데이터베이스, 설정 생성) |
| `c4_stop` | HALTED 상태로 전환, 현재 진행 상황 보존 |
| `c4_get_task` | 큐에서 다음 태스크 요청 (Worker 모드) |
| `c4_submit` | 커밋 SHA와 유효성 검사 결과와 함께 완료된 태스크 제출 |
| `c4_claim` | 직접 구현을 위해 태스크 클레임 (Direct 모드) |
| `c4_report` | 태스크 완료 보고 (Direct 모드) |
| `c4_mark_blocked` | 태스크를 이유와 함께 차단됨으로 표시 |
| `c4_request_changes` | 제출된 태스크에 변경 요청 |
| `c4_add_todo` | 큐에 새 태스크 추가 |
| `c4_task_list` | 선택적 필터로 태스크 목록 (상태, 도메인, id) |
| `c4_stale_tasks` | `in_progress` 상태에 멈춘 태스크 표시 |
| `c4_worker_heartbeat` | 리스를 유지하기 위한 Worker 하트비트 전송 |
| `c4_workers` | 활성 Worker와 현재 태스크 목록 |
| `c4_dashboard` | 프로젝트 상태 요약 위젯 (`format=widget`으로 UI) |
| `c4_task_graph` | 모든 태스크의 시각적 의존성 그래프 (`format=widget`으로 UI) |

---

## 파일 작업 (Core)

코드베이스를 읽고, 검색하고, 탐색하는 도구.

| 도구 | 설명 |
|------|------|
| `c4_read_file` | 줄 번호와 선택적 범위로 파일 읽기 |
| `c4_find_file` | 이름 패턴으로 파일 퍼지 검색 |
| `c4_search_for_pattern` | 파일 전체에서 정규식/리터럴 검색 (ripgrep 기반) |
| `c4_list_dir` | 메타데이터와 함께 디렉토리 내용 나열 |
| `c4_create_text_file` | 텍스트 파일 생성 또는 덮어쓰기 |
| `c4_replace_content` | 파일 내 정교한 검색-교체 |
| `c4_diff_summary` | 스테이징됨 + 스테이징 안됨 git diff 요약 표시 (`format=widget`으로 UI) |
| `c4_execute` | 셸 커맨드 실행 (권한 훅으로 게이트됨) |

---

## Git 작업 (Core)

| 도구 | 설명 |
|------|------|
| `c4_git_log` | Conventional Commits 파싱으로 최근 커밋 표시 |
| `c4_git_status` | 작업 트리 상태 표시 |
| `c4_git_diff` | 리비전 또는 파일 간 diff 표시 |

---

## 유효성 검사 및 테스트 (Core)

| 도구 | 설명 |
|------|------|
| `c4_run_validation` | lint + 테스트 실행 (자동 감지: go test, pytest, cargo test, pnpm test). 심각도: CRITICAL/HIGH/MEDIUM/LOW (`format=widget`으로 UI) |
| `c4_error_trace` | 디버깅용 오류 트레이스 형식화 및 표시 (`format=widget`으로 UI) |

---

## 설정 및 상태 (Core)

| 도구 | 설명 |
|------|------|
| `c4_config_get` | `.c4/config.yaml`에서 설정 값 읽기 |
| `c4_health` | 모든 CQ 컴포넌트 상태 확인 (Doctor 동등) |
| `c4_version` | CQ 바이너리 버전과 빌드 태그 표시 |

---

## 스펙 및 설계 (Extended)

구조화된 명세와 아키텍처 결정 기록을 위한 도구.

| 도구 | 설명 |
|------|------|
| `c4_get_spec` | `docs/specs/`에서 스펙 문서 읽기 |
| `c4_save_spec` | 스펙 문서 작성 또는 업데이트 |
| `c4_list_specs` | 메타데이터와 함께 모든 스펙 목록 |
| `c4_get_design` | 아키텍처 설계 문서 읽기 |
| `c4_save_design` | 설계 문서 작성 또는 업데이트 |
| `c4_list_designs` | 모든 설계 문서 목록 |
| `c4_discovery_complete` | Discovery 단계 완료 표시, 설계로 전환 |
| `c4_design_complete` | Design 단계 완료 표시, 계획으로 전환 |

---

## Lighthouse 및 체크포인트 (Extended)

| 도구 | 설명 |
|------|------|
| `c4_lighthouse` | 구현이 Lighthouse 계약과 일치하는지 확인 |
| `c4_checkpoint` | 수동 체크포인트 리뷰 트리거 |
| `c4_artifact_save` | 콘텐츠 해시와 함께 빌드 아티팩트 저장 |
| `c4_artifact_load` | 이전에 저장된 아티팩트 로드 |

---

## 지식 (Extended)

> 클라우드 동기화를 위해 `connected` 또는 `full` 티어 필요. FTS5 + pgvector (1536-dim OpenAI) + 3-way RRF.

| 도구 | 설명 |
|------|------|
| `c4_knowledge_record` | 지식 저장 (결정, 패턴, 인사이트, 오류, 발견). AI 자동 캡처: 도구 설명이 자율 저장을 트리거. |
| `c4_knowledge_search` | 지식 검색: 벡터 + FTS + ilike 3단계 폴백. 순위 결과 반환. |
| `c4_knowledge_distill` | 문서 수 ≥ 5일 때 자동 지식 증류 (클러스터 + 요약) |
| `c4_knowledge_ingest` | 파일 또는 URL을 지식 저장소에 일괄 수집 |
| `c4_knowledge_sync` | 로컬 지식을 Supabase 클라우드와 동기화 |
| `c4_knowledge_publish` | 공유 프로젝트 네임스페이스에 지식 발행 |
| `c4_knowledge_pull` | 다른 프로젝트에서 발행된 지식 가져오기 |
| `c4_knowledge_usage` | 지식 접근 통계 표시 |
| `c4_knowledge_embed` | 텍스트의 임베딩 생성 (저장 없이) |
| `c4_knowledge_chunk` | 오버랩이 있는 임베딩용 문서 청킹 |
| `c4_pattern_suggest` | 지식 베이스에서 구현 패턴 제안 |
| `c4_knowledge_feed` | 실시간 지식 피드 위젯 (`format=widget`으로 UI) |
| `c4_knowledge_status` | 지식 저장소 통계 표시 (수, 마지막 동기화, 커버리지) |

---

## 세션 인텔리전스 (Extended)

| 도구 | 설명 |
|------|------|
| `c4_session_index` | 요약과 타임스탬프가 있는 모든 세션 목록 |
| `c4_session_summarize` | LLM을 사용하여 현재 세션 요약 (안전망 캡처) |
| `c4_session_snapshot` | 수동 컨텍스트 스냅샷 생성 |
| `c4_session_recall` | 이전 세션의 컨텍스트 불러오기 |
| `c4_session_summary` | 대화 종료 시 완전한 세션 요약 캡처 |

---

## 메모리 (Extended)

| 도구 | 설명 |
|------|------|
| `c4_memory_import` | ChatGPT 또는 Claude 대화 내보내기를 지식 저장소로 가져오기 |

---

## Drive (Extended)

> TUS 재개 가능 업로드가 있는 클라우드 파일 스토리지. `connected` 또는 `full` 티어 필요.

| 도구 | 설명 |
|------|------|
| `c4_drive_upload` | Drive에 파일 업로드 (TUS 재개 가능, 내용 주소 지정) |
| `c4_drive_download` | Drive에서 파일 다운로드 (Range 기반 재개) |
| `c4_drive_list` | 메타데이터와 함께 Drive 파일 목록 |
| `c4_drive_delete` | Drive에서 파일 삭제 |
| `c4_drive_info` | 파일 메타데이터 가져오기 (크기, 해시, URL) |
| `c4_drive_mkdir` | Drive에 디렉토리 생성 |

---

## 파일 인덱스 (Extended)

> 크로스 디바이스 파일 검색 — 연결된 모든 머신에서 파일 찾기.

| 도구 | 설명 |
|------|------|
| `c4_fileindex_search` | 모든 인덱싱된 디바이스에서 이름/경로로 파일 검색 |
| `c4_fileindex_status` | 파일 인덱스 커버리지와 마지막 업데이트 시간 표시 |

---

## Relay (Extended)

| 도구 | 설명 |
|------|------|
| `cq_workers` | 지연 시간과 터널 정보와 함께 relay를 통해 연결된 Worker 목록 |
| `cq_relay_call` | relay를 통해 원격 Worker의 MCP 도구 호출 |

---

## LLM Gateway (Extended)

> `llm_gateway` 빌드 태그 필요. 프롬프트 캐싱이 있는 멀티 프로바이더.

| 도구 | 설명 |
|------|------|
| `c4_llm_call` | 프로바이더 라우팅으로 LLM 호출 (Anthropic/OpenAI/Gemini/Ollama) |
| `c4_llm_providers` | 설정된 LLM 프로바이더와 가용성 목록 |
| `c4_llm_costs` | LLM 비용 분석 및 캐시 절약 표시 (`format=widget`으로 UI) |
| `c4_llm_usage_stats` | 세션/프로젝트별 상세 토큰 사용 통계 |

---

## Hub — 분산 작업 (조건부, Hub 티어)

> Hub 연결 필요 (`serve.hub.enabled: true` + 클라우드 자격증명).

### 작업 작업

| 도구 | 설명 |
|------|------|
| `c4_job_submit` | 스펙과 라우팅으로 Hub 큐에 작업 제출 |
| `c4_job_status` | 작업 상태와 진행률 가져오기 (`format=widget`으로 UI) |
| `c4_job_summary` | 작업 결과와 출력 아티팩트 가져오기 (`format=widget`으로 UI) |
| `c4_job_cancel` | 실행 중이거나 대기 중인 작업 취소 |
| `c4_job_list` | 상태와 메트릭이 있는 최근 작업 목록 |
| `c4_job_logs` | 작업 실행 로그 스트림 |

### Worker 작업

| 도구 | 설명 |
|------|------|
| `c4_hub_workers` | affinity 점수와 현재 부하가 있는 Worker 목록 |
| `c4_hub_worker_status` | 상세 Worker 상태와 능력 가져오기 |
| `c4_hub_worker_tags` | 라우팅을 위한 Worker 태그 업데이트 |
| `c4_nodes_map` | 연결된 모든 노드의 시각적 맵 (`format=widget`으로 UI) |

### DAG 파이프라인

| 도구 | 설명 |
|------|------|
| `c4_hub_dag_create` | 노드와 엣지로 DAG 파이프라인 생성 |
| `c4_hub_dag_status` | 노드별 진행률이 있는 DAG 실행 상태 가져오기 |
| `c4_hub_dag_list` | 모든 DAG 목록 |
| `c4_hub_dag_cancel` | 실행 중인 DAG 취소 |

### 아티팩트

| 도구 | 설명 |
|------|------|
| `c4_hub_artifact_upload` | Hub 스토리지에 아티팩트 업로드 |
| `c4_hub_artifact_download` | Hub에서 아티팩트 다운로드 |
| `c4_hub_artifact_list` | 작업의 아티팩트 목록 |

### Cron 스케줄링

| 도구 | 설명 |
|------|------|
| `c4_cron_create` | 작업 스펙으로 cron 스케줄 등록 |
| `c4_cron_list` | 마지막 실행 시간이 있는 cron 스케줄 목록 |
| `c4_cron_delete` | cron 스케줄 삭제 |

### Worker 대기 모드 (Hub 조건부)

| 도구 | 설명 |
|------|------|
| `c4_worker_standby` | 대기 모드 진입 — Hub 작업을 기다리고 실행 |
| `c4_worker_complete` | 대기 Worker에서 작업 완료 신호 |
| `c4_worker_shutdown` | 태스크 핸드오프로 Worker 정상 종료 |

---

## 실험 (Extended)

| 도구 | 설명 |
|------|------|
| `c4_experiment_record` | 실험 결과 기록 (exp_id, 메트릭, 설정) |
| `c4_experiment_search` | 메트릭, 날짜, 태그로 실험 검색 (`format=widget`으로 UI) |
| `c4_experiment_compare` | 두 개 이상의 실험 나란히 비교 |

---

## Soul 및 Persona (Extended)

| 도구 | 설명 |
|------|------|
| `c4_soul_evolve` | 결과에 기반하여 soul/판단 기준 업데이트 |
| `c4_soul_check` | 현재 soul에 대한 결정 평가 |
| `c4_persona_learn` | 학습된 동작 패턴 기록 |
| `c4_persona_learn_from_diff` | git diff에서 패턴 자동 추출 |
| `c4_persona_apply` | 현재 태스크에 persona 컨텍스트 적용 |
| `c4_persona_status` | 현재 persona 프로파일과 학습된 패턴 표시 |
| `c4_twin_record` | 디지털 트윈에 상호작용 기록 |
| `c4_twin_query` | 동작 예측을 위해 디지털 트윈 쿼리 |
| `c4_growth_loop` | Growth Loop 평가 사이클 트리거 |
| `c4_global_knowledge` | 로컬 persona 패턴을 전역 지식으로 푸시 |

---

## CDP (Chrome DevTools Protocol, Extended)

| 도구 | 설명 |
|------|------|
| `c4_cdp_run` | CDP를 통해 브라우저 탭에서 JavaScript 실행 |
| `c4_webmcp_discover` | 페이지에서 Web MCP 엔드포인트 자동 발견 |
| `c4_webmcp_call` | 발견된 Web MCP 도구 호출 |
| `c4_web_fetch` | 콘텐츠 협상 + HTML→Markdown 변환으로 URL 가져오기 |
| `c4_cdp_screenshot` | 현재 브라우저 상태의 스크린샷 찍기 |

---

## 알림 및 EventBus (Extended)

| 도구 | 설명 |
|------|------|
| `c4_notify` | 설정된 채널로 알림 전송 (Telegram/Dooray) |
| `c4_notification_channels` | 설정된 알림 채널 목록 |
| `c4_rule_add` | 선택적 알림 채널로 EventBus 규칙 추가 |
| `c4_rule_list` | 모든 EventBus 라우팅 규칙 목록 |
| `c4_event_publish` | EventBus에 이벤트 발행 |
| `c4_event_subscribe` | EventBus 이벤트 구독 (스트리밍) |
| `c4_mail_send` | 이름 있는 세션에 세션 간 메일 발송 |
| `c4_mail_list` | 메일 받은 편지함의 메시지 목록 |
| `c4_mail_read` | 특정 메일 메시지 읽기 |

---

## Skill 평가 (Extended)

| 도구 | 설명 |
|------|------|
| `c4_skill_eval_run` | Skill의 테스트 케이스에 k-trial haiku 분류 실행 |
| `c4_skill_eval_generate` | Skill의 긍정 + 부정 테스트 케이스 생성 |
| `c4_skill_eval_status` | 평가된 모든 Skill의 트리거 정확도 표시 (임계값: 0.90) |

---

## Research Loop (Extended)

| 도구 | 설명 |
|------|------|
| `c4_research_loop_start` | 자율 Research Loop 시작 (LoopOrchestrator) |
| `c4_research_loop_status` | 현재 루프 상태, 반복 횟수, 최적 메트릭 표시 |
| `c4_research_loop_stop` | Research Loop 정상 중지 |
| `c4_research_intervene` | 루프 방향을 조종하기 위한 수동 개입 |

---

## 관측성 — C7 Observe (조건부, `c7_observe` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `c4_observe_metrics` | 수집된 메트릭 쿼리 (도구 호출, 지연 시간, 오류율) |
| `c4_observe_logs` | 필터로 구조화된 로그 쿼리 |
| `c4_observe_trace` | 요청의 분산 트레이스 표시 |
| `c4_observe_status` | 관측성 파이프라인 상태 표시 |

---

## 접근 제어 — C6 Guard (조건부, `c6_guard` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `c4_guard_check` | 현재 정책에 의해 작업이 허용되는지 확인 |
| `c4_guard_audit` | guard 결정의 감사 로그 표시 |
| `c4_guard_policy_add` | RBAC 정책 규칙 추가 |
| `c4_guard_policy_list` | 모든 활성 정책 목록 |
| `c4_guard_deny` | 이유와 함께 작업 수동 거부 |

---

## 외부 커넥터 — C8 Gate (조건부, `c8_gate` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `c4_gate_webhook` | 아웃바운드 알림을 위한 webhook 등록 |
| `c4_gate_schedule` | 원격 작업 스케줄링 |
| `c4_gate_slack_send` | Slack에 메시지 전송 |
| `c4_gate_github_pr` | GitHub 풀 리퀘스트 생성 또는 업데이트 |
| `c4_gate_connector_list` | 설정된 외부 커넥터 목록 |
| `c4_gate_connector_test` | 커넥터 엔드포인트 테스트 |

---

## Python 사이드카 (LSP, Extended)

> Python/JavaScript/TypeScript 전용. Go/Rust → `c4_search_for_pattern` 사용.

| 도구 | 설명 |
|------|------|
| `c4_find_symbol` | 이름으로 심볼 정의 찾기 (Jedi/multilspy) |
| `c4_get_overview` | 모듈/클래스/함수 개요 가져오기 |
| `c4_replace_body` | 함수/클래스 본문 교체 |
| `c4_insert_before` | 심볼 앞에 코드 삽입 |
| `c4_insert_after` | 심볼 뒤에 코드 삽입 |
| `c4_rename` | 파일 전체에서 심볼 이름 변경 |
| `c4_find_refs` | 심볼의 모든 참조 찾기 |
| `c4_parse_document` | 구조화된 문서 파싱 (PDF, DOCX, HTML) |
| `c4_extract_text` | 문서에서 일반 텍스트 추출 |
| `c4_onboard` | 새 CQ 사용자를 위한 대화형 온보딩 |

---

## MCP Apps (위젯 시스템)

`format=widget`으로 도구를 호출하면 응답에 `_meta.ui.resourceUri`가 포함됩니다. MCP 클라이언트(Claude Code, Cursor, VS Code)가 `resources/read`를 통해 HTML을 가져와 샌드박스 iframe에 렌더링합니다.

| 위젯 URI | 도구 | 설명 |
|---------|------|------|
| `ui://cq/dashboard` | `c4_dashboard` | 프로젝트 상태 요약 |
| `ui://cq/job-progress` | `c4_job_status` | 작업 진행률 바 |
| `ui://cq/job-result` | `c4_job_summary` | 작업 결과 |
| `ui://cq/experiment-compare` | `c4_experiment_search` | 실험 비교 |
| `ui://cq/task-graph` | `c4_task_graph` | 태스크 의존성 그래프 |
| `ui://cq/nodes-map` | `c4_nodes_map` | 연결된 노드 맵 |
| `ui://cq/knowledge-feed` | `c4_knowledge_search` | 지식 피드 |
| `ui://cq/cost-tracker` | `c4_llm_costs` | LLM 비용 추적기 |
| `ui://cq/test-results` | `c4_run_validation` | 테스트 결과 |
| `ui://cq/git-diff` | `c4_diff_summary` | Git diff 뷰어 |
| `ui://cq/error-trace` | `c4_error_trace` | 오류 트레이스 뷰어 |

---

## 도구 수 요약

| 카테고리 | 수 | 비고 |
|---------|---|------|
| 프로젝트 및 상태 | 16 | Core |
| 파일 작업 | 8 | Core |
| Git | 3 | Core |
| 유효성 검사 | 2 | Core |
| 설정 및 상태 | 3 | Core |
| 스펙 및 설계 | 8 | Extended |
| Lighthouse 및 체크포인트 | 4 | Extended |
| 지식 | 13 | Extended, 동기화는 클라우드 |
| 세션 인텔리전스 | 5 | Extended |
| 메모리 | 1 | Extended |
| Drive | 6 | Extended, 클라우드 |
| 파일 인덱스 | 2 | Extended |
| Relay | 2 | Extended |
| LLM Gateway | 4 | Extended |
| Hub 작업 | 19 | 조건부 (Hub 티어) |
| Worker 대기 모드 | 3 | 조건부 (Hub 티어) |
| 실험 | 3 | Extended |
| Soul 및 Persona | 10 | Extended |
| CDP 및 WebMCP | 5 | Extended |
| 알림 및 EventBus | 9 | Extended |
| Skill 평가 | 3 | Extended |
| Research Loop | 4 | Extended |
| C7 Observe | 4 | 조건부 (빌드 태그) |
| C6 Guard | 5 | 조건부 (빌드 태그) |
| C8 Gate | 6 | 조건부 (빌드 태그) |
| Python 사이드카 (LSP) | 10 | Extended, Python/JS/TS |
| **합계** | **169** | 기본 118 + Hub 26 + 조건부 25 |

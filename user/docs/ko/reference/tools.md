# MCP 도구 레퍼런스

CQ는 AI 에이전트(Claude Code, Cursor, Codex CLI, Gemini, External Brain을 통한 ChatGPT)에게 **217개의 MCP 도구**를 제공합니다. 도구는 기능에 따라 카테고리로 분류됩니다.

**도구 등급:**
- **Core** — 항상 로드됨. 즉시 사용 가능
- **Extended** — 필요 시 로드됨. MCP 초기화 이후 사용 가능
- **Conditional** — 특정 빌드 태그 또는 Hub 연결 필요

---

## 프로젝트 & 상태 관리 (Core)

C4 프로젝트 생명주기, 상태 머신, 태스크 큐를 관리하는 도구입니다.

| 도구 | 설명 |
|------|------|
| `cq_status` | 현재 프로젝트 상태, 태스크 수, 활성 Worker 표시 |
| `cq_start` | C4 프로젝트 초기화 (`.c4/` 데이터베이스, 설정 파일 생성) |
| `cq_get_task` | 큐에서 다음 태스크 요청 (Worker 모드) |
| `cq_submit` | 완료된 태스크를 커밋 SHA 및 검증 결과와 함께 제출 |
| `cq_claim` | 직접 구현을 위한 태스크 점유 (Direct 모드) |
| `cq_report` | 태스크 완료 보고 (Direct 모드) |
| `cq_mark_blocked` | 태스크를 사유와 함께 blocked 상태로 표시 |
| `cq_request_changes` | 제출된 태스크에 변경 요청 |
| `cq_add_todo` | 큐에 새 태스크 추가 |
| `cq_task_list` | 태스크 목록 조회 (상태, 도메인, ID 필터 지원) |
| `cq_stale_tasks` | `in_progress` 상태에서 멈춰 있는 태스크 표시 |
| `cq_worker_heartbeat` | Worker 임대 유지를 위한 heartbeat 전송 |
| `cq_workers` | 활성 Worker 목록 및 현재 태스크 표시 |
| `cq_dashboard` | 프로젝트 상태 요약 위젯 (`format=widget`으로 UI 렌더링) |
| `cq_task_graph` | 전체 태스크 의존성 그래프 (`format=widget`으로 UI 렌더링) |
| `cq_task_events` | 태스크 상태 변경 이벤트 스트림 |
| `cq_reset_task` | 태스크를 pending 상태로 초기화 |
| `cq_phase_lock_acquire` | 동시 페이즈 전환 방지를 위한 페이즈 락 획득 |
| `cq_phase_lock_release` | 획득한 페이즈 락 해제 |
| `cq_clear` | C4 상태 초기화 (개발/테스트 용도) |

---

## 파일 작업 (Core)

코드베이스 읽기, 검색, 탐색 도구입니다.

| 도구 | 설명 |
|------|------|
| `cq_read_file` | 줄 번호 및 범위 지정으로 파일 읽기 |
| `cq_find_file` | 이름 패턴으로 파일 퍼지 검색 |
| `cq_file_find` | 확장 glob 지원 파일 검색 |
| `cq_search_for_pattern` | 파일 전체 정규식/리터럴 검색 (ripgrep 기반) |
| `cq_list_dir` | 디렉토리 내용 및 메타데이터 목록 |
| `cq_create_text_file` | 텍스트 파일 생성 또는 덮어쓰기 |
| `cq_replace_content` | 파일 내 정밀 find-and-replace |
| `cq_diff_summary` | staged + unstaged git diff 요약 (`format=widget`으로 UI 렌더링) |
| `cq_execute` | 셸 명령 실행 (permission hook으로 제어) |
| `cq_search_commits` | 메시지 또는 내용으로 git 커밋 이력 검색 |

---

## 코드 인텔리전스 / LSP (Extended)

> Python/JavaScript/TypeScript 전용. Go/Rust → `cq_search_for_pattern` 사용 권장.

| 도구 | 설명 |
|------|------|
| `cq_find_symbol` | 이름으로 심볼 정의 찾기 (Jedi/multilspy) |
| `cq_get_symbols_overview` | 모듈/클래스/함수 개요 조회 |
| `cq_replace_symbol_body` | 함수/클래스 본문 교체 |
| `cq_insert_before_symbol` | 심볼 앞에 코드 삽입 |
| `cq_insert_after_symbol` | 심볼 뒤에 코드 삽입 |
| `cq_rename_symbol` | 파일 전체에서 심볼 이름 변경 |
| `cq_find_referencing_symbols` | 심볼의 모든 참조 찾기 |
| `cq_parse_document` | 구조화 문서 파싱 (PDF, DOCX, HTML) |
| `cq_extract_text` | 문서에서 순수 텍스트 추출 |
| `cq_onboard` | 신규 CQ 사용자를 위한 대화형 온보딩 |

---

## 검증 & 테스트 (Core)

| 도구 | 설명 |
|------|------|
| `cq_run_validation` | lint + 테스트 실행 (go test, pytest, cargo test, pnpm test 자동 감지). 심각도: CRITICAL/HIGH/MEDIUM/LOW (`format=widget`으로 UI 렌더링) |
| `cq_run_checkpoint` | 수동 체크포인트 리뷰 트리거 |
| `cq_run_complete` | 결과와 함께 실행 완료 표시 |
| `cq_run_should_continue` | 현재 실행 계속 여부 확인 (루프 가드) |
| `cq_error_trace` | 디버깅을 위한 에러 트레이스 포맷 및 표시 (`format=widget`으로 UI 렌더링) |

---

## 설정 & 상태 확인 (Core)

| 도구 | 설명 |
|------|------|
| `cq_config_get` | `.c4/config.yaml`에서 설정 값 읽기 |
| `cq_config_set` | `.c4/config.yaml`에 설정 값 쓰기 |
| `cq_health` | CQ 전체 컴포넌트 상태 확인 (Doctor 기능과 동일) |
| `cq_whoami` | 현재 사용자 신원 및 연결된 프로젝트 표시 |
| `cq_gpu_status` | 연결된 Worker의 GPU 가용성 및 사용률 표시 |

---

## 스펙 & 설계 문서 (Extended)

구조화된 명세서 및 아키텍처 결정 기록 도구입니다.

| 도구 | 설명 |
|------|------|
| `cq_get_spec` | `docs/specs/`에서 스펙 문서 읽기 |
| `cq_save_spec` | 스펙 문서 작성 또는 업데이트 |
| `cq_list_specs` | 전체 스펙 목록 및 메타데이터 조회 |
| `cq_get_design` | 아키텍처 설계 문서 읽기 |
| `cq_save_design` | 설계 문서 작성 또는 업데이트 |
| `cq_list_designs` | 전체 설계 문서 목록 조회 |
| `cq_discovery_complete` | Discovery 페이즈 완료 표시, Design 페이즈로 전환 |
| `cq_design_complete` | Design 페이즈 완료 표시, Planning 페이즈로 전환 |

---

## Lighthouse & 체크포인트 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_lighthouse` | 구현이 Lighthouse 계약을 충족하는지 확인 |
| `cq_checkpoint` | 수동 체크포인트 리뷰 트리거 |
| `cq_artifact_save` | 콘텐츠 해시와 함께 빌드 아티팩트 저장 |
| `cq_artifact_get` | 이전에 저장된 아티팩트 불러오기 |
| `cq_artifact_list` | 저장된 아티팩트 전체 목록 및 메타데이터 조회 |
| `cq_snapshot` | 수동 컨텍스트 스냅샷 생성 |

---

## 지식 관리 (Extended)

> 클라우드 동기화는 `connected` 또는 `full` 티어 필요. FTS5 + pgvector (1536-dim OpenAI) + 3-way RRF.

| 도구 | 설명 |
|------|------|
| `cq_knowledge_record` | 지식 저장 (결정, 패턴, 인사이트, 에러, 발견). AI 자동 캡처: 도구 설명이 자율 저장을 트리거함. |
| `cq_knowledge_search` | 지식 검색: vector + FTS + ilike 3단계 폴백. 순위별 결과 반환. |
| `cq_knowledge_distill` | 문서 수 ≥ 5일 때 지식 자동 정제 (클러스터링 + 요약) |
| `cq_knowledge_ingest` | 파일 또는 URL을 지식 저장소에 일괄 수집 |
| `cq_knowledge_ingest_paper` | 구조화된 메타데이터와 함께 연구 논문 수집 |
| `cq_knowledge_publish` | 공유 프로젝트 네임스페이스에 지식 게시 |
| `cq_knowledge_pull` | 다른 프로젝트에서 게시된 지식 가져오기 |
| `cq_knowledge_get` | ID로 특정 지식 문서 조회 |
| `cq_knowledge_delete` | 지식 문서 삭제 |
| `cq_knowledge_stats` | 지식 저장소 통계 표시 (수, 마지막 동기화, 커버리지) |
| `cq_knowledge_reindex` | 지식 검색 인덱스 재빌드 |
| `cq_knowledge_discover` | 입력 개념으로부터 관련 지식 탐색 |
| `cq_pattern_suggest` | 지식 베이스에서 구현 패턴 제안 |
| `cq_recall` | 현재 태스크에 맥락적으로 관련된 기억 회상 |

---

## 세션 인텔리전스 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_analyze_history` | 패턴 및 인사이트를 위한 대화 이력 분석 |
| `cq_reflect` | 현재 세션 또는 태스크에 대한 구조화된 회고 생성 |
| `cq_pop_status` | 현재 POP (Point of Progress) 상태 표시 |
| `cq_pop_extract` | 현재 세션에서 핵심 결정 및 컨텍스트 추출 |
| `cq_pop_reflect` | 진행 상황 회고 및 세션 상태 업데이트 |
| `cq_profile_load` | 저장된 세션 프로파일 불러오기 |
| `cq_profile_save` | 현재 세션 컨텍스트를 이름 붙인 프로파일로 저장 |

---

## Drive (Extended)

> TUS 재개 가능 업로드가 포함된 클라우드 파일 저장소. `connected` 또는 `full` 티어 필요.

| 도구 | 설명 |
|------|------|
| `cq_drive_upload` | Drive에 파일 업로드 (TUS 재개 가능, 콘텐츠 주소 지정) |
| `cq_drive_download` | Drive에서 파일 다운로드 (Range 기반 재개) |
| `cq_drive_list` | Drive 파일 목록 및 메타데이터 조회 |
| `cq_drive_delete` | Drive에서 파일 삭제 |
| `cq_drive_info` | 파일 메타데이터 조회 (크기, 해시, URL) |
| `cq_drive_mkdir` | Drive에 디렉토리 생성 |
| `cq_drive_dataset_list` | Drive에 저장된 데이터셋 목록 조회 |
| `cq_drive_dataset_pull` | Drive에서 로컬 스토리지로 데이터셋 가져오기 |
| `cq_drive_dataset_upload` | 버전 관리와 함께 Drive에 데이터셋 업로드 |

---

## Relay (Extended)

| 도구 | 설명 |
|------|------|
| `cq_relay_call` | Relay를 통해 원격 Worker의 MCP 도구 호출 |
| `cq_nodes_map` | 연결된 모든 노드의 시각적 맵 (`format=widget`으로 UI 렌더링) |

---

## LLM Gateway (Extended)

> `llm_gateway` 빌드 태그 필요. 프롬프트 캐싱을 지원하는 멀티 프로바이더.

| 도구 | 설명 |
|------|------|
| `cq_llm_call` | 프로바이더 라우팅으로 LLM 호출 (Anthropic/OpenAI/Gemini/Ollama) |
| `cq_llm_providers` | 설정된 LLM 프로바이더 및 가용성 목록 |
| `cq_llm_costs` | LLM 비용 내역 및 캐시 절약 표시 (`format=widget`으로 UI 렌더링) |
| `cq_llm_usage_stats` | 세션/프로젝트별 상세 토큰 사용 통계 |

---

## Hub — 분산 작업 (Conditional, Hub 티어)

> Hub 연결 필요 (`serve.hub.enabled: true` + 클라우드 자격증명).

### 작업 운영

| 도구 | 설명 |
|------|------|
| `cq_job_submit` | 스펙 및 라우팅과 함께 Hub 큐에 작업 제출 |
| `cq_job_status` | 작업 상태 및 진행률 조회 (`format=widget`으로 UI 렌더링) |
| `cq_job_summary` | 작업 결과 및 출력 아티팩트 조회 (`format=widget`으로 UI 렌더링) |
| `cq_job_cancel` | 실행 중이거나 대기 중인 작업 취소 |
| `cq_job_list` | 최근 작업 목록 및 상태, 메트릭 조회 |
| `cq_hub_submit` | 전체 스펙 제어가 가능한 저수준 Hub 작업 제출 |
| `cq_hub_status` | Hub 연결 및 큐 상태 조회 |
| `cq_hub_list` | Hub 큐의 모든 작업 목록 조회 |
| `cq_hub_cancel` | ID로 Hub 작업 취소 |
| `cq_hub_retry` | 실패한 Hub 작업 재시도 |
| `cq_hub_wait` | Hub 작업 완료까지 대기 (블로킹) |
| `cq_hub_watch` | Hub 작업 진행 상황 실시간 감시 |
| `cq_hub_estimate` | 작업 스펙에 필요한 리소스 요구사항 추정 |
| `cq_hub_summary` | Hub 큐 요약 통계 조회 |
| `cq_hub_download` | 완료된 작업의 출력 아티팩트 다운로드 |
| `cq_hub_upload` | Hub 스토리지에 입력 아티팩트 업로드 |

### Worker 운영

| 도구 | 설명 |
|------|------|
| `cq_hub_workers` | 어피니티 점수 및 현재 부하와 함께 Worker 목록 조회 |
| `cq_hub_workers_unified` | Hub 모든 리전의 Worker 통합 뷰 |
| `cq_hub_stats` | Hub 인프라 상세 통계 |
| `cq_hub_lease_renew` | 타임아웃 방지를 위한 Hub 작업 임대 갱신 |

### Hub 메트릭

| 도구 | 설명 |
|------|------|
| `cq_hub_log_metrics` | 실행 중인 Hub 작업의 메트릭 로깅 (stdout 파서) |
| `cq_hub_metrics` | Hub 작업의 수집된 메트릭 조회 |

### DAG 파이프라인

| 도구 | 설명 |
|------|------|
| `cq_hub_dag_create` | 노드와 엣지로 DAG 파이프라인 생성 |
| `cq_hub_dag_add_node` | 기존 DAG에 노드 추가 |
| `cq_hub_dag_add_dep` | DAG 노드 간 의존성 엣지 추가 |
| `cq_hub_dag_execute` | DAG 파이프라인 실행 |
| `cq_hub_dag_from_yaml` | YAML 정의에서 DAG 생성 |
| `cq_hub_dag_status` | 노드별 진행 상황이 포함된 DAG 실행 상태 조회 |
| `cq_hub_dag_list` | 전체 DAG 목록 조회 |

### Cron 스케줄링

| 도구 | 설명 |
|------|------|
| `cq_cron_create` | 작업 스펙과 함께 cron 스케줄 등록 |
| `cq_cron_list` | 마지막 실행 시간과 함께 cron 스케줄 목록 조회 |
| `cq_cron_delete` | cron 스케줄 삭제 |

### Worker 생명주기

| 도구 | 설명 |
|------|------|
| `cq_worker_standby` | 대기 모드 진입 — Hub 작업 대기 및 실행 |
| `cq_worker_complete` | 대기 Worker에서 작업 완료 신호 전송 |
| `cq_worker_shutdown` | 태스크 인계와 함께 Worker 정상 종료 |
| `cq_ensure_supervisor` | Worker 슈퍼바이저 프로세스 실행 보장 |

---

## 실험 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_experiment_record` | 실험 결과 기록 (exp_id, 메트릭, 설정) |
| `cq_experiment_search` | 메트릭, 날짜, 태그로 실험 검색 (`format=widget`으로 UI 렌더링) |
| `cq_experiment_register` | 메타데이터와 함께 새 실험 정의 등록 |

---

## 페르소나 & Growth (Extended)

| 도구 | 설명 |
|------|------|
| `cq_soul_get` | 현재 soul/판단 기준 조회 |
| `cq_soul_set` | soul/판단 기준 업데이트 |
| `cq_soul_resolve` | soul 기준 간 충돌 해결 |
| `cq_persona_learn` | 학습된 행동 패턴 기록 |
| `cq_persona_learn_from_diff` | git diff에서 패턴 자동 추출 |
| `cq_persona_evolve` | 누적된 패턴을 기반으로 페르소나 진화 |
| `cq_persona_stats` | 현재 페르소나 프로파일 및 학습된 패턴 표시 |
| `cq_rule_add` | 라우팅 또는 행동 규칙 추가 |
| `cq_rule_list` | 활성 규칙 전체 목록 조회 |
| `cq_rule_remove` | ID로 규칙 삭제 |
| `cq_rule_toggle` | 삭제 없이 규칙 활성화/비활성화 |
| `cq_intelligence_stats` | 사용자 전체의 집단 지능 통계 표시 |

---

## CDP (Chrome DevTools Protocol, Extended)

| 도구 | 설명 |
|------|------|
| `cq_cdp_run` | CDP를 통해 브라우저 탭에서 JavaScript 실행 |
| `cq_cdp_action` | CDP를 통해 브라우저 액션 수행 (클릭, 입력, 탐색) |
| `cq_cdp_list` | 사용 가능한 CDP 브라우저 대상 목록 조회 |
| `cq_webmcp_discover` | 페이지에서 Web MCP 엔드포인트 자동 발견 |
| `cq_webmcp_call` | 발견된 Web MCP 도구 호출 |
| `cq_webmcp_context` | Web MCP 활성화 페이지에서 컨텍스트 가져오기 |
| `cq_web_fetch` | 콘텐츠 협상 + HTML→Markdown 변환으로 URL 가져오기 |

---

## 알림 & EventBus (Extended)

| 도구 | 설명 |
|------|------|
| `cq_notify` | 설정된 채널(Telegram/Dooray)로 알림 전송 |
| `cq_notification_channels` | 설정된 알림 채널 목록 조회 |
| `cq_notification_get` | 채널의 알림 설정 조회 |
| `cq_notification_set` | 채널의 알림 설정 구성 |
| `cq_event_publish` | EventBus에 이벤트 게시 |
| `cq_event_list` | 필터를 적용한 최근 EventBus 이벤트 목록 조회 |
| `cq_record_gate` | 게이트 결정 이벤트 기록 |

---

## 메일 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_mail_send` | 이름 붙인 세션에 세션 간 메일 전송 |
| `cq_mail_ls` | 메일 수신함 메시지 목록 조회 |
| `cq_mail_read` | 특정 메일 메시지 읽기 |
| `cq_mail_rm` | 수신함에서 메일 메시지 삭제 |

---

## 워크스페이스 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_workspace_create` | 격리된 상태로 새로운 이름 붙인 워크스페이스 생성 |
| `cq_workspace_load` | 저장된 워크스페이스 컨텍스트 불러오기 |
| `cq_workspace_save` | 현재 워크스페이스 상태 저장 |
| `cq_worktree_status` | 모든 활성 브랜치의 git worktree 상태 표시 |
| `cq_worktree_cleanup` | 병합되었거나 오래된 worktree 정리 |

---

## 시크릿 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_secret_set` | 암호화된 시크릿 저장소에 시크릿 저장 |
| `cq_secret_get` | 키로 시크릿 조회 |
| `cq_secret_list` | 모든 시크릿 키 목록 조회 (값은 표시하지 않음) |
| `cq_secret_delete` | 키로 시크릿 삭제 |

---

## 집단 지능 & 동기화 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_collective_stats` | 집단 지능 통계 표시 |
| `cq_collective_sync` | 로컬 지식을 집단 지성에 동기화 |

---

## 스킬 평가 (Extended)

| 도구 | 설명 |
|------|------|
| `cq_skill_eval_run` | 스킬의 테스트 케이스에 대해 k-trial haiku 분류 실행 |
| `cq_skill_eval_generate` | 스킬의 긍정/부정 테스트 케이스 생성 |
| `cq_skill_eval_status` | 평가된 전체 스킬의 트리거 정확도 표시 (임계값: 0.90) |
| `cq_skill_optimize` | 평가 결과를 기반으로 스킬 최적화 |

---

## Research Loop (Extended)

| 도구 | 설명 |
|------|------|
| `cq_research_loop_start` | 자율 Research Loop 시작 (LoopOrchestrator) |
| `cq_research_status` | 현재 루프 상태, 반복 횟수, 최고 메트릭 표시 |
| `cq_research_loop_stop` | Research Loop 정상 종료 |
| `cq_research_next` | Research Loop를 다음 반복으로 진행 |
| `cq_research_record` | 연구 발견 또는 중간 결과 기록 |
| `cq_research_start` | 단일 연구 태스크 시작 |
| `cq_research_approve` | 연구 결과 승인 후 루프 진행 |
| `cq_research_suggest` | 다음 연구 방향 제안 조회 |

---

## 관측성 — C7 Observe (Conditional, `c7_observe` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `cq_observe_metrics` | 수집된 메트릭 조회 (도구 호출, 지연 시간, 에러율) |
| `cq_observe_logs` | 필터를 적용한 구조화 로그 조회 |
| `cq_observe_traces` | 요청의 분산 트레이스 표시 |
| `cq_observe_trace_stats` | 트레이스 통계 및 지연 시간 백분위수 표시 |
| `cq_observe_health` | 관측성 파이프라인 상태 표시 |
| `cq_observe_config` | 관측성 수집 설정 구성 |
| `cq_observe_policy` | 데이터 보존 및 샘플링 정책 설정 |

---

## 접근 제어 — C6 Guard (Conditional, `c6_guard` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `cq_guard_check` | 현재 정책에 따라 작업 허용 여부 확인 |
| `cq_guard_audit` | 가드 결정 감사 로그 표시 |
| `cq_guard_policy_list` | 활성 정책 전체 목록 조회 |
| `cq_guard_policy_set` | RBAC 정책 규칙 추가 또는 업데이트 |
| `cq_guard_role_assign` | 사용자 또는 에이전트에 역할 할당 |

---

## 외부 커넥터 — C8 Gate (Conditional, `c8_gate` 빌드 태그)

| 도구 | 설명 |
|------|------|
| `cq_gate_webhook_register` | 아웃바운드 알림을 위한 webhook 등록 |
| `cq_gate_webhook_list` | 등록된 webhook 목록 조회 |
| `cq_gate_webhook_test` | webhook 엔드포인트 테스트 |
| `cq_gate_schedule_add` | cron 표현식으로 원격 액션 스케줄링 |
| `cq_gate_schedule_list` | 등록된 모든 게이트 액션 스케줄 목록 조회 |
| `cq_gate_connector_status` | 설정된 모든 외부 커넥터 상태 조회 |

---

## 레거시 / 하위 호환 (c4_ 접두사)

> 기존 에이전트 워크플로우와의 하위 호환성을 위해 `c4_` 접두사를 유지합니다.

| 도구 | 설명 |
|------|------|
| `c4_get_task` | `cq_get_task`의 별칭 (Worker 모드 태스크 수령) |
| `c4_health` | `cq_health`의 별칭 |
| `c4_knowledge_ingest_paper` | `cq_knowledge_ingest_paper`의 별칭 |
| `c4_research_suggest` | `cq_research_suggest`의 별칭 |
| `c4_status` | `cq_status`의 별칭 |
| `c4_test` | C4 자가 테스트 실행 |
| `c4_test_tool` | 특정 MCP 도구 호출 테스트 |

---

## MCP 앱 (위젯 시스템)

`format=widget`으로 도구를 호출하면 응답에 `_meta.ui.resourceUri`가 포함됩니다. MCP 클라이언트(Claude Code, Cursor, VS Code)는 `resources/read`를 통해 HTML을 가져와 샌드박스 iframe에 렌더링합니다.

| 위젯 URI | 도구 | 설명 |
|---------|------|------|
| `ui://cq/dashboard` | `cq_dashboard` | 프로젝트 상태 요약 |
| `ui://cq/job-progress` | `cq_job_status` | 작업 진행률 바 |
| `ui://cq/job-result` | `cq_job_summary` | 작업 결과 |
| `ui://cq/experiment-compare` | `cq_experiment_search` | 실험 비교 |
| `ui://cq/task-graph` | `cq_task_graph` | 태스크 의존성 그래프 |
| `ui://cq/nodes-map` | `cq_nodes_map` | 연결된 노드 맵 |
| `ui://cq/cost-tracker` | `cq_llm_costs` | LLM 비용 추적기 |
| `ui://cq/test-results` | `cq_run_validation` | 테스트 결과 |
| `ui://cq/git-diff` | `cq_diff_summary` | Git diff 뷰어 |
| `ui://cq/error-trace` | `cq_error_trace` | 에러 트레이스 뷰어 |

---

## 도구 수 요약

| 카테고리 | 수 | 비고 |
|---------|-----|------|
| 프로젝트 & 상태 | 20 | Core |
| 파일 작업 | 10 | Core |
| 코드 인텔리전스 / LSP | 10 | Extended, Python/JS/TS |
| 검증 & 테스트 | 5 | Core |
| 설정 & 상태 확인 | 5 | Core |
| 스펙 & 설계 문서 | 8 | Extended |
| Lighthouse & 체크포인트 | 6 | Extended |
| 지식 관리 | 14 | Extended, 동기화는 클라우드 |
| 세션 인텔리전스 | 7 | Extended |
| Drive | 9 | Extended, 클라우드 |
| Relay | 2 | Extended |
| LLM Gateway | 4 | Extended |
| Hub 작업 | 16 | Conditional (Hub 티어) |
| Hub Worker 운영 | 4 | Conditional (Hub 티어) |
| Hub 메트릭 | 2 | Conditional (Hub 티어) |
| Hub DAG 파이프라인 | 7 | Conditional (Hub 티어) |
| Cron 스케줄링 | 3 | Conditional (Hub 티어) |
| Worker 생명주기 | 4 | Conditional (Hub 티어) |
| 실험 | 3 | Extended |
| 페르소나 & Growth | 11 | Extended |
| CDP & WebMCP | 7 | Extended |
| 알림 & EventBus | 7 | Extended |
| 메일 | 4 | Extended |
| 워크스페이스 | 5 | Extended |
| 시크릿 | 4 | Extended |
| 집단 지능 & 동기화 | 2 | Extended |
| 스킬 평가 | 4 | Extended |
| Research Loop | 8 | Extended |
| C7 Observe | 7 | Conditional (빌드 태그) |
| C6 Guard | 5 | Conditional (빌드 태그) |
| C8 Gate | 6 | Conditional (빌드 태그) |
| 레거시 (c4_ 접두사) | 7 | 하위 호환 별칭 |
| **합계** | **217** | — |

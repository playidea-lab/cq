# CQ Feature Decision Matrix

> `/c4-help <키워드>` 검색 시 이 매트릭스를 참조하여 기능 안내.

## Remote Workspace (v1.38.0+)

| 수단 | 용도 | 도구/CLI |
|------|------|---------|
| **relay** (즉시 조작) | 원격 파일 읽기/쓰기, 상태 확인, 짧은 명령 | `cq_workers()`, `cq_relay_call(worker_id, tool, args)` |
| **Hub** (잡 큐) | 학습, 빌드, 장시간 실험 (게시판 패턴) | `cq hub submit "command" --tag gpu` |
| **Git** (코드) | 코드 버전 관리 + relay로 원격 pull | `git push` → `cq_relay_call("c4_execute", "git pull")` |
| **Drive** (데이터) | 데이터셋/체크포인트 버전 관리 (DVC 패턴) | `c4_drive_dataset_upload/pull` |
| **Transfer** (레거시) | NAT 뒤 P2P 직접 전송 (Drive 대체 권장) | `cq transfer <file> --to <worker-id>` |

## 데이터 전송

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| 파일을 클라우드에 저장/공유 | Drive (C0) | `c4_drive_upload`, `c4_drive_download`, `c4_drive_list` |
| 데이터셋 버전 관리 (CAS) | Drive Dataset | `c4_drive_dataset_upload/pull` (content hash) |
| 소량 텍스트 파일 원격 전송 (<1MB) | relay_call | `cq_relay_call(worker, "c4_create_text_file", {path, content})` |
| NAT 뒤 워커로 대용량 직접 전송 | Transfer (레거시) | `cq transfer <file> --to <worker-id>` |
| 잡에 코드 스냅샷 첨부 | Hub Snapshot | `cq hub submit` (Drive CAS 자동) |

## 실험/연구

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| GPU 서버에서 모델 학습 | Hub Auto Worker | `cq hub submit --tag gpu` (cq serve가 자동 실행) |
| 자율 연구 루프 (가설→실험→검증→반복) | Research Loop | `c4_research_loop_start` |
| 연구 루프 방향 전환 | Research Steer | `c4_research_intervene` (steering/injection/abort) |
| 실험 결과 기록 | Knowledge | `c4_experiment_record` |
| 과거 실험 패턴 조회 | Knowledge Search | `c4_knowledge_search` |
| 가설 계보 추적 | Lineage | LineageBuilder (orchestrator 내장) |

## 잡 관리 (C5 Hub)

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| 잡 제출 | Hub Submit | `c4_hub_submit` / `cq hub submit` |
| 잡 상태 확인 | Hub Status | `c4_hub_status` / `cq hub job status <id>` |
| 잡 로그 조회 | Hub Logs | `cq hub job log <id> [--follow]` |
| DAG 파이프라인 실행 | Hub DAG | `c4_hub_dag_create` → `c4_hub_dag_execute` |
| 워커 목록/상태 | Hub Workers | `c4_hub_workers` / `cq hub workers` |

## 알림/이벤트

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| Dooray/Discord/Slack 알림 | Notify | `c4_notify` |
| 이벤트 기반 자동화 규칙 | EventBus Rules | `c4_rule_add` / `c4_rule_list` |
| 이벤트 발행 | EventBus Publish | `c4_event_publish` |

## 지식 관리 (C9)

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| 인사이트/패턴 기록 | Knowledge Record | `c4_knowledge_record` |
| 지식 검색 (FTS5 + vector) | Knowledge Search | `c4_knowledge_search` |
| 외부 문서 수집 (URL/PDF) | Knowledge Ingest | `c4_knowledge_ingest` |
| Hub 간 지식 동기화 | Knowledge Publish/Pull | `c4_knowledge_publish`, `c4_knowledge_pull` |

## 페르소나/학습

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| 사용자 코딩 스타일 학습 | Persona Learn | `c4_persona_learn` (C2 diff 분석) |
| 소울 진화 (패턴 결정화) | Soul Evolution | `scripts/soul-evolve.sh` |
| 개인 온톨로지 추출 | POP Pipeline | `c4_pop_extract` → `c4_pop_reflect` |

## LLM 호출

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| 외부 LLM 호출 (OpenAI/Anthropic) | LLM Gateway | `c4_llm_call` |
| 비용 조회 | LLM Costs | `c4_llm_costs` |
| 프로바이더 목록 | LLM Providers | `c4_llm_providers` |

## 문서 처리 (C2)

| 상황 | 기능 | 도구/CLI |
|------|------|---------|
| PDF/문서 파싱 | Document Parse | `c4_parse_document` |
| 텍스트 추출 | Text Extract | `c4_extract_text` |
| 워크스페이스 관리 | Workspace | `c4_workspace_create/load/save` |

---

> 도구 상세 사용법: `c4_lighthouse get <tool_name>`

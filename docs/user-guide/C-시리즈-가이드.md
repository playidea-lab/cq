# C 시리즈 생태계 사용 가이드

**Cstem**(System의 발음 변형): C1·C2·C3·C4·C5·C9가 유기적으로 연결된 생태계.  
AI 에이전트와 개발자를 위한 C 시리즈 컴포넌트 통합 사용 설명서입니다.

## 전체 구조

```
C0 Drive ─────── 클라우드 파일 스토리지
C1 Messenger ─── 실시간 메시징 + 대시보드
C2 Docs ──────── 문서 라이프사이클 (파싱/워크스페이스/프로필)
C3 EventBus ──── 이벤트 라우팅 + 웹훅 + DLQ
C4 Engine ────── MCP 오케스트레이션 (이 프로젝트의 핵심)
C5 Hub ───────── 분산 작업 큐 서버
C9 Knowledge ─── 지식 관리 (검색/임베딩/증류)
```

모든 컴포넌트는 **C4 MCP 도구**로 호출합니다. Supabase(클라우드)와 SQLite(로컬) 이중 구조.

---

## C0 Drive — 클라우드 파일 스토리지

Supabase Storage 기반 파일 관리. SHA256 해시로 중복 제거.

### MCP 도구 (6개)

| 도구 | 설명 | 필수 파라미터 |
|------|------|---------------|
| `c4_drive_upload` | 파일 업로드 | `local_path`, `drive_path` |
| `c4_drive_download` | 파일 다운로드 | `drive_path`, `local_path` |
| `c4_drive_list` | 폴더 내용 조회 | `path` (기본: "/") |
| `c4_drive_delete` | 파일/폴더 삭제 | `path` |
| `c4_drive_info` | 파일 메타데이터 조회 | `path` |
| `c4_drive_mkdir` | 폴더 생성 | `path` |

### 사용 예시

```
# 모델 체크포인트 업로드
c4_drive_upload(
  local_path="/tmp/model_v1.pkl",
  drive_path="/ml-models/model_v1.pkl",
  metadata={"version": "1.0", "framework": "pytorch"}
)

# 프로젝트 파일 목록
c4_drive_list(path="/ml-models")

# 파일 다운로드
c4_drive_download(
  drive_path="/ml-models/model_v1.pkl",
  local_path="/tmp/download/"
)
```

### 설정 요건

```bash
# 환경변수 (Supabase 연결)
SUPABASE_URL=https://xxx.supabase.co
SUPABASE_ANON_KEY=eyJ...
```

미설정 시 도구는 등록되지만 인증 에러 반환.

---

## C1 Messenger — 실시간 메시징 대시보드

Supabase Realtime 기반 팀 메시징. 에이전트/사용자/시스템을 동등한 멤버로 관리.

### MCP 도구 (5개)

| 도구 | 설명 | 필수 파라미터 |
|------|------|---------------|
| `c1_search` | 메시지 전문 검색 | `query` |
| `c1_check_mentions` | 미읽은 멘션 확인 | `agent_name` |
| `c1_get_briefing` | 채널 요약 + 최근 메시지 | (없음) |
| `c1_send_message` | 채널에 메시지 전송 | `channel_name`, `content` |
| `c1_update_presence` | 에이전트 상태 업데이트 | `agent_name`, `status` |

### 채널 타입

| 타입 | 용도 | 예시 |
|------|------|------|
| `topic` | 일반 토론 | project-alpha |
| `auto` | 시스템 자동 생성 | general, tasks, events, knowledge |
| `worker` | C4/C5 워커 채널 | worker-1, worker-2 |
| `dm` | 1:1 대화 | — |

### 멤버 상태 (Presence)

| 상태 | 의미 | 표시 |
|------|------|------|
| `online` | 활성 | 녹색 |
| `working` | 작업 중 | 녹색 |
| `idle` | 대기 | 노란색 |
| `offline` | 비활성 | 회색 |

### 사용 예시

```
# 작업 상태 업데이트
c1_update_presence(
  agent_name="code-reviewer",
  status="working",
  status_text="Reviewing T-003-0"
)

# 채널에 메시지 전송
c1_send_message(
  channel_name="tasks",
  content="T-001-0 구현 완료. 테스트 통과.",
  agent_name="feature-builder"
)

# 미읽은 멘션 확인
c1_check_mentions(agent_name="feature-builder")

# 프로젝트 브리핑 (채널 요약 + 최근 메시지)
c1_get_briefing()
```

### 자동 동작 (ContextKeeper)

C4 Engine이 자동으로 수행:
- **시스템 채널 생성**: general, tasks, events, knowledge
- **이벤트 자동 포스팅**: 태스크 완료, 지식 기록, 체크포인트 등
- **채널 요약 갱신**: LLM으로 새 메시지 5개 이상일 때 요약 업데이트

### 설정 요건

```bash
SUPABASE_URL=https://xxx.supabase.co
SUPABASE_ANON_KEY=eyJ...
```

---

## C2 Docs — 문서 라이프사이클

학술 논문, 제안서, 보고서의 작성 워크플로우 관리. 사용자 글쓰기 스타일 학습.

### MCP 도구 (8개)

| 도구 | 설명 | 필수 파라미터 |
|------|------|---------------|
| `c4_parse_document` | 문서 파싱 (HWP/DOCX/PDF/XLSX/PPTX) | `file_path` |
| `c4_extract_text` | 텍스트 추출 | `file_path` |
| `c4_workspace_create` | 워크스페이스 생성 | `name` |
| `c4_workspace_load` | 워크스페이스 로드 | `project_dir` |
| `c4_workspace_save` | 워크스페이스 저장 | `project_dir`, `state` |
| `c4_persona_learn` | 글쓰기 스타일 학습 (초안↔최종 비교) | `draft_path`, `final_path` |
| `c4_profile_load` | 사용자 프로필 로드 | (기본: `.c2/profile.yaml`) |
| `c4_profile_save` | 사용자 프로필 저장 | `data` |

### 워크스페이스 타입

| 타입 | 기본 섹션 |
|------|----------|
| `academic_paper` | abstract, introduction, related_work, method, experiments, discussion, conclusion |
| `proposal` | executive_summary, background, objectives, approach, timeline, budget |
| `report` | (사용자 정의) |

### 사용 예시

```
# 논문 워크스페이스 생성
c4_workspace_create(
  name="DINOv3 Paper",
  project_type="academic_paper",
  goal="ICML 2026 제출"
)

# HWP 문서 파싱
c4_parse_document(file_path="/docs/proposal.hwp")
→ {blocks: [{type: "heading", ...}, {type: "paragraph", ...}], block_count: 42}

# 글쓰기 스타일 학습
c4_persona_learn(
  draft_path="/tmp/draft.md",
  final_path="/tmp/final.md",
  auto_apply=true
)
→ {summary: "톤 패턴 2건, 구조 패턴 1건 발견"}
```

### 설정

- 프로필 위치: `.c2/profile.yaml` (자동 생성)
- 워크스페이스: `{project_dir}/c2_workspace.md`
- 문서 파싱은 **Python sidecar** 필요 (LibreOffice for HWP)

---

## C3 EventBus — 이벤트 라우팅 시스템

gRPC UDS 데몬 + WebSocket 브릿지 + Dead Letter Queue. 규칙 기반 이벤트 디스패치.

### MCP 도구 (6개)

| 도구 | 설명 | 필수 파라미터 |
|------|------|---------------|
| `c4_event_publish` | 이벤트 발행 | `type`, `source` |
| `c4_event_list` | 이벤트 목록 조회 | (선택: `type`, `limit`) |
| `c4_rule_add` | 라우팅 규칙 추가 | `name`, `event_pattern`, `action_type`, `action_config` |
| `c4_rule_list` | 규칙 목록 조회 | (없음) |
| `c4_rule_remove` | 규칙 삭제 | `name` |
| `c4_rule_toggle` | 규칙 활성/비활성 | `name`, `enabled` |

### 표준 이벤트 타입 (16종)

```
task.created, task.updated, task.completed, task.blocked
checkpoint.approved, checkpoint.rejected
review.changes_requested
validation.passed, validation.failed
knowledge.recorded, knowledge.searched
drive.uploaded
(커스텀 타입 지원)
```

### 액션 타입

| 타입 | 설명 | config 예시 |
|------|------|-------------|
| `log` | stderr 로그 출력 | `{}` |
| `webhook` | 외부 URL POST (HMAC-SHA256) | `{"url": "...", "secret": "..."}` |
| `c1_post` | C1 Messenger 채널 포스팅 | `{"channel": "#tasks", "template": "..."}` |

### 필터 문법 (v2)

```json
{
  "status": {"$eq": "done"},
  "priority": {"$gt": 5},
  "tag": {"$in": ["urgent", "important"]},
  "title": {"$regex": "^Task"},
  "error": {"$exists": false}
}
```

연산자: `$eq`, `$ne`, `$gt`, `$lt`, `$in`, `$regex`, `$exists`

### 사용 예시

```
# 이벤트 발행
c4_event_publish(
  type="task.completed",
  source="c4.core",
  data={"task_id": "T-001", "status": "done"}
)

# 웹훅 규칙 추가
c4_rule_add(
  name="notify-on-complete",
  event_pattern="task.completed",
  filter_json='{"status": {"$eq": "done"}}',
  action_type="webhook",
  action_config='{"url": "https://api.example.com/notify", "secret": "key"}'
)

# C1 채널 알림 규칙
c4_rule_add(
  name="task-to-messenger",
  event_pattern="task.*",
  action_type="c1_post",
  action_config='{"channel": "tasks", "template": "{{event_type}}: {{task_id}}"}'
)
```

### 설정

- 자동 내장 모드: C4 Engine 시작 시 자동 실행 (별도 데몬 불필요)
- 데이터: `.c4/eventbus/events.db` (프로젝트 루트 기준)
- WebSocket: `127.0.0.1:{port}/ws/events` (설정 시)
- 보존 기간: 기본 7일, DLQ 14일

---

## C4 Engine — MCP 오케스트레이션 엔진

프로젝트의 핵심. 상태 머신 기반 태스크 관리 + 137개 MCP 도구.

### 핵심 MCP 도구 (자주 쓰는 것)

| 카테고리 | 도구 | 설명 |
|----------|------|------|
| 상태 | `c4_status` | 프로젝트 상태 + 큐 + 워커 |
| 태스크 | `c4_add_todo` | 태스크 추가 |
| 태스크 | `c4_get_task` | 태스크 할당 (Worker 모드) |
| 태스크 | `c4_claim` / `c4_report` | 태스크 시작/완료 (Direct 모드) |
| 태스크 | `c4_submit` | 완료 제출 |
| 태스크 | `c4_task_list` | 태스크 목록 조회 |
| 파일 | `c4_read_file`, `c4_find_file`, `c4_search_for_pattern` | 파일 조작 |
| LSP | `c4_find_symbol`, `c4_get_symbols_overview` | 코드 탐색 |
| 검증 | `c4_run_validation` | lint + test 실행 |

### 워크플로우

```
INIT → PLAN → EXECUTE ⇄ CHECKPOINT → COMPLETE
                ↕
              HALTED
```

### 실행 모드

| 모드 | 설명 | 흐름 |
|------|------|------|
| **Worker** | 독립적 태스크, 병렬 가능 | `c4_get_task` → 작업 → `c4_submit` |
| **Direct** | 의존성 높은 작업 | `c4_claim` → 작업 → `c4_report` |

### 빠른 시작 (스킬)

```
/c4-plan "기능 설명"    # 계획 수립 + 태스크 생성
/c4-run                 # Worker 스폰 → 자동 실행
/c4-status              # 진행 상황 확인
/c4-finish              # 빌드 + 테스트 + 설치 + 커밋
```

---

## C5 Hub — 분산 작업 큐 서버

Go 기반 Job Queue. Worker Pull 모델, Lease 기반 잡 관리.

### MCP 도구 (Hub 활성 시 29개)

| 카테고리 | 도구 | 설명 |
|----------|------|------|
| **Job** | `c4_hub_submit` | 잡 제출 |
| **Job** | `c4_hub_status` | 잡 상태 조회 |
| **Job** | `c4_hub_list` | 잡 목록 |
| **Job** | `c4_hub_cancel` | 잡 취소 |
| **Job** | `c4_hub_retry` | 실패한 잡 재시도 |
| **Worker** | `c4_worker_standby` | 워커 대기 (블로킹) |
| **Worker** | `c4_worker_complete` | 작업 완료 보고 |
| **Worker** | `c4_worker_shutdown` | 워커 정상 종료 |
| **DAG** | `c4_hub_dag_create` | DAG 워크플로우 생성 |
| **DAG** | `c4_hub_dag_execute` | DAG 실행 |
| **Edge** | `c4_hub_edge_register` | 엣지 노드 등록 |
| **Deploy** | `c4_hub_deploy` | 배포 트리거 |

### 서버 실행

```bash
cd c5 && go build -o bin/c5 ./cmd/c5/
./bin/c5 serve --port 8585 --db ./c5.db
```

### C4 연동 설정

```yaml
# .c4/config.yaml
hub:
  enabled: true
  url: "http://localhost:8585"
  api_prefix: "/v1"          # C5 Hub은 /v1 필수
  team_id: "my-team"
```

### 워커 흐름

```
1. c4_hub_submit(command="task_id=T-001-0")  # 잡 제출
2. c4_worker_standby(worker_id="w1")          # 대기 → 잡 수신
3. (잡 실행)
4. c4_worker_complete(job_id, worker_id, status="SUCCEEDED")
5. 다시 standby (루프)
```

상세: [워커-가이드.md](./워커-가이드.md)

---

## C9 Knowledge — 지식 관리 시스템

FTS5 + 벡터 검색 + 사용 추적 + 자동 패턴 증류.

### MCP 도구 (14개)

| 도구 | 설명 | 핵심 파라미터 |
|------|------|---------------|
| `c4_knowledge_record` | 지식 기록 | `doc_type`, `title`, `content`, `tags` |
| `c4_knowledge_get` | 문서 조회 | `doc_id`, `cite` (인기도 부스트) |
| `c4_knowledge_search` | 하이브리드 검색 (FTS+벡터+인기도) | `query`, `limit` |
| `c4_knowledge_ingest` | 파일/URL 수집 (RAG) | `file_path` 또는 `url` |
| `c4_knowledge_distill` | 자동 패턴 추출 (LLM) | `threshold`, `dry_run` |
| `c4_knowledge_stats` | 통계 + 관측성 | (없음) |
| `c4_knowledge_delete` | 문서 삭제 | `doc_id` |
| `c4_knowledge_publish` | 커뮤니티 공유 | `doc_id` |
| `c4_knowledge_discover` | 크로스-프로젝트 검색 | `query` |
| `c4_knowledge_pull` | 클라우드 동기화 | `doc_type` |
| `c4_knowledge_reindex` | 인덱스 재빌드 | (없음) |
| `c4_experiment_record` | 실험 결과 기록 | `title`, `content` |
| `c4_experiment_search` | 실험 검색 | `query` |
| `c4_pattern_suggest` | 패턴 추천 | `context` |

### 문서 타입

| 타입 | 용도 |
|------|------|
| `experiment` | 실험 결과, 가설 검증 |
| `pattern` | 재사용 가능한 패턴/인사이트 |
| `insight` | 발견, 관찰 |
| `hypothesis` | 제안 (confidence/evidence 포함) |

### 검색 알고리즘 (3-Way RRF)

```
최종 점수 = Σ(1 / (60 + rank_i + 1))
  - FTS5 텍스트 매칭 순위
  - 벡터 코사인 유사도 순위
  - 시간 가중 인기도 순위 (30일 반감기)
```

### 사용 예시

```
# 실험 결과 기록
c4_experiment_record(
  title="Mixed precision으로 FPS 30% 향상",
  content="A100에서 FP16 적용 시...",
  tags=["performance", "gpu"]
)

# 관련 지식 검색
c4_knowledge_search(query="모델 최적화 기법", limit=5)

# URL에서 지식 수집 (RAG)
c4_knowledge_ingest(
  url="https://arxiv.org/abs/2406.05000",
  title="DINOv3 Paper",
  tags=["vision", "detection"]
)

# 자동 패턴 증류 (미리보기)
c4_knowledge_distill(threshold=0.7, dry_run=true)

# 통계 확인
c4_knowledge_stats()
```

### 저장 구조

```
.c4/knowledge/
├── docs/          # Markdown 문서 (SSOT)
│   ├── exp-abc.md
│   └── pat-def.md
└── index.db       # SQLite (FTS5 + 벡터 + 사용 추적)
```

---

## 컴포넌트 간 연동

```
                ┌─── C0 Drive (파일 업/다운)
                │
C4 Engine ──────┼─── C1 Messenger (메시지/프레즌스)
  (MCP 137)     │       ↑ AutoPost
                ├─── C2 Docs (문서 파싱/워크스페이스)
                │
                ├─── C3 EventBus (이벤트 → 웹훅/C1/로그)
                │       ↑ task.completed, knowledge.recorded 등
                ├─── C5 Hub (잡 큐 → 워커)
                │
                └─── C9 Knowledge (검색/기록/증류)
```

### 연동 예시

1. **태스크 완료 → 자동 알림**
   - C4가 태스크 완료 → C3 EventBus에 `task.completed` 발행
   - C3 규칙이 C1 Messenger "tasks" 채널에 포스팅
   - 웹훅으로 외부 서비스 알림

2. **워커 실행 → 지식 기록**
   - C5 Hub에서 잡 수신 → C4 Worker가 구현
   - 구현 중 발견한 패턴 → C9 Knowledge에 기록
   - C1 worker 채널에 진행 상황 포스팅

3. **문서 수집 → 지식화**
   - C2로 PDF 파싱 → 텍스트 추출
   - C9 Knowledge에 ingest → 청크 임베딩
   - C0 Drive에 원본 파일 보관

---

## 설정 요약

### 필수 환경변수

| 변수 | 용도 | 영향 범위 |
|------|------|----------|
| `SUPABASE_URL` | 클라우드 연결 | C0, C1, Cloud |
| `SUPABASE_ANON_KEY` | 인증 | C0, C1, Cloud |

### .c4/config.yaml

```yaml
# Hub (C5)
hub:
  enabled: true
  url: "http://localhost:8585"
  api_prefix: "/v1"
  team_id: "my-team"

# LLM Gateway (C9 임베딩, C1 요약, c4_llm_call)
llm_gateway:
  enabled: true
  default: anthropic
  cache_by_default: true  # Anthropic Prompt Caching 자동 적용 (기본값 true)
  providers:
    anthropic:
      enabled: true
      api_key_env: ANTHROPIC_API_KEY  # 환경변수 이름 (실제 키는 .env에)
    openai:
      enabled: true
      api_key_env: OPENAI_API_KEY

# Cloud (C0, C1)
cloud:
  enabled: true
  project_id: "my-project"
```

### 의존성 매트릭스

| 컴포넌트 | Supabase | C5 Hub | LLM Gateway | Python Sidecar |
|----------|----------|--------|-------------|----------------|
| C0 Drive | **필수** | — | — | — |
| C1 Messenger | **필수** | — | 선택 (요약) | — |
| C2 Docs | — | — | — | 파싱 시 필요 |
| C3 EventBus | — | — | — | — |
| C4 Engine | — | — | — | LSP 시 필요 |
| C5 Hub | — | **자체** | — | — |
| C9 Knowledge | — | — | 선택 (임베딩) | — |

**미설정 시 동작**: 해당 기능만 비활성. 나머지는 정상 동작.

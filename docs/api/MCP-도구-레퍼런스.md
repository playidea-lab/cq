# MCP 도구 레퍼런스

이 문서는 C4 MCP 서버가 제공하는 도구(tools)의 카탈로그입니다.

## 개요

C4 MCP 서버는 **57개 도구**를 제공합니다:

### Core State (4개) — `Go: c4-core/internal/mcp/handlers/state.go` + Python
| 도구 | 설명 |
|------|------|
| `c4_status` | 프로젝트 상태 조회 (state, queue, workers, metrics) |
| `c4_start` | PLAN/HALTED → EXECUTE 전환 |
| `c4_clear` | 상태 초기화 (개발/디버깅용) |
| `c4_ensure_supervisor` | Supervisor Loop 실행 보장 |

### Task Management — Worker Mode (4개) — `Go: c4-core/internal/mcp/handlers/tasks.go`
| 도구 | 설명 |
|------|------|
| `c4_get_task` | Worker에게 태스크 할당 (+ 에이전트 라우팅) |
| `c4_submit` | 태스크 완료 보고 (commit_sha + validation_results) |
| `c4_add_todo` | 태스크 추가 (dependencies, model, execution_mode 지원) |
| `c4_mark_blocked` | 재시도 실패 → blocked 상태 + repair queue |

### Task Management — Direct Mode (2개) — `Go: c4-core/internal/mcp/handlers/tracking.go`
| 도구 | 설명 |
|------|------|
| `c4_claim` | 태스크 직접 시작 (Worker 프로토콜 우회, active_claim.json 생성) |
| `c4_report` | Direct 모드 완료 보고 (summary + files_changed) |

### Validation & Supervision (3개) — `Go: checkpoint` / `Python: validation, cleanup`
| 도구 | 설명 |
|------|------|
| `c4_run_validation` | lint, test 등 검증 실행 |
| `c4_checkpoint` | Supervisor 체크포인트 결정 (APPROVE/REQUEST_CHANGES/REPLAN) |
| `c4_cleanup_workers` | 좀비 워커 정리 (idle/TTL 만료/일관성 위반) |

### Discovery Phase (4개) — `Python: c4/mcp/handlers/discovery.py`
| 도구 | 설명 |
|------|------|
| `c4_save_spec` | EARS 요구사항 저장 |
| `c4_list_specs` | 요구사항 목록 조회 |
| `c4_get_spec` | 요구사항 상세 조회 |
| `c4_discovery_complete` | Discovery → Design 전환 |

### Design Phase (4개) — `Python: c4/mcp/handlers/design.py`
| 도구 | 설명 |
|------|------|
| `c4_save_design` | 설계 사양 저장 |
| `c4_get_design` | 설계 조회 |
| `c4_list_designs` | 설계 목록 |
| `c4_design_complete` | Design → Plan 전환 |

### Code Analysis & Symbols (6개) — `Python: c4/mcp/handlers/code_ops.py`
| 도구 | 설명 |
|------|------|
| `c4_find_symbol` | 심볼 검색 (함수/클래스/메서드, 디렉토리 재귀 지원) |
| `c4_get_symbols_overview` | 파일의 심볼 개요 (depth 조절 가능) |
| `c4_replace_symbol_body` | 심볼 본문 교체 (LSP → Tree-sitter fallback) |
| `c4_insert_before_symbol` | 심볼 앞에 내용 삽입 |
| `c4_insert_after_symbol` | 심볼 뒤에 내용 삽입 |
| `c4_rename_symbol` | 코드베이스 전체 심볼 이름 변경 (LSP ∪ Tree-sitter) |

### File Operations (6개) — `Python: c4/mcp/handlers/file_ops.py`
| 도구 | 설명 |
|------|------|
| `c4_read_file` | 파일 읽기 (라인 번호, 부분 읽기 지원) |
| `c4_create_text_file` | 파일 생성/덮어쓰기 |
| `c4_list_dir` | 디렉토리 목록 (재귀 옵션) |
| `c4_find_file` | glob 패턴 파일 검색 |
| `c4_search_for_pattern` | 정규식 검색 (context_lines 지원) |
| `c4_replace_content` | 파일 내용 교체 (리터럴/정규식) |

### Git & History (2개) — `Python: c4/mcp/handlers/git_ops.py`
| 도구 | 설명 |
|------|------|
| `c4_analyze_history` | 커밋 히스토리 분석 (클러스터링 + 스토리 생성) |
| `c4_search_commits` | 의미론적 커밋 검색 (작성자/날짜/경로 필터) |

### Worktree Management (2개) — `Python: c4/mcp/handlers/tasks.py`
| 도구 | 설명 |
|------|------|
| `c4_worktree_status` | Worker worktree 상태 조회 |
| `c4_worktree_cleanup` | worktree 정리 (활성 워커 유지 옵션) |

### Agent Routing (2개) — `Python: c4/mcp/handlers/core.py`
| 도구 | 설명 |
|------|------|
| `c4_test_agent_routing` | 에이전트 라우팅 테스트/디버깅 |
| `c4_query_agent_graph` | 에이전트 그래프 쿼리 (agents, skills, domains, chains) |

### GPU & Job Scheduling (3개) — `Python: c4/mcp/handlers/gpu.py`
| 도구 | 설명 |
|------|------|
| `c4_gpu_status` | GPU 상태 (개수, VRAM, 사용률, 백엔드) |
| `c4_job_submit` | GPU 작업 제출 (command + task_id 연계) |
| `c4_job_status` | GPU 작업 상태 조회 |

### Knowledge Store v2 (3개) — `Python: c4/mcp/handlers/knowledge.py`
| 도구 | 설명 |
|------|------|
| `c4_knowledge_search` | 하이브리드 검색 (Vector + FTS5, RRF merge, 필터링) |
| `c4_knowledge_record` | 지식 문서 생성 (experiment/pattern/insight/hypothesis) |
| `c4_knowledge_get` | 지식 문서 조회 (ID 기반, 백링크 포함) |

### Knowledge Legacy (v2 위임) (3개) — `Python: c4/mcp/handlers/knowledge.py`
| 도구 | 설명 |
|------|------|
| `c4_experiment_search` | 실험 검색 → `c4_knowledge_search` 위임 |
| `c4_experiment_record` | 실험 기록 → `c4_knowledge_record` 위임 |
| `c4_pattern_suggest` | 패턴 제안 → v2 search 사용 |

### Artifacts (3개) — `Python: c4/mcp/handlers/artifacts.py`
| 도구 | 설명 |
|------|------|
| `c4_artifact_list` | 태스크 아티팩트 목록 (이름, 유형, 크기, 해시) |
| `c4_artifact_save` | content-addressable 아티팩트 저장 (SHA256) |
| `c4_artifact_get` | 아티팩트 경로/메타데이터 조회 |

---

## 상세 스펙

### c4_status

프로젝트 상태를 조회합니다.

**파라미터**: 없음

**응답 예시**:
```json
{
  "initialized": true,
  "project_id": "my-project",
  "status": "EXECUTE",
  "queue": { "pending": 3, "in_progress": 1, "done": 5 },
  "workers": {},
  "metrics": { "tasks_completed": 42 },
  "parallelism": { "recommended": 7, "ready_now": 12 },
  "workflow": { "phase": "execute", "next": "worker_loop" }
}
```

---

### c4_start

PLAN 또는 HALTED → EXECUTE 전환.

**파라미터**: 없음

---

### c4_get_task

Worker에게 다음 태스크를 할당합니다.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `worker_id` | string | O | Worker 고유 식별자 |

**응답**: 할당된 태스크 정보 (task_id, title, dod, branch, recommended_agent, agent_chain) 또는 `null`

**동작**:
1. Worker 등록/갱신
2. pending 태스크 중 dependencies 완료 + scope 미잠금 태스크 검색
3. Scope Lock 획득, in_progress 전환
4. 에이전트 라우팅 정보 계산
5. TaskAssignment 반환

---

### c4_submit

태스크 완료 보고 (Worker 모드).

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `task_id` | string | O | 완료한 태스크 ID |
| `commit_sha` | string | O | Git commit SHA |
| `validation_results` | array | O | `[{name, status, message}]` |

---

### c4_claim

Direct 모드 태스크 시작. Worker 프로토콜을 우회하여 메인 세션에서 직접 작업합니다.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `task_id` | string | O | claim할 태스크 ID |

**동작**: 태스크를 in_progress로 전환, `.c4/active_claim.json` 생성 (Hook 강제용)

---

### c4_report

Direct 모드 완료 보고.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `task_id` | string | O | 완료한 태스크 ID |
| `summary` | string | O | 작업 요약 |
| `files_changed` | array | X | 변경 파일 목록 |

**동작**: 태스크 완료 처리, `active_claim.json` 삭제, review_required이면 리뷰 태스크 생성

---

### c4_add_todo

새 태스크를 큐에 추가합니다.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `task_id` | string | O | 태스크 ID (예: `T-001`) |
| `title` | string | O | 태스크 제목 |
| `dod` | string | O | Definition of Done |
| `scope` | string | X | 작업 범위 |
| `dependencies` | array | X | 선행 태스크 ID |
| `domain` | string | X | 도메인 (web-frontend, ml-dl 등) |
| `model` | string | X | 모델 티어 (opus/sonnet/haiku) |
| `priority` | integer | X | 우선순위 (높을수록 먼저) |
| `execution_mode` | string | X | worker/direct/auto (기본: worker) |
| `review_required` | boolean | X | 리뷰 태스크 자동 생성 (기본: true) |

---

### c4_checkpoint

Supervisor 체크포인트 결정.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `checkpoint_id` | string | O | 체크포인트 ID |
| `decision` | string | O | APPROVE / REQUEST_CHANGES / REPLAN |
| `notes` | string | O | 리뷰 코멘트 |
| `required_changes` | array | X | 변경 요청 목록 |

---

### c4_run_validation

검증 명령 실행.

| 파라미터 | 타입 | 필수 | 기본값 | 설명 |
|----------|------|------|--------|------|
| `names` | array | X | 전체 | 실행할 검증 이름 |
| `fail_fast` | boolean | X | true | 첫 실패 시 중단 |
| `timeout` | integer | X | 300 | 검증당 타임아웃(초) |

---

### c4_find_symbol

심볼을 패턴으로 검색합니다. LSP → Tree-sitter fallback 체인 사용.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `name_path_pattern` | string | O | 패턴 (예: `MyClass/method`, `function_name`) |
| `relative_path` | string | O | 파일 또는 디렉토리 경로 |
| `depth` | integer | X | 자식 심볼 포함 깊이 (기본: 0) |
| `include_body` | boolean | X | 소스코드 본문 포함 (기본: false) |

**응답**: 매칭 심볼 목록 (name, type, line, full_name, docstring, signature)

---

### c4_get_symbols_overview

파일의 심볼 구조를 개요로 반환합니다.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `relative_path` | string | O | 파일 경로 |
| `depth` | integer | X | 깊이 (기본: 0 = top-level only) |

---

### c4_replace_symbol_body

심볼 본문을 교체합니다. LSP → Tree-sitter fallback.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `name_path` | string | O | 심볼명 (예: `MyClass.method`) |
| `new_body` | string | O | 새 소스코드 |
| `file_path` | string | X | 파일 경로 (생략 시 자동 검색) |

---

### c4_rename_symbol

코드베이스 전체에서 심볼 이름을 변경합니다. LSP ∪ Tree-sitter 합집합으로 누락 방지.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `name_path` | string | O | 현재 심볼명 |
| `new_name` | string | O | 새 이름 |
| `file_path` | string | X | 정의 파일 |

---

### c4_knowledge_search

하이브리드 검색 (Vector + FTS5, RRF merge).

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `query` | string | O | 검색 쿼리 |
| `top_k` | integer | X | 최대 결과 수 (기본: 10) |
| `filters` | object | X | `{type, domain, hypothesis_status}` |

**응답**: `{count, results: [{id, title, type, domain, score, rrf_score}]}`

---

### c4_knowledge_record

Obsidian-style Markdown 지식 문서 생성.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `doc_type` | string | O | experiment / pattern / insight / hypothesis |
| `title` | string | O | 문서 제목 |
| `body` | string | X | Markdown 본문 |
| `domain` | string | X | 도메인 (ml, web, infra 등) |
| `tags` | array | X | 태그 목록 |
| `confidence` | number | X | 신뢰도 0.0-1.0 |
| `hypothesis` | string | X | 가설 텍스트 |
| `hypothesis_status` | string | X | proposed/testing/supported/refuted/inconclusive |
| `task_id` | string | X | 관련 태스크 ID |

**저장**: `.c4/knowledge/docs/{prefix}-{hash8}.md` (YAML frontmatter + body)

---

### c4_knowledge_get

ID로 지식 문서 조회.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `doc_id` | string | O | 문서 ID (예: `exp-a1b2c3d4`) |

**응답**: 전체 메타데이터 + body + backlinks

---

### c4_mark_blocked

태스크를 blocked 상태로 표시.

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `task_id` | string | O | 태스크 ID |
| `worker_id` | string | O | Worker ID |
| `failure_signature` | string | O | 에러 시그니처 |
| `attempts` | integer | O | 시도 횟수 |
| `last_error` | string | X | 마지막 에러 |

---

### c4_clear

C4 상태 초기화 (개발/디버깅용).

| 파라미터 | 타입 | 필수 | 설명 |
|----------|------|------|------|
| `confirm` | boolean | O | true 필수 |
| `keep_config` | boolean | X | config.yaml 유지 여부 |

---

## 에러 처리

모든 도구는 에러 시 `{"error": "메시지"}` 형식으로 응답합니다.

| 에러 | 원인 | 해결 |
|------|------|------|
| `C4 not initialized` | 프로젝트 미초기화 | `/c4-init` 실행 |
| `Invalid transition` | 잘못된 상태 전이 | `c4_status`로 현재 상태 확인 |
| `Task not found` | 존재하지 않는 태스크 | 태스크 ID 확인 |
| `Scope locked` | 다른 Worker가 작업 중 | 대기 또는 다른 태스크 |
| `Validation failed` | 검증 명령 실패 | 코드 수정 후 재시도 |

---

## 슬래시 명령어 매핑

| 슬래시 명령어 | MCP 도구 |
|--------------|----------|
| `/c4-status` | `c4_status` |
| `/c4-run` | `c4_start` + worker loop |
| `/c4-validate` | `c4_run_validation` |
| `/c4-checkpoint` | `c4_checkpoint` |
| `/c4-add-task` | `c4_add_todo` |
| `/c4-submit` | `c4_submit` |

---

## 다음 단계

- [명령어 레퍼런스](../user-guide/명령어-레퍼런스.md)
- [아키텍처](../developer-guide/아키텍처.md)
- [워크플로우 개요](../user-guide/워크플로우-개요.md)

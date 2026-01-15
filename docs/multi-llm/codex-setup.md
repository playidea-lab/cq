# Codex CLI로 C4 사용하기

OpenAI Codex CLI에서 C4 MCP 서버를 연결하여 프로젝트를 관리하는 방법입니다.

## 전제 조건

- [C4 설치 완료](../getting-started/설치-가이드.md)
- Codex CLI 설치 완료
- `c4d` 명령어가 PATH에 있어야 함

## 설정

### 1. MCP 서버 설정

`~/.codex/config.toml`에 C4 MCP 서버를 추가합니다:

```toml
[mcp_servers.c4]
command = "c4d"
args = ["mcp"]
```

또는 프로젝트별로 설정하려면 프로젝트 루트에 `.codex/config.toml`을 생성:

```toml
[mcp_servers.c4]
command = "c4d"
args = ["mcp"]
env = { C4_PROJECT_ROOT = "." }
```

### 2. 연결 확인

Codex CLI를 시작하고 MCP 서버 상태를 확인합니다:

```
codex> /mcp
```

`c4` 서버가 목록에 표시되어야 합니다.

## 사용법

### 상태 확인

```
codex> c4_status()
```

응답에 `workflow` 필드가 포함됩니다:

```json
{
  "status": "INIT",
  "workflow": {
    "phase": "init",
    "next": "discovery",
    "hint": "Start planning: scan docs/*.md for requirements..."
  }
}
```

`workflow.hint`를 따라 다음 단계를 수행하세요.

### 워크플로우 실행

#### 1. Discovery Phase (요구사항 수집)

```
codex> 프로젝트 문서를 분석하고 EARS 요구사항을 수집해줘

# Codex가 다음을 수행:
# 1. docs/*.md 스캔
# 2. 도메인 감지
# 3. c4_save_spec() 호출
# 4. c4_discovery_complete() 호출
```

#### 2. Design Phase (설계)

```
codex> 각 기능의 아키텍처를 설계해줘

# Codex가 다음을 수행:
# 1. c4_list_specs()로 명세 확인
# 2. 아키텍처 옵션 정의
# 3. c4_save_design() 호출
# 4. c4_design_complete() 호출
```

#### 3. Execute Phase (실행)

```
codex> Worker Loop를 시작해줘

# Codex가 다음을 수행:
# 1. c4_start()로 EXECUTE 상태 전환
# 2. c4_get_task(worker_id) 반복 호출
# 3. 각 태스크 구현
# 4. c4_run_validation() 실행
# 5. c4_submit() 호출
```

### MCP 도구 목록

| 도구 | 설명 |
|------|------|
| `c4_status()` | 프로젝트 상태 + 워크플로우 가이드 |
| `c4_get_task(worker_id)` | 다음 태스크 할당 |
| `c4_submit(task_id, commit_sha, validation_results)` | 태스크 완료 제출 |
| `c4_run_validation(names)` | lint, test 등 검증 실행 |
| `c4_start()` | 실행 시작 (PLAN → EXECUTE) |
| `c4_save_spec(...)` | EARS 요구사항 저장 |
| `c4_discovery_complete()` | Discovery 완료 |
| `c4_save_design(...)` | 설계 저장 |
| `c4_design_complete()` | Design 완료 |
| `c4_add_todo(...)` | 태스크 추가 |
| `c4_checkpoint(...)` | 체크포인트 결정 기록 |
| `c4_mark_blocked(...)` | 태스크 블록 처리 |

## 멀티 워커

여러 Codex CLI 세션에서 동시에 작업할 수 있습니다:

```
# 터미널 1 (Claude Code)
/c4-run

# 터미널 2 (Codex CLI)
codex> c4_get_task("codex-worker-1")
# ... 태스크 구현 ...
codex> c4_submit(task_id, commit_sha, validations)
```

모든 워커가 같은 SQLite DB (`.c4/c4.db`)를 사용하므로 태스크가 자동으로 분배됩니다.

## 트러블슈팅

### MCP 서버 연결 실패

```
Error: Failed to connect to MCP server 'c4'
```

**해결:**
1. `c4d mcp` 명령어가 작동하는지 확인
2. C4가 설치된 경로 확인
3. 환경 변수 `C4_PROJECT_ROOT` 설정

### 프로젝트 초기화 안됨

```json
{"success": false, "error": "C4 not initialized"}
```

**해결:**
```bash
cd /your/project
c4  # C4 초기화
```

## 참고

- [C4 빠른 시작](../getting-started/빠른-시작.md)
- [워크플로우 개요](../user-guide/워크플로우-개요.md)
- [Codex CLI MCP 문서](https://developers.openai.com/codex/mcp/)

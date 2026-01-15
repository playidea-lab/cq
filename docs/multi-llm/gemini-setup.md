# Gemini CLI로 C4 사용하기

Google Gemini CLI에서 C4 MCP 서버를 연결하여 프로젝트를 관리하는 방법입니다.

## 전제 조건

- [C4 설치 완료](../getting-started/설치-가이드.md)
- Gemini CLI 설치 완료 (`npm install -g @anthropic-ai/gemini-cli` 또는 공식 설치)
- `c4d` 명령어가 PATH에 있어야 함

## 설정

### 1. MCP 서버 설정

`~/.gemini/settings.json`에 C4 MCP 서버를 추가합니다:

```json
{
  "mcpServers": {
    "c4": {
      "command": "c4d",
      "args": ["mcp"]
    }
  }
}
```

프로젝트별로 설정하려면 프로젝트 루트에 `.gemini/settings.json`을 생성:

```json
{
  "mcpServers": {
    "c4": {
      "command": "c4d",
      "args": ["mcp"],
      "env": {
        "C4_PROJECT_ROOT": "."
      }
    }
  }
}
```

### 2. 연결 확인

Gemini CLI를 시작하고 MCP 도구를 확인합니다:

```
gemini> /tools
```

`c4_status`, `c4_get_task` 등의 도구가 표시되어야 합니다.

## 사용법

### 상태 확인

```
gemini> c4 프로젝트 상태를 확인해줘
```

Gemini가 `c4_status()`를 호출하고 응답의 `workflow` 필드를 해석합니다:

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

### 워크플로우 실행

#### 1. Discovery Phase (요구사항 수집)

```
gemini> 프로젝트 문서를 분석해서 C4 요구사항을 수집해줘

# Gemini가 workflow.hint를 따라:
# 1. docs/*.md 스캔
# 2. 도메인 감지
# 3. c4_save_spec() 호출
# 4. c4_discovery_complete() 호출
```

#### 2. Design Phase (설계)

```
gemini> 설계 단계를 진행해줘

# Gemini가:
# 1. c4_list_specs()로 명세 확인
# 2. 아키텍처 옵션 정의
# 3. c4_save_design() 호출
# 4. c4_design_complete() 호출
```

#### 3. Execute Phase (실행)

```
gemini> 태스크를 실행해줘

# Gemini가:
# 1. c4_start()로 EXECUTE 상태 전환
# 2. c4_get_task(worker_id) 반복
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

여러 CLI 세션에서 동시에 작업할 수 있습니다:

```
# 터미널 1 (Claude Code)
/c4-run

# 터미널 2 (Gemini CLI)
gemini> 다음 태스크를 할당받아서 처리해줘
# Gemini가 c4_get_task("gemini-worker-1") 호출
# ... 태스크 구현 ...
# c4_submit() 호출
```

모든 워커가 같은 SQLite DB (`.c4/c4.db`)를 사용하므로 태스크가 자동으로 분배됩니다.

## FastMCP 통합

Gemini CLI는 FastMCP와 원활하게 통합됩니다. Python으로 커스텀 MCP 서버를 만들어 C4와 연동할 수 있습니다:

```python
from fastmcp import FastMCP
import subprocess

mcp = FastMCP("c4-helper")

@mcp.tool()
def run_c4_workflow():
    """Run the full C4 workflow"""
    # c4d mcp와 통신
    ...
```

## 트러블슈팅

### MCP 서버 연결 실패

```
Error: Failed to start MCP server 'c4'
```

**해결:**
1. `c4d mcp` 명령어가 작동하는지 확인:
   ```bash
   c4d mcp  # 오류 없이 시작되어야 함
   ```
2. C4가 설치된 경로 확인
3. settings.json 문법 확인 (JSON 유효성)

### 프로젝트 초기화 안됨

```json
{"success": false, "error": "C4 not initialized"}
```

**해결:**
```bash
cd /your/project
c4  # C4 초기화
```

### 도구가 표시되지 않음

**해결:**
1. Gemini CLI 재시작
2. settings.json 경로 확인 (`~/.gemini/settings.json`)
3. MCP 서버 로그 확인

## 참고

- [C4 빠른 시작](../getting-started/빠른-시작.md)
- [워크플로우 개요](../user-guide/워크플로우-개요.md)
- [Gemini CLI MCP 문서](https://geminicli.com/docs/tools/mcp-server/)
- [Google Gemini CLI GitHub](https://github.com/google-gemini/gemini-cli)

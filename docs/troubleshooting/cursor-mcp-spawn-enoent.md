# Cursor MCP Spawn ENOENT 버그 분석

> **문서 작성일**: 2026-02-01
> **Task ID**: HF-002

## 증상

Cursor에서 C4 MCP 서버 실행 시 다음 오류 발생:

```
spawn ENOENT
```

또는 MCP 도구가 인식되지 않음.

## 원인 분석

### 1. 설정 파일 위치 차이

| 환경 | MCP 설정 파일 위치 |
|------|-------------------|
| Claude Code | 프로젝트 루트의 `.mcp.json` |
| Cursor | `~/.cursor/mcp.json` (글로벌) |

### 2. 설정 내용 차이

**Claude Code (`.mcp.json`)**:
```json
{
  "mcpServers": {
    "c4": {
      "type": "stdio",
      "command": "uv",
      "args": [
        "--directory",
        "/Users/changmin/git/c4",
        "run",
        "python",
        "-m",
        "c4.mcp_server"
      ],
      "env": {
        "C4_PROJECT_ROOT": "/Users/changmin/git/c4"
      }
    }
  }
}
```

**Cursor (`~/.cursor/mcp.json`)**:
```json
{
  "mcpServers": {
    "c4": {
      "command": "uv",
      "args": ["run", "--directory", "/Users/changmin/.c4", "python", "-m", "c4.mcp_server"],
      "cwd": "${workspaceFolder}",
      "env": {
        "C4_PROJECT_ROOT": "${workspaceFolder}"
      }
    }
  }
}
```

### 3. 핵심 문제점

1. **잘못된 디렉토리**: Cursor 설정이 `~/.c4` (글로벌 설치)를 참조하지만, 실제 사용하려는 프로젝트는 `/Users/changmin/git/c4`
2. **`${workspaceFolder}` 미지원**: Cursor가 이 변수를 제대로 확장하지 못할 수 있음
3. **`type` 필드 누락**: Claude Code는 `"type": "stdio"`를 명시하지만 Cursor 설정에는 없음
4. **`--directory` 옵션 위치**: Claude Code는 `uv --directory <path> run`이지만 Cursor는 `uv run --directory <path>` (순서 차이)

### 4. ENOENT 발생 시나리오

1. Cursor가 `uv run --directory ~/.c4 python -m c4.mcp_server` 실행
2. `~/.c4/`에 c4 패키지가 설치되어 있지 않거나 버전이 다름
3. 또는 `uv` 명령이 PATH에 없음
4. → `spawn ENOENT` 오류

## 해결 방법

### 방법 1: Cursor 글로벌 설정 수정 (권장)

`~/.cursor/mcp.json` 파일을 다음과 같이 수정:

```json
{
  "mcpServers": {
    "c4": {
      "type": "stdio",
      "command": "/Users/changmin/.local/bin/uv",
      "args": [
        "--directory",
        "/Users/changmin/git/c4",
        "run",
        "python",
        "-m",
        "c4.mcp_server"
      ],
      "env": {
        "C4_PROJECT_ROOT": "/Users/changmin/git/c4",
        "PATH": "/Users/changmin/.local/bin:/usr/local/bin:/usr/bin:/bin"
      }
    }
  }
}
```

**변경 사항**:
- `command`에 `uv` 절대 경로 사용
- `--directory`를 `uv` 바로 뒤에 배치 (순서 수정)
- 프로젝트 경로를 절대 경로로 지정
- `PATH` 환경 변수 명시적 설정

### 방법 2: 프로젝트별 설정 사용

프로젝트 루트에 `.cursor/mcp.json` 파일 생성:

```json
{
  "mcpServers": {
    "c4": {
      "type": "stdio",
      "command": "uv",
      "args": [
        "--directory",
        "${workspaceFolder}",
        "run",
        "python",
        "-m",
        "c4.mcp_server"
      ],
      "env": {
        "C4_PROJECT_ROOT": "${workspaceFolder}"
      }
    }
  }
}
```

> **주의**: Cursor가 프로젝트별 MCP 설정을 지원하는지 확인 필요.

### 방법 3: Shell Wrapper 사용

1. `/Users/changmin/.local/bin/c4-mcp` 스크립트 생성:

```bash
#!/bin/bash
cd "${C4_PROJECT_ROOT:-$(pwd)}"
exec uv run python -m c4.mcp_server "$@"
```

2. 실행 권한 부여: `chmod +x ~/.local/bin/c4-mcp`

3. Cursor 설정 수정:
```json
{
  "mcpServers": {
    "c4": {
      "command": "/Users/changmin/.local/bin/c4-mcp",
      "env": {
        "C4_PROJECT_ROOT": "/Users/changmin/git/c4"
      }
    }
  }
}
```

## Claude Code vs Cursor 환경 차이점 요약

| 항목 | Claude Code | Cursor |
|------|-------------|--------|
| 설정 파일 | `.mcp.json` (프로젝트) | `~/.cursor/mcp.json` (글로벌) |
| 변수 지원 | 없음 (절대 경로 필요) | `${workspaceFolder}` (미검증) |
| `type` 필드 | `"stdio"` 명시 | 생략 가능 |
| PATH 상속 | 쉘 환경 상속 | 제한적 상속 가능 |
| 프로젝트 컨텍스트 | 자동 감지 | 명시적 설정 필요 |

## 검증 체크리스트

설정 변경 후 다음을 확인:

- [ ] `uv` 명령 경로 확인: `which uv`
- [ ] C4 프로젝트 경로 확인: `ls /Users/changmin/git/c4/c4/mcp_server.py`
- [ ] MCP 서버 수동 실행 테스트:
  ```bash
  cd /Users/changmin/git/c4
  uv run python -m c4.mcp_server
  ```
- [ ] Cursor 재시작 후 MCP 도구 인식 확인

## 관련 이슈

- Cursor MCP 문서: https://docs.cursor.com/context/model-context-protocol
- C4 MCP 서버 코드: `c4/mcp_server.py`

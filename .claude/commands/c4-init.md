# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

### Step 1: C4 설치 경로 확인

```bash
C4_DIR=$(cat ~/.c4-install-path 2>/dev/null || echo "$HOME/git/c4")
```

### Step 2: 초기화 실행

```bash
uv run --directory "$C4_DIR" c4 init --path "$(pwd)"
```

**참고**: project-id는 자동으로 현재 폴더 이름 사용. `$ARGUMENTS`가 있으면 그걸 사용:

```bash
# ARGUMENTS가 있는 경우만
uv run --directory "$C4_DIR" c4 init --path "$(pwd)" --project-id "$ARGUMENTS"
```

### Step 3: 재시작 안내

**중요**: `.mcp.json`이 새로 생성된 경우 Claude Code 재시작이 필요합니다.

MCP 서버는 Claude Code 시작 시에만 로드되므로, 새 프로젝트에서는:

1. Claude Code 종료
2. 터미널에서 다시 시작

**권장 워크플로우 (재시작 불필요)**:

```bash
# 터미널에서 실행 - 자동 init + Claude Code 시작
cd /path/to/project
c4
```

## Usage

```text
/c4-init           # 폴더 이름으로 project-id 자동 설정
/c4-init myproject # 명시적 project-id 지정
```

## 새 프로젝트 권장 방법

Claude Code 안에서 `/c4-init` 대신, 터미널에서:

```bash
c4                 # 현재 디렉토리에서 auto-init + Claude 시작
c4 --path /other   # 다른 디렉토리 지정
```

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

`.mcp.json`이 새로 생성된 경우 Claude Code 재시작 필요.

## Usage

```
/c4-init           # 폴더 이름으로 project-id 자동 설정
/c4-init myproject # 명시적 project-id 지정
```

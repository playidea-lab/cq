---
name: c4-init
description: |
  Initialize C4 in the current project directory. Detects C4 installation path,
  runs init with optional project-id, and handles session restart logic. Use when
  the user wants to set up C4 for a new project or re-initialize after clearing.
  Triggers: "C4 초기화", "프로젝트 설정", "init c4", "initialize project",
  "set up c4".
allowed-tools: Bash(cq *), mcp__cq__*
---

# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

### Step 1: C4 설치 경로 확인

```bash
C4_DIR=$(cat ~/.c4-install-path 2>/dev/null || echo "$HOME/git/cq")
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

### Step 3: 완료 확인

**재시작 필요 여부:**

| 상황 | 재시작 |
|------|--------|
| `.mcp.json`이 이미 존재 (c4 clear 후 재init) | **불필요** - MCP가 자동으로 새 상태 인식 |
| `.mcp.json`이 새로 생성됨 (최초 init) | **필요** - MCP 서버 로드 필요 |

**확인 방법**: `/c4-status` 실행하여 상태가 정상 표시되면 재시작 불필요.

**새 프로젝트 권장 워크플로우**:

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

# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

### Step 1: 사전 체크

1. 현재 디렉토리 경로 확인 (`pwd`)
2. `mcp__c4__c4_status` 호출하여 이미 초기화되었는지 확인
3. 이미 초기화된 경우 상태 표시 후 종료

### Step 2: Git 확인

1. `.git` 디렉토리 존재 확인
2. 없으면 `git init` 실행 (C4는 git이 필요함)

### Step 3: MCP 로컬 설정 생성

`.claude/settings.json` 파일 생성 (없으면):

```json
{
  "mcpServers": {
    "c4": {
      "command": "uv",
      "args": ["--directory", "$C4_INSTALL_DIR", "run", "python", "-m", "c4.mcp_server"],
      "env": {
        "C4_PROJECT_ROOT": "$CURRENT_PROJECT_DIR"
      }
    }
  }
}
```

**중요**:
- `$C4_INSTALL_DIR`는 C4가 설치된 경로 (보통 `~/.c4` 또는 사용자 지정 경로)
- `$CURRENT_PROJECT_DIR`는 현재 프로젝트의 절대 경로

C4 설치 경로 찾기:
```bash
# 글로벌 설정에서 확인
cat ~/.claude.json | grep -A5 '"c4"'
```

### Step 4: C4 초기화

```bash
uv run --directory $C4_INSTALL_DIR c4 init --project-id "$ARGUMENTS"
```

- `$ARGUMENTS`가 없으면 현재 디렉토리 이름 사용

### Step 5: 확인

1. `mcp__c4__c4_status` 호출하여 초기화 확인
2. 생성된 파일 목록 표시:
   - `.c4/` - C4 상태 디렉토리
   - `.c4/config.yaml` - 프로젝트 설정
   - `.c4/state.json` - 상태 파일
   - `.claude/settings.json` - MCP 로컬 설정 (새로 생성)

### Step 6: 안내

```
✅ C4 초기화 완료!

⚠️  중요: Claude Code를 재시작하세요 (MCP 설정 반영)

다음 단계:
  /c4-plan    - 기획 문서 해석 및 태스크 생성
  /c4-status  - 상태 확인
```

## Usage

```
/c4-init [project-id]
```

## Example

```
/c4-init my-awesome-project
```

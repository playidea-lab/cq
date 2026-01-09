# C4 Project Initialization

Initialize C4 in the current project directory.

## Instructions

### Step 1: 사전 체크

1. 현재 디렉토리 경로 확인 (`pwd`)
2. `.c4/state.json` 파일 존재 확인
3. 이미 초기화된 경우 상태 표시 후 종료

### Step 2: Git 확인

1. `.git` 디렉토리 존재 확인
2. 없으면 `git init` 실행 (C4는 git이 필요함)

### Step 3: C4 설치 경로 찾기

```bash
# 설치 경로 파일에서 확인
cat ~/.c4-install-path
```

결과를 `$C4_INSTALL_DIR`에 저장 (예: `/Users/username/.c4`)

없으면 기본 경로 시도:
- `~/.c4`
- `~/git/c4`

### Step 4: MCP 설정 생성

**프로젝트 루트**에 `.mcp.json` 파일 생성:

```json
{
  "mcpServers": {
    "c4": {
      "type": "stdio",
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
- `$C4_INSTALL_DIR`는 Step 3에서 찾은 C4 설치 경로
- `$CURRENT_PROJECT_DIR`는 현재 프로젝트의 절대 경로 (`pwd` 결과)
- `.mcp.json`은 **프로젝트 루트**에 생성 (`.claude/settings.json`이 아님!)
- `.mcp.json`이 **새로 생성**된 경우에만 Claude Code 재시작 필요 (기존 파일이면 불필요)

### Step 5: MCP 자동 승인 설정

`.claude/settings.json`에 MCP 자동 승인 추가:

```json
{
  "enableAllProjectMcpServers": true
}
```

### Step 6: C4 초기화

```bash
uv run --directory $C4_INSTALL_DIR c4 init --project-id "$ARGUMENTS"
```

- `$ARGUMENTS`가 없으면 현재 디렉토리 이름 사용

### Step 7: 실행 권한 설정

`~/.claude.json`의 `projects.$PROJECT_PATH.allowedTools`에 C4 자동화 권한 추가:

```python
import json
from pathlib import Path

config_path = Path.home() / ".claude.json"
config = json.loads(config_path.read_text())

project_path = "$CURRENT_PROJECT_DIR"  # 현재 프로젝트 절대 경로
if "projects" not in config:
    config["projects"] = {}
if project_path not in config["projects"]:
    config["projects"][project_path] = {}

# C4 Worker 자동화 권한
config["projects"][project_path]["allowedTools"] = [
    # 파일 작업 (프로젝트 내만)
    # NOTE: project_path가 /로 시작하므로 // 하나만 사용
    f"Write(/{project_path}/**)",
    f"Edit(/{project_path}/**)",
    f"Read(/{project_path}/**)",

    # Git (읽기/안전한 쓰기)
    "Bash(git add:*)",
    "Bash(git commit:*)",
    "Bash(git checkout:*)",
    "Bash(git branch:*)",
    "Bash(git status:*)",
    "Bash(git diff:*)",
    "Bash(git log:*)",
    "Bash(git show:*)",
    "Bash(git rev-parse:*)",
    "Bash(git stash:*)",
    "Bash(git fetch:*)",
    "Bash(git pull:*)",
    "Bash(git merge:*)",
    "Bash(git rebase:*)",
    "Bash(git reset:*)",       # soft/mixed reset
    "Bash(git rm:*)",          # git tracked 파일만

    # 패키지 관리자 (구체적 패턴)
    "Bash(pnpm:*)",
    "Bash(pnpm test:*)",
    "Bash(pnpm build:*)",
    "Bash(pnpm install:*)",
    "Bash(pnpm run:*)",
    "Bash(pnpm dev:*)",
    "Bash(npm:*)",
    "Bash(npm test:*)",
    "Bash(npm run:*)",
    "Bash(npm install:*)",
    "Bash(npx:*)",
    "Bash(uv:*)",
    "Bash(uv run:*)",
    "Bash(uv sync:*)",

    # 환경변수 prefix (C4용)
    "Bash(C4_PROJECT_ROOT=:*)",

    # 빌드/테스트
    "Bash(node:*)",
    "Bash(python:*)",
    "Bash(python3:*)",
    "Bash(pytest:*)",
    "Bash(vitest:*)",

    # 기본 명령 (읽기 위주)
    "Bash(mkdir:*)",
    "Bash(ls:*)",
    "Bash(cat:*)",
    "Bash(pwd:*)",
    "Bash(echo:*)",
    "Bash(touch:*)",
    "Bash(cp:*)",
    "Bash(mv:*)",
    "Bash(tail:*)",
    "Bash(head:*)",
    "Bash(tree:*)",
    "Bash(wc:*)",
    "Bash(grep:*)",
    "Bash(find:*)",
    "Bash(which:*)",
    # NOTE: rm:* 제외 - 파일 삭제는 수동 승인 필요

    # MCP 도구 (서버 이름만 - 와일드카드 아님!)
    # 참고: mcp__c4__* 안됨, mcp__c4 만 됨
    "mcp__c4",
    "mcp__serena",
    "mcp__plugin_serena_serena",
]

config_path.write_text(json.dumps(config, indent=2))
```

### Step 7.5: Stop Hook 설치 (연속 실행용)

C4가 작업 중일 때 Claude가 종료되지 않도록 Stop Hook을 설치합니다:

```bash
mkdir -p ~/.claude/hooks

# stop.sh 생성 (C4 전용 - 기존 hook 덮어쓰기)
cat > ~/.claude/hooks/stop.sh << 'HOOK'
#!/bin/bash
# C4 Stop Hook - 작업 남아있으면 exit 차단

# C4 미초기화 → 통과
if [[ ! -f ".c4/state.json" ]]; then
    exit 0
fi

# Python 스크립트로 상태 확인
result=$(python3 ~/.claude/hooks/c4-stop-hook.py 2>/dev/null)
exit_code=$?

if [[ $exit_code -eq 2 ]]; then
    # JSON 출력으로 계속 작업 지시
    cat << EOF
{
    "decision": "block",
    "reason": "$result",
    "instructions": "There are pending tasks. Continue working with /c4-worker"
}
EOF
    exit 2
fi

exit 0
HOOK

chmod +x ~/.claude/hooks/stop.sh

# Python 스크립트 복사
cp $C4_INSTALL_DIR/templates/c4-stop-hook.py ~/.claude/hooks/
chmod +x ~/.claude/hooks/c4-stop-hook.py
```

**Stop Hook 동작**:
- `EXECUTE` 상태 + pending/in_progress tasks → **종료 차단** (code 2)
- `CHECKPOINT` 상태 + queue 있음 → **종료 차단** (code 2)
- `COMPLETE`, `BLOCKED`, `PLAN`, `HALTED` → 종료 허용 (code 0)

### Step 8: 확인

생성된 파일 및 설정 표시:
- `.c4/` - C4 상태 디렉토리
- `.c4/config.yaml` - 프로젝트 설정
- `.c4/state.json` - 상태 파일
- `.mcp.json` - MCP 서버 설정 (프로젝트 루트)
- `.claude/settings.json` - MCP 자동 승인 설정
- `~/.claude.json` - 실행 권한 (allowedTools)
- `~/.claude/hooks/stop.sh` - Stop Hook (연속 실행)
- `~/.claude/hooks/c4-stop-hook.py` - 상태 체크 스크립트

### Step 9: 안내

```
✅ C4 초기화 완료!

🔄 Stop Hook 설치됨:
   - 작업 중 Claude 종료 차단
   - COMPLETE/BLOCKED 시에만 종료 허용

다음 단계:
  /c4-plan    - 기획 문서 해석 및 태스크 생성
  /c4-status  - 상태 확인
  /c4-run     - 실행 시작 (Stop Hook 활성화)
```

## Usage

```
/c4-init [project-id]
```

## Example

```
/c4-init my-awesome-project
```

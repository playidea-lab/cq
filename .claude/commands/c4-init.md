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

    # Bash 전체 허용 (Security Hook이 위험 명령 차단)
    # Step 7.6에서 설치한 c4-bash-security-hook.sh가 다음을 차단:
    #   - rm -rf /, rm -rf ~  (대량 삭제)
    #   - chmod 777, sudo chmod (권한 변경)
    #   - git push --force main (강제 푸시)
    #   - curl|sh, wget|bash (원격 실행)
    #   - npm publish (의도치 않은 배포)
    "Bash(*)",

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

### Step 7.6: Bash Security Hook 설치 (위험 명령 차단)

`Bash(*)` 권한 사용 시 위험한 명령을 차단하는 보안 Hook을 설치합니다:

```bash
# Security hook 복사
cp $C4_INSTALL_DIR/templates/c4-bash-security-hook.sh ~/.claude/hooks/
chmod +x ~/.claude/hooks/c4-bash-security-hook.sh
```

**~/.claude/settings.json**에 hook 등록 추가:

```python
import json
from pathlib import Path

settings_path = Path.home() / ".claude" / "settings.json"
settings_path.parent.mkdir(parents=True, exist_ok=True)

# 기존 설정 로드 또는 새로 생성
if settings_path.exists():
    settings = json.loads(settings_path.read_text())
else:
    settings = {}

# hooks 구조 초기화
if "hooks" not in settings:
    settings["hooks"] = {}
if "pre-tool-use" not in settings["hooks"]:
    settings["hooks"]["pre-tool-use"] = []

# Bash security hook 추가 (중복 체크)
bash_hook = {
    "matcher": "Bash",
    "hooks": [
        {
            "type": "command",
            "command": "~/.claude/hooks/c4-bash-security-hook.sh"
        }
    ]
}

# 기존 Bash hook이 있으면 제거 후 새로 추가
settings["hooks"]["pre-tool-use"] = [
    h for h in settings["hooks"]["pre-tool-use"]
    if h.get("matcher") != "Bash"
]
settings["hooks"]["pre-tool-use"].append(bash_hook)

settings_path.write_text(json.dumps(settings, indent=2))
```

**차단되는 위험 명령**:
| 카테고리 | 예시 | 이유 |
|---------|------|------|
| 대량 삭제 | `rm -rf /`, `rm -rf ~` | 시스템 파괴 방지 |
| 권한 변경 | `chmod 777`, `sudo chmod` | 보안 유지 |
| Git 강제 | `git push --force main` | 코드 손실 방지 |
| 원격 실행 | `curl ... \| sh` | 악성코드 방지 |
| 시스템 | `dd if=`, `mkfs` | 디스크 손상 방지 |
| 퍼블리시 | `npm publish` | 의도치 않은 배포 방지 |

### Step 8: 확인

생성된 파일 및 설정 표시:
- `.c4/` - C4 상태 디렉토리
- `.c4/config.yaml` - 프로젝트 설정
- `.c4/state.json` - 상태 파일
- `.mcp.json` - MCP 서버 설정 (프로젝트 루트)
- `.claude/settings.json` - MCP 자동 승인 설정
- `~/.claude.json` - 실행 권한 (allowedTools)
- `~/.claude/settings.json` - Hook 등록 설정
- `~/.claude/hooks/stop.sh` - Stop Hook (연속 실행)
- `~/.claude/hooks/c4-stop-hook.py` - 상태 체크 스크립트
- `~/.claude/hooks/c4-bash-security-hook.sh` - Bash 보안 Hook

### Step 9: 안내

```
✅ C4 초기화 완료!

🔒 보안 설정:
   - Bash Security Hook: 위험 명령 자동 차단
   - rm -rf /, chmod 777, git push --force main 등 차단

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

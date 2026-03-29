# Cursor에서 C4 사용하기

Cursor IDE에서 C4 MCP를 사용하면 Claude Code와 동일한 워크플로우(plan → run → checkpoint → refine → finish)를 쓸 수 있습니다.

## 빠른 시작: `cq cursor`로 init 후 실행

프로젝트 루트에서 다음 한 줄이면 C4용 Cursor 설정이 되고, Cursor가 해당 폴더를 열며 MCP가 로드된 상태로 시작합니다.

```bash
cq cursor
```

또는 프로젝트 경로를 지정하려면:

```bash
cq cursor --dir /path/to/your/cq-project
```

동작 요약:

1. **.c4/** 디렉터리 생성 (없을 때)
2. **.mcp.json** (Claude Code용) 설정
3. **.cursor/mcp.json** 생성·갱신 — Cursor가 이 프로젝트를 열 때 C4 MCP를 자동 로드
4. **Cursor 실행** — 지정한(또는 현재) 디렉터리를 워크스페이스로 열어서 시작

Cursor가 뜬 뒤에는 MCP가 이미 연결된 상태이므로, “상태 확인해줘”, “계획 세워줘” 등으로 바로 C4 워크플로우를 사용할 수 있습니다.

## 전제 조건

- **cq 바이너리**: `~/.local/bin/cq`에 설치되어 있고 PATH에 있거나, Cursor MCP 설정에서 절대 경로 지정
- **Cursor CLI**: 터미널에서 `cursor` 명령이 동작해야 함 (Cursor 설치 시 “Shell Command: Install ‘cursor’ command” 실행)
- **프로젝트 루트에서 열기**: `cq cursor`로 실행하면 자동으로 해당 폴더가 열리므로, 수동으로 열 때도 C4 프로젝트 루트를 열어 두면 됨

## 1. MCP 서버 설정 (수동 시)

### 프로젝트 설정 (권장)

이 저장소에는 **`.cursor/mcp.json`**이 포함되어 있습니다. Cursor는 이 파일을 읽어 cq MCP 서버를 띄웁니다.

```json
{
  "mcpServers": {
    "cq": {
      "command": "cq",
      "args": ["mcp", "--dir", "."],
      "env": {},
      "type": "stdio"
    }
  }
}
```

- **command**: `cq`는 PATH에 있어야 합니다. 설치 후 `~/.local/bin`을 PATH에 넣거나, `"command": "/Users/본인계정/.local/bin/cq"`처럼 절대 경로를 쓸 수 있습니다.
- **args**: `--dir .`는 “현재 작업 디렉터리 = 프로젝트 루트”를 의미합니다. Cursor가 MCP를 띄울 때 **워크스페이스 루트를 cwd로 사용**하므로, **반드시 프로젝트 루트 폴더를 연 상태**에서 사용하세요.

### MCP가 프로젝트를 못 찾을 때

“project root not found” 같은 오류가 나면, Cursor가 MCP 프로세스를 다른 cwd에서 띄우는 경우입니다. 이때는 **환경 변수**로 프로젝트 루트를 고정하세요.

- **Cursor 설정 UI**: Settings (Cmd+,) → Tools & MCP → cq 서버 → Environment에 추가  
  - `C4_PROJECT_ROOT` = `/실제/경로/cq`
- **또는 `.cursor/mcp.json`에서**:
  ```json
  "env": {
    "C4_PROJECT_ROOT": "/실제/경로/cq"
  }
  ```
  그리고 `args`를 `["mcp"]`만 두면, cq는 `C4_PROJECT_ROOT`를 사용합니다.

### 적용 방법

설정을 바꾼 뒤에는 **Cursor를 완전히 종료했다가 다시 실행**해야 MCP가 다시 연결됩니다.

## 2. C4 워크플로우 사용

AGENTS.md(CLAUDE.md)에 적힌 대로, Cursor 에이전트도 **C4 MCP 도구를 우선** 사용합니다.

| 사용자 의도     | C4 MCP 도구 예시                          |
|----------------|--------------------------------------------|
| 상태 확인      | `c4_status`                                |
| 태스크 추가    | `c4_add_todo`                              |
| 계획/설계      | `/plan` 트리거 → `.claude/skills/plan/SKILL.md` 절차 + MCP 도구 |
| 실행           | `/run` → Worker 스폰 등                 |
| 마무리         | `/finish` → 빌드·테스트·커밋·지식 기록  |

“상태 확인해줘”, “태스크 추가해줘”, “계획 세워줘”, “실행해줘”, “마무리해줘”처럼 말하면 에이전트가 C4 스킬과 MCP 도구를 사용하도록 유도할 수 있습니다.

## 3. Cursor 제한 사항

- **25회 Tool Call 제한**: Agent 모드에서 도구 호출이 25회까지로 제한됩니다. `/run`처럼 호출이 많은 작업은 중간에 “continue”로 이어서 실행하거나, **MAX 모드**(약 200회)를 쓰거나, 완전 자동화가 필요하면 Claude Code/Codex CLI를 사용하세요.
- **cursor-agent(headless)**: `cursor-agent`의 MCP spawn 버그로 인해 MCP 도구가 동작하지 않을 수 있습니다. 자동화 시에는 `cursor-agent -p --force "cq status 실행해줘"`처럼 **cq CLI를 bash로 호출**하는 방식을 권장합니다.

자세한 내용은 [문제 해결 — Cursor](문제-해결.md#cursor-25회-tool-call-제한)을 참고하세요.

## 4. 요약

**권장**: 프로젝트 루트에서 `cq cursor`를 실행하면 `.cursor/mcp.json`이 설정되고 Cursor가 MCP 로드된 상태로 시작합니다.

**수동**:
1. **프로젝트 루트**를 Cursor에서 연다.  
2. **`.cursor/mcp.json`**으로 cq MCP가 연결되도록 되어 있는지 확인하고, 필요 시 `C4_PROJECT_ROOT`를 설정한다.  
3. Cursor를 **한 번 재시작**한 뒤, “상태 확인해줘”, “계획 세워줘” 등으로 C4 워크플로우를 사용한다.

재미나, Codex처럼 Cursor에서도 C4 MCP로 동일한 오케스트레이션을 사용할 수 있습니다.

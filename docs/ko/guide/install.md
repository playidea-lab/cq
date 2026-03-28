# 설치 가이드

2분 안에 CQ를 실행해보세요.

## 한 줄 설치

```sh
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

`cq` 바이너리를 설치하고, `.mcp.json`을 설정하며, `.c4/`를 초기화합니다. 설치 후 Claude Code를 재시작하면 169개의 MCP 도구가 자동으로 등록됩니다.

### 커스텀 설치 디렉토리

```sh
C4_INSTALL_DIR=/opt/cq curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

### 비인터랙티브 설치 (CI / 헤드리스 환경)

```sh
C4_GLOBAL_INSTALL=y curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | sh
```

## 설치 스크립트가 하는 일

1. Go 1.22+, Python 3.11+, uv 확인
2. 저장소 클론 (이미 있으면 `git pull`)
3. Go 바이너리 빌드 (`c4-core/bin/cq`)
4. Python 의존성 설치 (`uv sync`)
5. `.mcp.json` 병합 (기존 항목 보존)
6. `.c4/` 디렉토리 초기화
7. 선택적으로 `~/.local/bin/cq`를 전역으로 설치

## 사전 요구사항

| 항목 | 필수 여부 | 비고 |
|------|---------|------|
| Go 1.22+ | 필수 | MCP 서버 빌드용 |
| Python 3.11+ | 필수 | LSP / 문서 파싱 사이드카 |
| uv | 필수 | Python 패키지 매니저 — [설치](https://docs.astral.sh/uv/) |
| Claude Code | 필수 | CQ가 연결되는 AI 도구 |
| jq | 선택 | `.mcp.json` 병합을 더 빠르게 |

## 플랫폼별 참고사항

### macOS (ARM64)

바이너리를 `cp`로 복사하지 마세요 — 코드 서명을 유지하려면 `go build -o`를 직접 사용하세요:

```sh
# 잘못된 방법 — 코드 서명이 깨짐
cp c4-core/bin/cq ~/.local/bin/cq

# 올바른 방법
cd c4-core && go build -o ~/.local/bin/cq ./cmd/c4/
```

### Linux (systemd)

부팅 시 자동 시작을 위해 CQ를 사용자 서비스로 등록하세요:

```sh
cq serve install    # 서비스 등록
cq serve status     # 상태 확인
cq serve start      # 시작
```

### Windows (WSL2)

CQ는 WSL2를 인식합니다. Relay 컴포넌트가 WSL2 네트워킹에서 흔히 발생하는 NAT 타임아웃을 방지하기 위해 TCP keepalive를 자동으로 적용합니다.

## 설치 확인

```sh
cq doctor
```

예상 출력:

```
[✓] cq binary: v1.37
[✓] Claude Code: installed
[✓] MCP server: .mcp.json connected
[✓] .c4/ directory: initialized
```

`[✗]` 항목이 있으면 계속 진행하기 전에 수정하세요.

## 로그인

클라우드 동기화, Growth Loop, Research Loop를 사용하려면 인증이 필요합니다:

```sh
cq auth login      # 브라우저를 통한 GitHub OAuth
cq auth status     # 로그인 확인
```

API 키가 필요 없습니다 — 자격증명이 빌드 시점에 바이너리에 내장됩니다.

## MCP 설정

설치 스크립트가 `.mcp.json`을 자동으로 작성합니다. 참고로 구조는 다음과 같습니다:

```json
{
  "mcpServers": {
    "cq": {
      "command": "/path/to/cq/c4-core/bin/cq",
      "args": ["mcp", "--dir", "/path/to/cq"],
      "env": {
        "C4_PROJECT_ROOT": "/path/to/cq"
      }
    }
  }
}
```

모든 프로젝트에서 전역으로 사용하려면 `~/.claude.json`에 추가하세요:

```json
{
  "mcpServers": {
    "cq": {
      "command": "~/.local/bin/cq",
      "args": ["mcp"]
    }
  }
}
```

`.mcp.json` 변경 후에는 Claude Code를 재시작하세요.

## 전역 옵션

### `--global-mcp`

모든 프로젝트에서 CQ를 전역 MCP 서버로 설치합니다 (`~/.claude.json`에 기록):

```sh
cd ~/c4 && ./install.sh --global-mcp
```

이 옵션 없이는 CQ가 `.mcp.json`에 프로젝트별로 등록됩니다.

### `--global-skills`

모든 Claude Code 세션에서 CQ Skill을 사용할 수 있도록 전역으로 설치합니다:

```sh
cd ~/c4 && ./install.sh --global-skills
```

---

## 업데이트

```sh
cq update    # 최신 바이너리를 가져와 재빌드
```

## 삭제

```sh
# 1. .mcp.json 또는 ~/.claude.json에서 "cq" 항목 제거
# 2. 선택적으로 바이너리 제거
rm -f ~/.local/bin/cq
# 3. 선택적으로 소스 제거
rm -rf /path/to/cq
```

## 문제 해결

| 증상 | 해결 방법 |
|------|---------|
| "MCP server not found" | `.mcp.json`의 바이너리 경로 확인; `cd c4-core && go build -o bin/cq ./cmd/c4/`로 재빌드 |
| macOS 코드 서명 오류 | `go build -o` 직접 사용, `cp` 사용 금지 |
| Python 사이드카 오류 | `uv sync` 실행; `python3 --version`으로 Python 3.11+ 확인 |
| Go 빌드 실패 | `go version` 실행 (1.22+ 필요); `cd c4-core && go mod download` |
| `c4_llm_call` 도구 없음 | 환경에 `ANTHROPIC_API_KEY` 또는 `OPENAI_API_KEY` 설정 |

## 다음 단계

- [빠른 시작](quickstart.md) — 5분 안에 첫 번째 계획 실행
- [티어](tiers.md) — solo, connected, full 비교

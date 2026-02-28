# OpenCode 가이드

> Claude Code 구독 없이 API 키만으로 C4를 사용하는 방법

---

## 개요

[OpenCode](https://opencode.ai/)는 Go 기반 오픈소스 TUI로, 다양한 LLM Provider를 지원합니다. C4 MCP 서버와 연동하여 Claude Code 구독 없이도 C4를 사용할 수 있습니다.

### 비용 비교 (팀 5명 기준)

| 방식 | 월 비용 | 비고 |
|------|--------|------|
| Claude Code 구독 | $100 | $20 x 5명 |
| **OpenCode + API** | **$25-50** | Sonnet 기준 |
| **절감** | **50-75%** | |

---

## 설치

### 1. OpenCode 설치

```bash
# Go 설치 필요 (1.21+)
go install github.com/opencode-ai/opencode@latest

# 또는 바이너리 다운로드
# https://github.com/opencode-ai/opencode/releases
```

### 2. C4 설치

```bash
curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
```

### 3. API 키 설정

C4에 API 키를 저장하고, OpenCode에서 사용할 수 있도록 환경변수로 내보냅니다.

```bash
# Step 1: C4에 API 키 저장 (영구 저장)
c4 config api-key set anthropic --key sk-ant-xxx

# Step 2: 환경변수로 내보내기 (현재 세션)
eval $(c4 env)

# 현재 설정된 키 확인
c4 config api-key list
```

**영구 설정** (권장): `.bashrc` 또는 `.zshrc`에 추가

```bash
# Bash
echo 'eval $(c4 env 2>/dev/null)' >> ~/.bashrc
source ~/.bashrc

# Zsh
echo 'eval $(c4 env 2>/dev/null)' >> ~/.zshrc
source ~/.zshrc

# Fish
echo 'c4 env --format=fish | source' >> ~/.config/fish/config.fish
```

**여러 Provider 설정**:

```bash
# Anthropic (기본)
c4 config api-key set anthropic --key sk-ant-xxx

# OpenAI
c4 config api-key set openai --key sk-xxx

# 모든 키 환경변수로 내보내기
eval $(c4 env)

# 특정 provider만 내보내기
eval $(c4 env anthropic)
```

> **💡 왜 이 과정이 필요한가?**
>
> - `c4 config api-key set`은 `~/.c4/credentials.yaml`에 저장합니다
> - OpenCode는 `${ANTHROPIC_API_KEY}` 환경변수를 참조합니다
> - `c4 env`가 이 둘을 연결합니다

---

## 설정

### MCP 서버 설정

`~/.opencode/config.json` 또는 프로젝트의 `.opencode/config.json`:

```json
{
  "providers": {
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  },
  "agents": {
    "coder": {
      "model": "claude-sonnet-4-20250514",
      "maxTokens": 8000
    }
  },
  "mcpServers": {
    "cq": {
      "type": "stdio",
      "command": "uv",
      "args": ["--directory", "/path/to/cq", "run", "python", "-m", "c4.mcp_server"],
      "env": {
        "C4_PROJECT_ROOT": "${PWD}"
      }
    }
  }
}
```

### C4 설정

```bash
# OpenCode를 기본 플랫폼으로 설정
c4 config platform opencode --global

# 또는 프로젝트별
c4 config platform opencode
```

---

## 사용법

### 프로젝트 시작

```bash
cd /path/to/project
opencode
```

### C4 MCP 도구 사용

OpenCode 내에서 C4 MCP 도구를 직접 호출할 수 있습니다:

```
> c4_status
> c4_get_task worker-1
> c4_submit task_id=T-001 commit_sha=abc123 ...
```

### 슬래시 커맨드 (선택)

`.opencode/commands/` 디렉토리에 커맨드 파일이 있습니다:

- `/c4-status` - 프로젝트 상태 확인
- `/c4-plan` - 계획 수립
- `/c4-run` - 자동 실행
- `/c4-stop` - 실행 중지
- `/c4-checkpoint` - 체크포인트 리뷰
- `/c4-submit` - 태스크 제출
- `/c4-validate` - 검증 실행
- `/c4-add-task` - 태스크 추가
- `/c4-clear` - 상태 초기화

---

## Provider 설정

### Anthropic (권장)

```json
{
  "providers": {
    "anthropic": {
      "apiKey": "${ANTHROPIC_API_KEY}"
    }
  },
  "agents": {
    "coder": {
      "model": "claude-sonnet-4-20250514"
    }
  }
}
```

**비용**: Input $3/MTok, Output $15/MTok

### OpenAI

```json
{
  "providers": {
    "openai": {
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "agents": {
    "coder": {
      "model": "gpt-4o"
    }
  }
}
```

### Ollama (로컬, 무료)

```json
{
  "providers": {
    "ollama": {
      "baseUrl": "http://localhost:11434"
    }
  },
  "agents": {
    "coder": {
      "model": "ollama/llama3"
    }
  }
}
```

### Groq (빠름, 저렴)

```json
{
  "providers": {
    "groq": {
      "apiKey": "${GROQ_API_KEY}"
    }
  },
  "agents": {
    "coder": {
      "model": "groq/llama3-70b-8192"
    }
  }
}
```

---

## 문제 해결

### MCP 서버 연결 안 됨

1. C4 경로 확인:
   ```bash
   cat ~/.c4-install-path
   ```

2. 의존성 재설치:
   ```bash
   cd ~/.c4 && uv sync
   ```

3. MCP 서버 수동 테스트:
   ```bash
   uv --directory ~/.c4 run python -m c4.mcp_server
   ```

### API 키 오류

설정된 키 확인:
```bash
# C4 설정 확인
c4 config api-key list

# 환경 변수 확인
echo $ANTHROPIC_API_KEY

# 환경 변수가 비어있다면, C4에서 내보내기
eval $(c4 env)

# 내보낼 키 미리 확인
c4 env --quiet
```

> **참고**: `c4 config api-key set`으로 저장한 키는 자동으로 환경변수가 되지 않습니다.
> 반드시 `eval $(c4 env)`로 내보내거나, shell profile에 추가하세요.

### 도구 호출 실패

OpenCode에서 MCP 도구 목록 확인:
```
> /tools
```

---

## 팀 온보딩

### 새 팀원 설정 (5분)

1. **OpenCode 설치**
   ```bash
   go install github.com/opencode-ai/opencode@latest
   ```

2. **C4 설치**
   ```bash
   curl -fsSL https://raw.githubusercontent.com/PlayIdea-Lab/cq/main/install.sh | bash
   ```

3. **API 키 설정** (리더에게 받은 키)
   ```bash
   c4 config api-key set anthropic --key sk-ant-xxx
   ```

4. **환경변수 영구 설정**
   ```bash
   # Bash/Zsh
   echo 'eval $(c4 env 2>/dev/null)' >> ~/.bashrc
   source ~/.bashrc
   ```

5. **사용 시작**
   ```bash
   cd /path/to/project
   opencode
   ```

### 비용 관리

- 팀 공유 API 키 사용 시 비용 중앙 관리 가능
- Anthropic Console에서 사용량 모니터링
- 예산 알림 설정 권장

---

## 참고

- [OpenCode 공식 사이트](https://opencode.ai/)
- [OpenCode GitHub](https://github.com/opencode-ai/opencode)
- [Anthropic API 가격](https://www.anthropic.com/pricing)
- [C4 플랫폼 지원](플랫폼-지원.md)

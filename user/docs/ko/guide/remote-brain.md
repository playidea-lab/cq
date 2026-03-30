# Remote AI 워크스페이스

[mcp.pilab.kr](https://mcp.pilab.kr)을 통해 ChatGPT, Claude Desktop, Cursor 등 MCP 호환 AI를 CQ 지식 베이스에 연결하세요.

## Remote AI 워크스페이스가 필요한 이유

로컬에 설치된 CQ는 169개의 MCP 도구와 GPU 접근을 제공합니다. Remote AI 워크스페이스는 **지식 레이어**를 제공합니다 — 로컬 설치 없이 어떤 AI 도구에서도, 어떤 디바이스에서도 접근 가능합니다.

- Claude Code에서 기록한 지식이 ChatGPT에서 사용 가능
- GPU 서버의 실험 결과가 데스크톱에 동기화
- 어떤 AI든 공유 워크스페이스를 읽고 쓸 수 있음

## 2단계로 연결

### 1단계: mcp.pilab.kr에 로그인

[https://mcp.pilab.kr](https://mcp.pilab.kr)을 방문하여 GitHub로 로그인합니다. OAuth 토큰이 생성됩니다.

### 2단계: AI 도구에 MCP 서버 추가

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

이것으로 끝입니다. GitHub OAuth가 인증을 처리합니다 — URL은 모든 사람에게 동일하며, 접근은 토큰으로 제어됩니다.

## AI 도구별 설정

### Claude Desktop

macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
Windows: `%APPDATA%\Claude\claude_desktop_config.json` 편집:

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

Claude Desktop을 재시작하세요.

### ChatGPT (Custom GPT / Connector)

ChatGPT의 MCP 커넥터 설정에 추가:

```
URL: https://mcp.pilab.kr/mcp
Auth: OAuth 2.1 (GitHub)
```

### Cursor

프로젝트의 `.cursor/mcp.json` 또는 전역의 `~/.cursor/mcp.json`에 추가:

```json
{
  "mcpServers": {
    "cq-brain": {
      "url": "https://mcp.pilab.kr/mcp",
      "type": "streamable-http"
    }
  }
}
```

## Remote AI 워크스페이스의 기능

Remote MCP 서버는 지식에 집중된 CQ 도구의 하위 집합을 노출합니다:

| 도구 | 설명 |
|------|------|
| `cq_knowledge_record` | 발견, 실험 결과, 결정 저장 |
| `cq_knowledge_search` | 지식 베이스 검색 |
| `cq_session_summary` | 최근 세션 요약 가져오기 |
| `cq_preferences_list` | 쌓인 선호도 보기 |

이 도구들은 Claude Code, ChatGPT, Cursor에서 동일하게 작동합니다 — 같은 데이터, 같은 백엔드.

## AI 자동 캡처

Remote AI 워크스페이스는 AI 도구가 당신이 요청하지 않아도 **자발적으로** 지식을 저장하도록 설계되었습니다.

도구 설명이 자동 저장을 트리거하도록 작성되어 있습니다:

- AI가 버그 근본 원인을 발견하면 → `cq_knowledge_record`가 자동으로 실행됨
- 난해한 문제의 해결책을 찾으면 → 즉시 저장됨
- 여러 파일에 걸쳐 패턴이 드러나면 → 컨텍스트와 함께 기록됨

"이걸 저장해"라고 프롬프트할 필요 없습니다 — AI가 보존할 가치가 있다고 인식하면 스스로 저장합니다.

## 세션 요약

세션이 종료될 때, AI가 `cq_session_summary`를 호출하여 캡처합니다:

- 내린 주요 결정
- 표현된 선호도
- 해결한 문제와 방법
- 변경된 파일과 이유

이것이 [Knowledge Loop](growth-loop.md)에 직접 반영됩니다 — 요약에서 추출된 선호도가 hint와 rule로 쌓입니다.

## OAuth 플로우

```
AI 도구 → mcp.pilab.kr/mcp
                │
          OAuth 2.1 (GitHub)
                │
          토큰 검증
                │
          Supabase 네임스페이스로 라우팅
                │
          지식 결과 반환
```

Cloudflare Worker (`mcp.pilab.kr`)가 OAuth 2.1 프록시 역할을 합니다. GitHub 토큰을 검증하고, Supabase에서 사용자 네임스페이스를 식별하고, 지식 작업을 프록시합니다. 원격 머신에 CQ 바이너리가 필요 없습니다.

## 크로스 플랫폼 지식 동기화

어떤 도구에서든 지식을 기록하면 즉시 다른 모든 곳에서 사용 가능합니다:

```
Claude Code 세션:  캐싱 버그 근본 원인 발견
  → cq_knowledge_record("익명 세션에 Redis TTL 미설정")

같은 날, ChatGPT:  "캐시가 왜 일관성 없이 동작하죠?"
  → cq_knowledge_search("cache")가 이전 발견 반환
```

복사-붙여넣기 없음. 재설명 없음. 지식이 당신을 따라다닙니다.

## 요구사항

- CQ 계정 (무료 — [mcp.pilab.kr](https://mcp.pilab.kr)에서 가입)
- OAuth용 GitHub 계정
- MCP 호환 AI 도구

Remote AI 워크스페이스 연결에 로컬 CQ 설치가 필요하지 않습니다.

## 로컬 설치와의 관계

Remote AI 워크스페이스와 로컬 CQ는 **동일한 Supabase 백엔드**를 사용합니다. 별개의 시스템이 아닙니다:

| 기능 | 로컬 CQ | Remote AI 워크스페이스 |
|------|---------|----------------------|
| 태스크 오케스트레이션 | 있음 | 없음 |
| GPU 작업 실행 | 있음 | 없음 |
| 파일 접근 | 있음 | 없음 |
| 지식 읽기/쓰기 | 있음 | 있음 |
| Knowledge Loop | 있음 | 있음 (요약 경유) |
| 필요한 설정 | 설치 + 빌드 | OAuth 로그인 |

GPU 실험과 개발에는 로컬 CQ를 사용하세요. 소프트웨어를 설치할 수 없는 도구에서 지식 접근에는 Remote AI 워크스페이스를 사용하세요.

## 다음 단계

- [Knowledge Loop](growth-loop.md) — 지식이 선호도와 규칙으로 쌓이는 방법
- [티어](tiers.md) — Remote AI 워크스페이스는 Pro와 Team 티어에서 사용 가능

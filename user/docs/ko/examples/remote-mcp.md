# Remote MCP 연결하기

CQ를 로컬에 설치하지 않고도 ChatGPT, Claude Desktop, Cursor 등 MCP 호환 AI에서 CQ 두뇌에 접근하세요.

---

## 하게 될 것

`mcp.pilab.kr`을 통해 ChatGPT(또는 다른 MCP 클라이언트)를 CQ 지식 베이스에 연결합니다. 설정 후 어디서든 지식 검색, 스냅샷 저장, 프로젝트 상태 확인이 가능합니다.

---

## 1단계: 로그인

[mcp.pilab.kr](https://mcp.pilab.kr)에서 GitHub으로 로그인하세요.

```sh
cq auth login    # 아직 안 했다면
```

---

## 2단계: AI 도구에 MCP 서버 추가

### Claude Desktop / Claude Code

MCP 설정에 추가:

```json
{
  "cq-brain": {
    "url": "https://mcp.pilab.kr/mcp",
    "type": "streamable-http"
  }
}
```

### ChatGPT

ChatGPT 설정 → MCP Servers → Add:

```
URL: https://mcp.pilab.kr/mcp
```

OAuth 플로우를 따라 GitHub으로 로그인하세요.

### Cursor

`.cursor/mcp.json`에 추가:

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

---

## 3단계: 사용하기

연결되면 AI가 다음 도구에 접근할 수 있습니다:

| 도구 | 기능 |
|------|------|
| `cq_snapshot` | 대화 스냅샷을 지식 베이스에 저장 |
| `cq_recall` | 지식 베이스 검색 |
| `cq_status` | 프로젝트 상태 확인 |

### 예시: ChatGPT에서 지식 검색

```
"CQ 지식에서 WebSocket 재시도 패턴을 검색해줘"
```

ChatGPT가 `cq_recall` 호출 → 과거 결정과 코드 패턴을 반환합니다.

### 예시: Claude Desktop에서 아이디어 저장

```
"이 대화를 스냅샷으로 저장해 — 캐싱 전략을 정했으니까"
```

Claude가 `cq_snapshot` 호출 → Supabase에 저장되어 어떤 도구에서든 검색 가능.

---

## 동작 방식

```
AI 도구 → mcp.pilab.kr/mcp
                │
                ▼
         Cloudflare Worker (OAuth 프록시)
                │
                ▼
         Supabase Edge Function (MCP 서버)
                │
                ▼
         지식 베이스 (Supabase DB)
```

원격 머신에 CQ 바이너리 필요 없음. 두뇌는 클라우드에 있습니다.

---

## 다음 단계

- [ChatGPT → Claude 워크플로우](chatgpt-to-claude.md) — ChatGPT에서 아이디어 시작, Claude에서 구현
- [Growth Loop](growth-loop-in-action.md) — 세션이 쌓이면서 선호도가 진화하는 과정

# 원격 MCP 접근

어떤 머신의 AI 도구에서든 CQ MCP 서버를 HTTP로 호출할 수 있도록 노출합니다.

::: info connected / full 티어 권장
원격 MCP는 모든 티어에서 작동하지만, 원격 머신의 도구들이 동일한 클라우드 프로젝트 상태를 공유하려면 `connected` 또는 `full` 티어가 가장 유용합니다.
:::

## 동작 원리

```
원격 머신                          내 머신 (cq serve)
──────────────                     ───────────────────────
Claude Code                        cq mcp-http
.mcp.json type:"url"  ────────►   POST /mcp  (JSON-RPC 2.0)
                                   GET  /mcp  (SSE keepalive)
                                   포트 4142
```

`cq serve`는 MCP 와이어 프로토콜인 JSON-RPC 2.0을 사용하는 HTTP 서버를 시작합니다.
원격 클라이언트는 모든 요청 헤더에 정적 API 키를 포함해 인증합니다.

---

## 1단계 — config에서 mcp_http 활성화

`~/.c4/config.yaml` (전역) 또는 `.c4/config.yaml` (프로젝트)를 편집합니다:

```yaml
serve:
  mcp_http:
    enabled: true
    port: 4142           # 기본값
    bind: "0.0.0.0"      # 네트워크에 노출 (기본값: 127.0.0.1 = 로컬호스트만)
```

::: tip 로컬 전용 접근
같은 머신에서만 접근이 필요한 경우(예: 노트북에서 여러 AI 도구 사용) `bind`를 기본값 `127.0.0.1`로 유지하세요. 원격 머신이 연결해야 하는 경우에만 `bind: "0.0.0.0"`으로 설정하세요.
:::

---

## 2단계 — API 키 설정

```sh
cq secret set mcp_http.api_key <your-key>
```

또는 환경 변수로 설정할 수 있습니다 (CI/Docker에서 유용):

```sh
export CQ_MCP_API_KEY=<your-key>
```

::: warning 필수 설정
키가 설정되지 않으면 `cq serve`가 mcp_http 컴포넌트 시작을 거부합니다.
:::

---

## 3단계 — 서버 시작

```sh
cq serve
```

다음과 같이 표시되어야 합니다:

```
✓ mcp-http  0.0.0.0:4142
```

엔드포인트가 접근 가능한지 확인:

```sh
curl -s -X POST http://localhost:4142/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <your-key>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | jq '.result.tools | length'
```

---

## 4단계 — 원격 AI 도구 연결

### Claude Code

프로젝트의 `.mcp.json`(또는 전역 접근의 경우 `~/.claude/mcp.json`)에 항목을 추가합니다:

```json
{
  "mcpServers": {
    "cq-remote": {
      "type": "url",
      "url": "http://192.168.1.100:4142/mcp",
      "headers": {
        "X-API-Key": "<your-key>"
      }
    }
  }
}
```

`192.168.1.100`을 `cq serve`가 실행 중인 머신의 IP 또는 호스트명으로 바꿉니다.

새 서버를 인식하려면 Claude Code를 재시작하세요. 다음으로 확인:

```
/c4-status
```

### 다른 MCP 클라이언트

[MCP Streamable HTTP 전송](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/#streamable-http)을 지원하는 모든 클라이언트가 연결 가능합니다:

- **URL**: `http://<host>:4142/mcp`
- **인증 헤더**: `X-API-Key: <your-key>` 또는 `Authorization: Bearer <your-key>`
- JSON-RPC 호출에는 **POST**, SSE keepalive에는 **GET**

---

## 사용 사례

| 시나리오 | 설정 |
|---------|------|
| 같은 노트북의 두 AI 도구 | `bind: "127.0.0.1"` (기본값), 둘 다 `http://127.0.0.1:4142/mcp` 지정 |
| 같은 LAN의 팀원 | `bind: "0.0.0.0"`, LAN IP 공유 |
| 모바일 / 웹 클라이언트 | `bind: "0.0.0.0"`, TLS가 있는 리버스 프록시로 노출 |
| Docker 컨테이너 | `CQ_MCP_API_KEY` 환경 변수 전달, 포트 4142 매핑 |

---

## 리버스 프록시(TLS) 뒤에서 실행

프로덕션 또는 공개 접근의 경우 리버스 프록시에서 TLS를 종료하고 로컬호스트로 포워딩합니다:

```nginx
# nginx 예시
server {
    listen 443 ssl;
    server_name cq.example.com;

    location /mcp {
        proxy_pass http://127.0.0.1:4142/mcp;
        proxy_set_header X-API-Key $http_x_api_key;
        # SSE 지원
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }
}
```

클라이언트는 `https://cq.example.com/mcp`를 사용합니다.

---

## 도구 타임아웃 조정

오래 실행되는 도구(예: `hub_dispatch_job`)는 기본 60초보다 더 긴 타임아웃이 필요할 수 있습니다:

```yaml
serve:
  mcp_http:
    enabled: true
    port: 4142
    tool_timeout_sec: 300   # 5분
```

---

## 문제 해결

| 문제 | 해결 방법 |
|------|----------|
| 시작 시 `api_key is required` | `cq secret set mcp_http.api_key <key>` 실행 |
| 클라이언트에서 `401 unauthorized` | 헤더의 키가 정확히 일치하는지 확인 |
| 연결 거부 | `cq serve`가 실행 중이고 `bind`/`port`가 일치하는지 확인 |
| 120초 후 SSE 연결 끊김 | 프록시 유휴 타임아웃 — nginx에서 `proxy_read_timeout 3600s` 설정 |
| 도구 타임아웃 | config에서 `tool_timeout_sec` 증가 |

---

## 보안 참고 사항

- API 키는 타이밍 공격을 방지하기 위해 상수 시간 비교로 검증됩니다.
- 소스 컨트롤에 키를 커밋하지 마세요 — `cq secret set` 또는 `CQ_MCP_API_KEY`를 사용하세요.
- `bind: "0.0.0.0"` 사용 시 방화벽으로 네트워크 접근을 제한하세요.
- 신뢰할 수 없는 로컬 네트워크 외부 트래픽에는 TLS(리버스 프록시)를 사용하세요.

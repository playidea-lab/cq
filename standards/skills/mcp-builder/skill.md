# MCP Builder

MCP (Model Context Protocol) 서버 개발 가이드.

## 트리거

"MCP 서버", "mcp builder", "MCP 개발", "도구 서버", "tool server"

## Steps

### 1. 도구 설계

MCP 서버 = LLM이 호출하는 도구 모음.

도구 설계 원칙:
- 이름: 동사_명사 형태 (`search_users`, `create_ticket`)
- 설명: LLM이 언제 이 도구를 써야 하는지 명확히
- 파라미터: 최소한, 필수/선택 구분, 타입 명시
- 반환: 구조화된 텍스트 (LLM이 파싱할 수 있도록)

### 2. Python (FastMCP)

```python
from fastmcp import FastMCP

mcp = FastMCP("my-server")

@mcp.tool()
def search_users(query: str, limit: int = 10) -> str:
    """사용자를 검색합니다.

    Args:
        query: 검색어 (이름, 이메일)
        limit: 최대 결과 수 (기본 10)
    """
    results = db.search(query, limit=limit)
    return json.dumps([u.dict() for u in results])
```

### 3. Go (JSON-RPC stdio)

```go
// Go는 공식 SDK가 없으므로 JSON-RPC over stdio를 직접 구현.
// mcp-go 커뮤니티 라이브러리 또는 직접 구현.
func main() {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        var req jsonrpc.Request
        json.Unmarshal(scanner.Bytes(), &req)
        switch req.Method {
        case "tools/call":
            result := handleToolCall(req.Params)
            writeResponse(os.Stdout, req.ID, result)
        case "tools/list":
            writeResponse(os.Stdout, req.ID, toolsList)
        }
    }
}
```

### 4. TypeScript (MCP SDK)

```typescript
import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';

const server = new Server(
  { name: 'my-server', version: '1.0.0' },
  { capabilities: { tools: {} } }
);

server.setRequestHandler(CallToolRequestSchema, async (req) => {
  switch (req.params.name) {
    case 'search_users':
      const results = await searchUsers(req.params.arguments);
      return { content: [{ type: 'text', text: JSON.stringify(results) }] };
  }
});

await server.connect(new StdioServerTransport());
```

### 4. .mcp.json 등록

```json
{
  "mcpServers": {
    "my-server": {
      "command": "python",
      "args": ["-m", "my_server"],
      "env": { "API_KEY": "${API_KEY}" }
    }
  }
}
```

### 5. 테스트

- 각 도구의 입출력 검증 (단위 테스트)
- 에러 케이스: 잘못된 파라미터, 외부 서비스 장애
- LLM 통합 테스트: 실제 프롬프트에서 도구가 올바르게 호출되는지

### 6. 보안

- 사용자 입력을 외부 명령에 직접 전달 금지
- API 키는 환경변수로 (코드에 하드코딩 금지)
- 파일 경로: path traversal 방지
- Rate limiting: 외부 API 호출 시

## 도구 설계 체크리스트

- [ ] 도구 이름이 동작을 명확히 설명하는가?
- [ ] 설명이 LLM이 언제 써야 하는지 알려주는가?
- [ ] 파라미터가 최소한인가?
- [ ] 에러 시 유용한 에러 메시지를 반환하는가?
- [ ] 반환값이 LLM이 이해할 수 있는 형태인가?

## 안티패턴

- 하나의 도구에 너무 많은 기능 (분할)
- 설명 없는 도구 (LLM이 언제 쓸지 모름)
- stdout에 디버깅 출력 (MCP 프로토콜과 충돌)
- 동기 blocking I/O (서버 응답 지연)

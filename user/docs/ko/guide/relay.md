# Relay MCP 서버 가이드

NAT 뒤에 있는 Worker에도 클라우드 WebSocket 브릿지를 통해 원격 MCP 접근이 가능합니다.

## Relay란?

`cq serve`를 실행하면 Worker가 릴레이 서버에 WebSocket 연결을 엽니다. 원격 MCP 클라이언트는 HTTPS로 연결하고, 릴레이가 둘을 중계합니다. Worker 측에서 인바운드 포트나 방화벽 변경이 필요 없습니다.

```
Claude Code / Client
        │
        │  HTTPS (MCP over HTTP)
        ▼
  cq-relay.fly.dev
        │
        │  WSS (WebSocket)
        ▼
  Worker (cq serve, NAT 뒤)
```

---

## 설정

### 1. 인증

```sh
cq auth login
```

### 2. cq serve 시작

```sh
cq serve
```

```
[relay] connected  worker_id=wkr_abc123
[relay] MCP endpoint ready  https://cq-relay.fly.dev/w/wkr_abc123/mcp
```

### 3. 상태 확인

```sh
cq relay status
```

---

## 외부 MCP 클라이언트 연결

### Claude Code `.mcp.json`

```json
{
  "mcpServers": {
    "my-worker": {
      "type": "http",
      "url": "https://cq-relay.fly.dev/w/wkr_abc123/mcp",
      "headers": {
        "Authorization": "Bearer <jwt-token>"
      }
    }
  }
}
```

JWT 토큰 확인: `cq auth token`

---

## 대용량 파일 전송: cq transfer

대용량 데이터(데이터셋, 모델 체크포인트)는 P2P 전송 사용:

```sh
# Worker에서 — 파일 공유
cq transfer send ./model.ckpt

# Client에서 — 수신
cq transfer recv <transfer-id>
```

WebRTC P2P 기반 — 릴레이를 거치지 않고 직접 전송.

---

## 보안

- 모든 릴레이 요청에 Bearer JWT 필요
- 릴레이 서버가 Supabase Auth API로 매 요청 검증
- Worker 격리: 각 Worker ID는 소유자 계정에 바인딩

---

## 문제 해결

| 증상 | 해결 |
|------|------|
| 릴레이 연결 안 됨 | `cq relay status` 확인 → `cq auth login` → `cq serve` 재시작 |
| 401 Unauthorized | `cq auth login`으로 세션 갱신 |
| workers: 0 | `cq serve` 실행 중인지 확인 |
| 지연 높음 | 대용량은 `cq transfer` 사용 |

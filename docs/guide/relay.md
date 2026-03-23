# Relay MCP Server Guide

Relay enables remote MCP access to any worker — even behind NAT — via a cloud WebSocket bridge.

## What is Relay?

A worker running `cq serve` opens a persistent WebSocket connection to the relay server. Remote MCP clients (Claude Code, other agents) connect through HTTPS. The relay bridges the two without requiring inbound ports or firewall changes on the worker.

```
Claude Code / Client
        │
        │  HTTPS  (MCP over HTTP)
        ▼
  cq-relay.fly.dev
  relay server (cloud)
        │
        │  WSS  (persistent WebSocket)
        ▼
  Worker machine
  (cq serve, behind NAT)
```

**Key properties:**

- No inbound ports needed on the worker — outbound WSS only.
- JWT-authenticated: every request verified against Supabase Auth API.
- MCP protocol is proxied transparently — all 118+ cq MCP tools are accessible remotely.

---

## Setup

### 1. Authenticate

```sh
cq auth login
```

This stores a session JWT at `~/.c4/session.json`. Relay reads this automatically — no extra configuration needed.

### 2. Start cq serve

```sh
cq serve
```

On startup, `cq serve` connects to the relay server and registers the worker. The worker ID is printed in the output:

```
[relay] connected  worker_id=wkr_abc123  url=wss://cq-relay.fly.dev/ws
[relay] MCP endpoint ready  https://cq-relay.fly.dev/w/wkr_abc123/mcp
```

### 3. Check relay status

```sh
cq relay status
```

Output:

```
Relay:    connected
Worker:   wkr_abc123
MCP URL:  https://cq-relay.fly.dev/w/wkr_abc123/mcp
Latency:  12ms
```

---

## Connecting External MCP Clients

Any MCP-compatible client can connect to the worker through relay.

### MCP URL format

```
https://cq-relay.fly.dev/w/{worker_id}/mcp
```

Replace `{worker_id}` with the value from `cq relay status`.

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

Get the JWT token:

```sh
cq auth token          # Print current JWT (copy to Authorization header)
```

### Other MCP clients

Any client that supports MCP Streamable HTTP (JSON-RPC 2.0 over HTTP) works. Set:

- **URL**: `https://cq-relay.fly.dev/w/{worker_id}/mcp`
- **Auth**: `Authorization: Bearer <jwt>`
- **Method**: POST with `Content-Type: application/json`

---

## Large File Transfer: cq transfer

For large payloads (datasets, model checkpoints), use peer-to-peer transfer instead of relay.

```sh
# On worker — share a file
cq transfer send ./model.ckpt
# Outputs: cq transfer recv <transfer-id>

# On client — receive
cq transfer recv <transfer-id>
```

`cq transfer` uses WebRTC-based P2P: the relay server brokers the initial handshake, then data flows directly between the two machines (no relay bottleneck for large transfers).

---

## Security

### Authentication

Every relay request carries a Bearer JWT. The relay server validates it against the Supabase Auth API on each request. Expired or invalid tokens return `401 Unauthorized`.

```
Client → relay: Authorization: Bearer <jwt>
relay → Supabase Auth API: verify JWT
Supabase: OK / invalid
relay: forward to worker / reject
```

### What's protected

- **Worker-to-relay** connection: authenticated with the worker's session JWT.
- **Client-to-relay** requests: verified JWT required.
- **Worker isolation**: each worker ID is bound to its owner's user account. A user cannot access another user's worker via relay.

---

## Troubleshooting

### Relay not connecting

```sh
cq relay status       # Check connection state
cq auth login         # Re-authenticate if session expired
cq serve              # Restart — relay reconnects automatically on startup
```

Check that the worker can reach the relay server:

```sh
curl -I https://cq-relay.fly.dev/health
```

### 401 Unauthorized

The JWT is expired or invalid.

```sh
cq auth login         # Refresh session
cq auth token         # Verify token is present
```

If using `.mcp.json`, update the `Authorization` header with the fresh token from `cq auth token`.

### `workers: 0` in relay status

The worker is not registered. Possible causes:

1. `cq serve` is not running — start it.
2. Auth failed on startup — check `cq auth login`.
3. Network issue — check outbound WSS (port 443) to `cq-relay.fly.dev`.

```sh
# Check serve logs for relay errors
cq serve 2>&1 | grep -i relay
```

### High latency through relay

Relay adds ~10–50ms round-trip overhead (relay server location dependent). For latency-sensitive workloads:

- Use `cq transfer` for large data.
- Consider running cq serve on a machine with direct network access.
- Self-host relay (contact support for `cq-relay` source).

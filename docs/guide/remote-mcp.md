# Remote MCP Access

Access CQ tools from any machine — even behind NAT.

## Recommended: Relay (v1.27+)

The easiest way to access CQ remotely is the **Relay** — a WSS bridge that traverses NAT automatically.

```
Your machine (NAT)              Relay (Fly.io)               Remote client
──────────────────              ──────────────               ─────────────
cq serve ──WSS──►  cq-relay.fly.dev  ◄──HTTPS──  Claude Code
                   /w/{worker}/mcp                .mcp.json type:"http"
```

### Setup

```sh
cq auth login    # auto-configures relay
cq               # starts service (relay auto-connects)
cq relay status  # verify
```

### Connect from another machine

Add to `.mcp.json`:

```json
{
  "mcpServers": {
    "my-worker": {
      "type": "http",
      "url": "https://cq-relay.fly.dev/w/<hostname>/mcp",
      "headers": {
        "Authorization": "Bearer <jwt-from-cq-auth-status>"
      }
    }
  }
}
```

JWT expires in 1 hour — the relay client auto-refreshes, but the `.mcp.json` header is static. For long-term access, use `cq serve` on the remote machine directly.

---

## Legacy: Direct HTTP (LAN only)

::: warning Legacy Approach
The HTTP method below requires direct network access (same LAN or VPN). For NAT traversal, use Relay above.
:::

Expose CQ's MCP server over HTTP so any AI tool on the same network can call CQ tools.

```
Remote machine                  Your machine (cq serve)
──────────────                  ───────────────────────
Claude Code                     cq mcp-http
.mcp.json type:"http" ────────► POST /mcp  (JSON-RPC 2.0)
                                GET  /mcp  (SSE keepalive)
                                port 4142
```

`cq serve` starts an HTTP server that speaks JSON-RPC 2.0 (the MCP wire protocol).
Remote clients authenticate with a static API key in every request header.

---

## Step 1 — Enable mcp_http in config

Edit `~/.c4/config.yaml` (global) or `.c4/config.yaml` (project):

```yaml
serve:
  mcp_http:
    enabled: true
    port: 4142           # default
    bind: "0.0.0.0"      # expose to network (default: 127.0.0.1 = localhost only)
```

::: tip Local-only access
Leave `bind` at the default `127.0.0.1` if you only need access from the same machine (e.g. multiple AI tools on one laptop). Set `bind: "0.0.0.0"` only when remote machines need to connect.
:::

---

## Step 2 — Set an API key

```sh
cq secret set mcp_http.api_key <your-key>
```

Or set it via environment variable (useful in CI/Docker):

```sh
export CQ_MCP_API_KEY=<your-key>
```

::: warning Required
`cq serve` will refuse to start the mcp_http component if no key is configured.
:::

---

## Step 3 — Start the server

```sh
cq serve
```

You should see:

```
✓ mcp-http  0.0.0.0:4142
```

To verify the endpoint is reachable:

```sh
curl -s -X POST http://localhost:4142/mcp \
  -H "Content-Type: application/json" \
  -H "X-API-Key: <your-key>" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' \
  | jq '.result.tools | length'
```

---

## Step 4 — Connect a remote AI tool

### Claude Code

Add an entry to `.mcp.json` in your project (or `~/.claude/mcp.json` for global access):

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

Replace `192.168.1.100` with the IP or hostname of the machine running `cq serve`.

Restart Claude Code to pick up the new server. Verify with:

```
/c4-status
```

### Other MCP clients

Any client that supports the [MCP Streamable HTTP transport](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/#streamable-http) can connect:

- **URL**: `http://<host>:4142/mcp`
- **Auth header**: `X-API-Key: <your-key>` or `Authorization: Bearer <your-key>`
- **POST** for JSON-RPC calls, **GET** for SSE keepalive

---

## Use cases

| Scenario | Setup |
|----------|-------|
| Two AI tools on the same laptop | `bind: "127.0.0.1"` (default), both point to `http://127.0.0.1:4142/mcp` |
| Teammate on the same LAN | `bind: "0.0.0.0"`, share the LAN IP |
| Mobile / web client | `bind: "0.0.0.0"`, expose via reverse proxy with TLS |
| Docker container | Pass `CQ_MCP_API_KEY` env, map port 4142 |

---

## Run behind a reverse proxy (TLS)

For production or public access, terminate TLS at a reverse proxy and forward to localhost:

```nginx
# nginx example
server {
    listen 443 ssl;
    server_name cq.example.com;

    location /mcp {
        proxy_pass http://127.0.0.1:4142/mcp;
        proxy_set_header X-API-Key $http_x_api_key;
        # SSE support
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }
}
```

Clients then use `https://cq.example.com/mcp`.

---

## Adjusting the tool timeout

Long-running tools (e.g. `hub_dispatch_job`) may need more than the default 60-second timeout:

```yaml
serve:
  mcp_http:
    enabled: true
    port: 4142
    tool_timeout_sec: 300   # 5 minutes
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `api_key is required` on startup | Run `cq secret set mcp_http.api_key <key>` |
| `401 unauthorized` from client | Check that the key in the header matches exactly |
| Connection refused | Confirm `cq serve` is running and `bind`/`port` match |
| SSE disconnects after 120 s | Proxy idle timeout — set `proxy_read_timeout 3600s` in nginx |
| Tool times out | Increase `tool_timeout_sec` in config |

---

## Security notes

- The API key is compared with constant-time comparison to prevent timing attacks.
- Never commit the key to source control — use `cq secret set` or `CQ_MCP_API_KEY`.
- Restrict network access with a firewall when using `bind: "0.0.0.0"`.
- Use TLS (reverse proxy) for any traffic outside a trusted local network.

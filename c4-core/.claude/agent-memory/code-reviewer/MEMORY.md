# Code Reviewer Agent Memory

## Project: c4-core (Go MCP Server)

### Architecture Overview (2026-02-08)
- **Primary**: `cmd/c4/mcp.go` + `internal/mcp/handlers/` = Go MCP server (stdio)
- **Bridge**: `internal/bridge/sidecar.go` manages Python sidecar (JSON-RPC/TCP)
- **Proxy**: `internal/mcp/handlers/proxy.go` forwards LSP/Knowledge/GPU to sidecar
- **Store**: `internal/mcp/handlers/sqlite_store.go` reads `.c4/c4.db` or `.c4/tasks.db`

### Key Finding: Dead Code Packages
- `internal/server/` (669 LOC) - Cloud HTTP server, Fly.io integration - NEVER imported
- `internal/auth/` (~600 LOC) - OAuth/PKCE/Session management - NEVER imported
- `internal/realtime/` (382 LOC) - Supabase WebSocket subscriptions - NEVER imported
- `internal/bridge/grpc_client.go` (569 LOC) - behind `//go:build grpc` tag (intentional gating)
- `internal/worker/worker.go` - just a package doc comment, empty
- `internal/git/git.go` - just a package doc comment, empty
- `internal/state/` - has tests but no imports from cmd/ or handlers/
- `internal/task/` - has full TaskStore impl + SupabaseStore, but not used by MCP handlers
- `internal/config/` - has full config loading via viper, only used by validation/runner.go

### Key Finding: Tool Count Mismatch
- register.go comment says "Total: 50 tools" but actual count is **47** unique tools
- 47 matches the .mcp.json expectation but comment is wrong

### Key Finding: Proxy Default Address Hardcoded
- `proxy.go:22` defaults to `localhost:50051` when addr is empty
- But sidecar uses dynamic port (port 0 = auto-assign)
- This means if sidecar fails, proxy will try wrong address

### Key Finding: Lighthouse Feature Issues (2026-02-14)
- `lighthousePromote` schema validation is dead code: Unregister(name) before GetToolSchema(name)
- T-LH- task ID parsing uses LastIndex("-") -- fragile if name contains hyphens, no name validation at registration
- `lighthouseRegister` raw SQL `UPDATE c4_lighthouses SET task_id=?` bypasses store abstraction, swallows errors
- Promote task completion via raw SQL skips persona stats/auto-learn
- `validateSchemaCompat` only checks property names + required, not types
- Lighthouse operations not atomic between Registry + SQLite (race possible in theory)
- No re-register after deprecate (getLighthouse finds deprecated record, blocks re-register)

### Patterns Observed
- All handler files follow consistent pattern: Register*Handlers + handle* functions
- No hardcoded user paths in Go code (good)
- Build tags properly used for gRPC (grpc_client.go)
- Error handling generally good but some `_ = err` silencing in sqlite_store.go
- Best-effort migrations with `_, _ = s.db.Exec(m)` swallow all errors including non-duplicate-column failures

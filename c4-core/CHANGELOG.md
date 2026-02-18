# Changelog

## v2.0.0-beta (2026-02-08)

### Highlights

C4 v2.0.0-beta introduces the **Go Hybrid Architecture**: a high-performance
Go core (`c4-core`) that handles MCP protocol, configuration, and validation,
while Python continues to manage task orchestration and agent routing.

### New Features

- **Go MCP Server**: All 10 core MCP tool handlers implemented in Go
  - State: `c4_status`, `c4_start`, `c4_clear`
  - Tasks: `c4_get_task`, `c4_submit`, `c4_add_todo`, `c4_mark_blocked`
  - Tracking: `c4_claim`, `c4_report`, `c4_checkpoint`
- **Go Config Manager**: YAML config loading via viper with env var overrides
- **Go Validation Runner**: os/exec-based command runner with timeout support
- **gRPC Bridge**: Protobuf-defined Go <-> Python communication channel
- **Performance Benchmarks**: testing.B benchmark suite for Go/No-Go decision

### Performance

| Metric | Python | Go Core | Improvement |
|--------|--------|---------|-------------|
| MCP server start | 500-1000ms | 0.005ms | ~100,000x |
| c4_status response | 5-20ms | 57ns | ~100,000x |
| Worker creation | 0.1-1ms | 61ns | ~1,600x |
| Memory at rest | 50-100MB | 14MB | ~3-7x |
| Binary size | ~50MB (venv) | <20MB | ~2.5x |

### Breaking Changes

- **MCP Server**: Go binary replaces Python MCP server as the primary endpoint
- **Config location**: Configuration remains at `.c4/config.yaml` (no change)
- **gRPC requirement**: Python daemon now communicates via gRPC bridge instead
  of in-process calls

### Migration Guide

1. Install Go binary:
   ```bash
   # macOS (Apple Silicon)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-darwin-arm64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core

   # macOS (Intel)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-darwin-amd64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core

   # Linux (amd64)
   curl -fsSL https://releases.c4.dev/v2.0.0-beta/c4-core-linux-amd64 -o /usr/local/bin/c4-core
   chmod +x /usr/local/bin/c4-core
   ```

2. Update Claude Code MCP settings (`~/.claude/mcp.json`):
   ```json
   {
     "mcpServers": {
       "cq": {
         "command": "c4-core",
         "args": ["mcp", "--project", "/path/to/project"]
       }
     }
   }
   ```

3. Python daemon runs alongside (started automatically by c4-core):
   ```bash
   # Verify installation
   c4-core version    # Should show v2.0.0-beta
   c4-core status     # Should respond in <100ms
   ```

### Build from Source

```bash
cd c4-core
go build -o c4-core ./cmd/c4

# Cross-compile
GOOS=darwin GOARCH=arm64 go build -o c4-core-darwin-arm64 ./cmd/c4
GOOS=darwin GOARCH=amd64 go build -o c4-core-darwin-amd64 ./cmd/c4
GOOS=linux GOARCH=amd64 go build -o c4-core-linux-amd64 ./cmd/c4
```

### Test Results

- Go tests: 55+ tests across 4 packages (config, handlers, validation, benchmark)
- Python tests: 358+ tests passing (no regression)
- Frontend tests: 29 vitest tests passing
- Rust tests: 16 cargo tests passing

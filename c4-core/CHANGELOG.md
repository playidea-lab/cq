# Changelog

## [v0.77.0] - 2026-03-08

### ✨ Features
- **notify**: `internal/notify` 패키지 — NotificationProfile + per-channel Sender (Dooray/Discord/Slack/Teams)
- **notifyhandler**: `c4_notification_set` / `c4_notification_get` / `c4_notify` MCP 도구
- **skills**: c4-plan/c4-run/c4-finish 마일스톤에 `c4_notify` 자동 호출

### 🐛 Bug Fixes
- **hypothesis**: status 필드 통일 — poll()과 MCP 핸들러 생성 경로 간 `status`/`hypothesis_status` 불일치 수정
- **hypothesis**: cleanup()에서 body 덮어쓰기 버그 수정 (Update body=nil)
- **hypothesis**: 마크다운 펜스 제거 패턴 수정 (```JSON 대소문자 처리)
- **notifyhandler**: internal/notify 위임, 파일 권한 octal 수정

### 🧪 Tests
- **hypothesis**: CrossComponent 테스트 추가 (status 필드 통일 검증)
- **hypothesis**: CleanupPreservesBody 테스트 추가 (body 보존 검증)

---

## [v0.43.0] - 2026-03-01

### ✨ Features
- **gemini**: Gemini 3.0 에이전트 업그레이드 + C1 "Hands" 브리지 (WebSocket 기반 네이티브 셸 실행)
- **enforce**: 3-layer deprecated 스킬 강제 시스템
  - Layer 1: c4-gate.sh Hook Gate (c4-polish/c4-refine → c4-finish 차단)
  - Layer 2: Arch Test (TestDeprecatedSkillsAreStubs / TestFinishSkillsHaveKnowledgeGate / TestPlanSkillsHaveKnowledgeRead)
  - Layer 3: Skill Linter (`scripts/lint-skills.sh`, `make lint-skills`)
- **c3/eventbus**: Dooray 양방향 응답 — `c4_dooray_respond` MCP 도구 + `dooray_respond_llm` action type + LLM caller
- **skills**: plan-run-finish 3단계 워크플로우 + C9 지식 게이트 패턴 (finish=기록, plan=조회)

### 🐛 Bug Fixes
- **reboot**: `.reboot` 파일에 UUID 기록하여 올바른 세션 복구 보장
- **archtest**: C3 EventBus dooray_respond 추가에 따른 ratchet allowlist 업데이트

### ♻️ Refactoring
- **skills**: c4-polish(336→17줄), c4-refine(339→17줄) deprecated stub으로 축약

### 🔧 Chores
- **dooray**: WebhookGateway Stop() mutex 해제 후 Shutdown 패턴 + HTTPS 강제 + 보안 모델 문서화
- **bats**: c4gate hooktest 케이스 14→19개 (deprecated 차단 4 + c4-finish 긍정 케이스 1 추가)

---

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

# C4 — AI Project Orchestration System

계획부터 완료까지 자동화된 프로젝트 관리. Go MCP Server + Python Sidecar + Tauri Desktop App.

## Architecture

```
Claude Code ──stdio──▶ Go MCP Server (47 tools)
                         ├─▶ Go native (21): state, tasks, files, git, validation
                         ├─▶ Go + SQLite (13): spec, design, checkpoint
                         └─▶ JSON-RPC proxy (13) ──TCP──▶ Python Sidecar
                                                           ├─▶ LSP (Multilspy → Jedi → Tree-sitter)
                                                           ├─▶ Knowledge Store v2 (FTS5 + Vector)
                                                           └─▶ GPU Scheduler
```

## Components

| Component | Directory | Tech | LOC |
|-----------|-----------|------|-----|
| **Go MCP Server** | `c4-core/` | Go 1.22, SQLite, cobra | ~10.5K |
| **Python Sidecar** | `c4/` | Python 3.12, multilspy, sqlite-vec | ~10.5K |
| **C1 Desktop App** | `c1/` | Tauri 2.x, React 18, Rust | ~5.6K |
| **Tests** | `tests/` | pytest, Vitest, cargo test | ~5.6K |
| **Total** | | | **~32K** |

## Quick Start

```bash
# 1. Clone
git clone https://git.pilab.co.kr/pi/c4.git && cd c4

# 2. Build Go MCP server
cd c4-core && go build -o bin/c4 ./cmd/c4 && cd ..

# 3. Install Python sidecar deps
uv sync

# 4. Claude Code에서 사용
#    .mcp.json이 이미 설정되어 있으므로 바로 사용 가능
/c4-status
```

## Workflow

```
INIT → DISCOVERY → DESIGN → PLAN → EXECUTE ⇄ CHECKPOINT → COMPLETE
```

```bash
/c4-plan "기능 설명"    # 계획 수립 + 태스크 생성
/c4-run                 # Worker 스폰 → 자동 실행
/c4-status              # 진행 상황 확인
/c4-checkpoint          # 코드 리뷰 요청
```

### Dual Execution Mode

| 모드 | 언제 | 도구 |
|------|------|------|
| **Worker** | 독립적, 병렬 가능한 태스크 | `c4_get_task` → `c4_submit` |
| **Direct** | 파일 간 의존성 높은 작업 | `c4_claim` → `c4_report` |

### Task Lifecycle

```
T-001-0 (구현) → R-001-0 (리뷰)
                    ↓
          ├─ APPROVE → 완료
          └─ REQUEST_CHANGES → T-001-1 → R-001-1 → ...
```

## Key Features

- **47 MCP Tools**: 상태, 태스크, 코드 분석, 파일, Git, 검증, 지식, GPU
- **Knowledge Store v2**: Obsidian Markdown SSOT + FTS5 + Vector hybrid search (RRF)
- **Code Intelligence**: Multilspy → Jedi → Tree-sitter 3단계 LSP fallback
- **GPU/ML Native**: GPU 감지, 스케줄링, DAG → Task 변환
- **Validation Runner**: lint, unit test 자동 실행
- **Checkpoint System**: APPROVE, REQUEST_CHANGES, REPLAN, REDESIGN
- **Team Collaboration**: Supabase 기반 팀 상태 공유

## C1 Desktop App

Multi-LLM 프로젝트 탐색기. Claude Code, Codex CLI, Cursor, Gemini CLI 4개 프로바이더 통합.

```bash
cd c1 && pnpm install && pnpm tauri dev
```

4개 뷰: Sessions (+Analytics), Dashboard (+Timeline +Validation), Config, Team (Cloud).

자세한 내용은 [c1/README.md](c1/README.md) 참조.

## Configuration

### MCP Server (.mcp.json)

```json
{
  "mcpServers": {
    "c4": {
      "command": "/path/to/c4/c4-core/bin/c4",
      "args": ["mcp"],
      "env": { "C4_PROJECT_DIR": "." }
    }
  }
}
```

### Project Config (.c4/config.yaml)

```yaml
project_id: my-project
default_branch: main

validation:
  commands:
    lint: "uv run ruff check"
    unit: "uv run pytest tests/unit"
  required: ["lint", "unit"]

review_as_task: true
checkpoint_as_task: true
max_revision: 3
```

### Environment Variables (Optional)

Cloud/Team 기능을 위해 프로젝트 루트 `.env`에 설정:

```
SUPABASE_URL=https://xxxx.supabase.co
SUPABASE_KEY=eyJ...
```

## Slash Commands

| 명령어 | 설명 |
|--------|------|
| `/c4-init` | 프로젝트 초기화 |
| `/c4-status` | 상태 확인 |
| `/c4-plan` | 계획 수립 + 태스크 생성 |
| `/c4-run` | 자동 실행 (Smart Auto Mode) |
| `/c4-stop` | 실행 중지 |
| `/c4-validate` | 검증 실행 |
| `/c4-submit` | 태스크 제출 |
| `/c4-checkpoint` | 리뷰 요청 |
| `/c4-add-task` | 태스크 추가 |

## Development

```bash
# Go MCP server
cd c4-core && go build ./... && go test ./...

# Python sidecar
uv run pytest tests/

# C1 frontend + backend
cd c1 && pnpm test
cd c1/src-tauri && cargo test
```

## Documentation

- [설치 가이드](docs/getting-started/설치-가이드.md)
- [빠른 시작](docs/getting-started/빠른-시작.md)
- [아키텍처](docs/developer-guide/아키텍처.md)
- [워크플로우 개요](docs/user-guide/워크플로우-개요.md)
- [Roadmap](docs/ROADMAP.md)

## License

MIT

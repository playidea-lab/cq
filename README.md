# C4 — AI Project Orchestration System

계획부터 완료까지 자동화된 프로젝트 관리. Go MCP Server + Python Sidecar + Tauri Desktop App.

## Architecture

```
Claude Code ──stdio──▶ Go MCP Server (56 tools)
                         ├─▶ Go native (21): state, tasks, files, git, validation
                         ├─▶ Go + SQLite (13): spec, design, checkpoint
                         └─▶ JSON-RPC proxy (16) ──TCP──▶ Python Sidecar
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

**Prerequisites:** Go 1.22+, Python 3.11+, [uv](https://docs.astral.sh/uv/), git

```bash
# 1. Clone
git clone https://git.pilab.co.kr/pi/c4.git && cd c4

# 2. Install (Go 빌드 + Python deps + .mcp.json 자동 설정)
./install.sh

# 3. Claude Code 재시작 후 사용
/c4-status
```

`install.sh`가 자동으로 수행하는 작업:
- Go 바이너리 빌드 (`c4-core/bin/c4`)
- Python 의존성 설치 (`uv sync`)
- `.mcp.json` 설정 (기존 서버 보존)
- `.c4/` 디렉토리 초기화

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

- **56 MCP Tools**: 상태, 태스크, 코드 분석, 파일, Git, 검증, 지식, GPU
- **Knowledge Store v2**: Obsidian Markdown SSOT + FTS5 + Vector hybrid search (RRF)
- **Code Intelligence**: Multilspy → Jedi → Tree-sitter 3단계 LSP fallback
- **GPU/ML Native**: GPU 감지, 스케줄링, DAG → Task 변환
- **Validation Runner**: lint, unit test 자동 실행
- **Checkpoint System**: APPROVE, REQUEST_CHANGES, REPLAN, REDESIGN
- **Team Collaboration**: Supabase 기반 팀 상태 공유

## What's Included

### MCP Tools (56개)

| 카테고리 | 수 | 예시 |
|----------|-----|------|
| Core (상태/태스크) | 11 | `c4_status`, `c4_start`, `c4_add_todo`, `c4_claim`, `c4_report`, `c4_request_changes` |
| Native (파일/Git) | 11 | `c4_find_file`, `c4_search_for_pattern`, `c4_read_file` |
| SQLite (스펙/디자인) | 12 | `c4_save_spec`, `c4_save_design`, `c4_checkpoint` |
| Proxy → Sidecar | 16 | `c4_find_symbol`, `c4_knowledge_search`, `c4_onboard` |
| Soul/Persona/Team | 6 | `c4_soul_get`, `c4_soul_set`, `c4_soul_resolve`, `c4_persona_stats`, `c4_persona_evolve`, `c4_whoami` |

### Slash Commands (14개)

`/c4-plan`, `/c4-run`, `/c4-status`, `/c4-submit`, `/c4-validate`,
`/c4-checkpoint`, `/c4-stop`, `/c4-clear`, `/c4-add-task`, `/c4-init`,
`/c4-interview`, `/c4-quick`, `/c4-swarm`, `/c4-release`

### Agents (37개)

`c4-scout` (경량 탐색) + 36개 도메인 전문가:
`code-reviewer`, `backend-architect`, `python-pro`, `golang-pro`,
`ml-engineer`, `data-scientist`, `security-auditor`, `debugger` 등

### Hooks (7개)

| 시점 | Hook | 기능 |
|------|------|------|
| PreToolUse | `security-check-before-commit` | 시크릿 스캔 |
| PreToolUse | `prevent-force-push-main` | main force push 방지 |
| PostToolUse | `auto-lint-python` | Ruff 자동 포맷 |
| PostToolUse | `auto-lint-typescript` | ESLint+Prettier |
| PostToolUse | `type-check-typescript` | tsc 타입 체크 |
| PostToolUse | `warn-console-log` | console.log 경고 |
| Stop | `final-cleanup-check` | TODO 마커 잔존 체크 |

### Personas (27개)

`.c4/personas/`에 자동 생성. 워크플로우 가중치 기반 역할 분리:
`general-purpose`, `paper-reviewer`, `paper-reader`, `paper-writer`,
`code-reviewer`, `backend-architect`, `ml-engineer`, `data-scientist` 등

### Soul System (사용자별 판단 시뮬레이터)

**3-Layer 아키텍처**: Persona (팀 기본) + Soul (개인 override) → Merged 판단 기준

```
.c4/personas/persona-developer.md   ← 팀 기본 (27개)
.c4/souls/changmin/soul-developer.md ← 개인 override
                                       → ResolveSoul() = persona + soul 병합
```

- **Workflow-Soul 연동**: 워크플로우 단계별 활성 역할 자동 전환 (EXECUTE→developer, CHECKPOINT→developer+ceo)
- **Learn Loop**: 태스크 완료 → autoLearn → Soul Learned 섹션 자동 축적
- **MCP 도구**: `c4_soul_get`, `c4_soul_set`, `c4_soul_resolve`

### SOUL (.c4/SOUL.md)

운영 철학 — DoD 없는 작업 금지, 테스트 없는 병합 금지, 리스크 기반 리뷰 우선순위.

### Knowledge Store

`.c4/knowledge/docs/` — Obsidian Markdown SSOT.
4가지 문서 유형: `experiment`, `pattern`, `insight`, `hypothesis`.

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
      "type": "stdio",
      "command": "/path/to/c4/c4-core/bin/c4",
      "args": ["mcp", "--dir", "/path/to/c4"],
      "env": { "C4_PROJECT_ROOT": "/path/to/c4" }
    }
  }
}
```

> `install.sh`가 자동으로 올바른 경로를 설정합니다.

### Project Config (.c4/config.yaml)

```yaml
# Project metadata
project_id: my-project
default_branch: main
domain: web-backend
work_branch_prefix: c4/w-          # Worker별 브랜치 접두사

# Review-as-Task
review_as_task: true                # T-XXX 생성 시 R-XXX 자동 생성
checkpoint_as_task: true            # CP를 태스크로 관리
max_revision: 3                     # REQUEST_CHANGES 최대 횟수 (초과 시 blocked)

# Economic mode — Claude 모델 라우팅
economic_mode:
  enabled: false
  preset: standard                  # standard | economic | ultra-economic | quality
  # model_routing:                  # preset 개별 override (선택)
  #   implementation: sonnet
  #   review: opus
  #   checkpoint: opus

# Validation commands
validation:
  lint: "uv run ruff check ."
  unit: "uv run pytest tests/unit/ -v"

# Git worktree (Worker별 독립 작업 디렉토리)
worktree:
  enabled: true
  auto_cleanup: true
```

#### Economic Mode Presets

| Preset | Implementation | Review | Checkpoint | Scout |
|--------|---------------|--------|------------|-------|
| `standard` | sonnet | opus | opus | haiku |
| `economic` | sonnet | sonnet | sonnet | haiku |
| `ultra-economic` | haiku | sonnet | sonnet | haiku |
| `quality` | opus | opus | opus | sonnet |

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
| `/c4-quick` | 빠른 시작 |
| `/c4-interview` | 요구사항 탐색 인터뷰 |
| `/c4-swarm` | 병렬 Worker 스폰 |
| `/c4-release` | Changelog 생성 |
| `/c4-clear` | 상태 초기화 |

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
[C4 License](./LICENSE.md)

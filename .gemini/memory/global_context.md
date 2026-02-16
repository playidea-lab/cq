# Global Project Context (Gemini Memory)

This file serves as the shared long-term memory for all Gemini CLI agents.
**DO NOT DELETE** this file. Append new insights or update outdated information.

## Project Architecture: C4 v2.0 (Go Hybrid)

### Core Components
- **c5 (Go Core)**: The high-performance engine handling MCP, Configuration, and Validation.
  - Path: `c5/`
  - Key Files: `c5/cmd/main.go`, `c5/internal/mcp/server.go`
- **c4-core (Python Orchestrator)**: Manages complex agent logic and legacy tasks.
  - Path: `c4-core/`
  - Key Files: `c4-core/c4/agent/`, `c4-core/c4/system/`
- **c1 (Frontend)**: React + Tauri UI for user interaction.
  - Path: `c1/`

### Operational Rules
- **Task Protocol**: Always use `c4_status` -> `c4_claim` -> `c4_work` -> `c4_validate` -> `c4_submit`.
- **Validation**: Mandatory `c4_validate` before submission.
- **Style**: Go (idiomatic, `gofmt`), Python (`ruff`, type hints), TypeScript (`eslint`, strict mode).

## Key Paths & Patterns
- **MCP Servers**: Defined in `c5/internal/mcp/` and `c4-core/c4/mcp/`.
- **Configuration**: Managed by `c5` (Viper) with env overrides.
- **Database**: `c5.db` (SQLite) stores task state.

## Current Focus
- Migration to full Go-based core (`c5`).
- Enhancing `c1` UI for better task visibility.

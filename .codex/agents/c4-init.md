---
name: c4-init
description: "Codex CLI용 C4 초기화 및 연결 점검"
triggers:
  - c4 init
  - init c4
  - 초기화
---

# Goal
현재 프로젝트를 Codex 경로로 초기화하고 MCP 연결까지 확인합니다.

## Workflow
1. 프로젝트 루트에서 `cq codex` 실행.
2. 자동 동기화 확인:
   - `.mcp.json`
   - `.codex/agents/c4-*.md`
   - `~/.codex/config.toml`의 `[mcp_servers.cq]`
3. `c4_status()` 호출로 툴 연결 확인.
4. 필요 시 `cq doctor`로 환경 진단.

## Safety Rules
- 초기화 전에 현재 디렉터리가 대상 프로젝트인지 확인.
- 기존 `.mcp.json`이 있더라도 덮어쓰기 영향 범위를 사용자에게 고지.

## Output Checklist
- [ ] 초기화 명령 결과
- [ ] 동기화된 파일/설정
- [ ] `c4_status` 확인 결과

---
name: c4-finish
description: "구현 이후 마무리 루틴 (gate 확인 -> build/test -> 설치 -> 문서/기록)"
triggers:
  - c4 finish
  - finish
  - 마무리
---

# Goal
품질 게이트를 확인한 뒤, 릴리스 가능한 상태로 마무리합니다.

## Workflow
1. Gate 확인:
   - `.c4/c4.db`에서 최근 `c4_gates` 조회
   - `polish | done`이 없으면 중단하고 `c4-polish`로 라우팅
2. `c4_phase_lock_acquire(phase="finish")`.
3. 빌드/검증:
   - `cd c4-core && go build ./... && go vet ./...`
   - 변경 범위에 따라 `go test`/`pytest`/`cargo test` 실행
4. 설치:
   - Codex MCP의 실제 `command` 경로를 우선 사용:
     - `TARGET_CQ_BIN=$(awk '/^\[mcp_servers\\.cq\\]/{f=1;next} f&&/^command = /{gsub(/command = |"/,"");print;exit} f&&/^\\[/{exit}' ~/.codex/config.toml)`
     - 비어 있으면 fallback: `~/.local/bin/cq`
   - `cd c4-core && go build -o "$TARGET_CQ_BIN" ./cmd/c4/`
5. 문서/기록:
   - 변경된 Codex 가이드/에이전트 문서 반영
   - 필요 시 `c4_knowledge_record(...)`
6. 커밋 후 `c4_phase_lock_release(phase="finish")`.

## Safety Rules
- 게이트 미충족 상태에서 finish 진행 금지.
- lock 획득 실패 시 override 여부를 사용자에게 확인.
- 실패 시 원인 해결 전 다음 단계 진행 금지.

## Output Checklist
- [ ] gate 상태
- [ ] build/test 결과
- [ ] 설치 결과
- [ ] lock release 결과

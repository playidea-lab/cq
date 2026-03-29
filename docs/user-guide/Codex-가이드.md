# Codex CLI 가이드

C4를 Codex CLI에서 운영하는 표준 경로입니다. 기본 원칙은 **구현 태스크는 Worker 프로토콜로 처리**하는 것입니다.

## 1. 빠른 시작 (권장)

프로젝트 루트에서 실행:

```bash
c4 codex
```

`c4 codex`는 다음을 자동으로 수행합니다.
- 프로젝트 `.mcp.json` 갱신
- `CLAUDE.md`/`.claude/skills` 동기화
- `~/.codex/config.toml`의 `[mcp_servers.cq]` 블록 갱신
- 프로젝트 `.codex/agents`의 `c4-*.md` 동기화

필요 시 수동 스크립트도 사용 가능합니다:

```bash
scripts/codex/setup-mcp.sh --write
scripts/codex/install-agents.sh
```

## 2. 표준 실행: Worker 모드

사용 조건:
- C4 워크플로우의 구현 태스크 전반

실행 순서:
1. `c4_status()`로 상태/큐 확인
2. `PLAN`/`HALTED`면 `c4_start()`
3. `c4_get_task(worker_id="codex-worker-...")`
4. 구현 후 `c4_run_validation(...)`
5. 커밋 SHA 확인
6. `c4_submit(task_id, worker_id, commit_sha, validation_results)`

주의:
- `c4_submit` 전 태스크가 `in_progress`인지 확인합니다.
- Worker 결과 보고 시 실제 코드 변경(`commit_sha`)이 없으면 완료로 간주하지 않습니다.

## 3. Direct 모드 (예외 경로)

사용 조건:
- `execution_mode=direct`로 생성된 태스크를 처리할 때만 사용

실행 순서:
1. `c4_claim(task_id="...")`
2. 구현 + 검증 + 커밋
3. `c4_report(task_id, summary, files_changed)`

주의:
- Direct 태스크에 `c4_submit`을 호출하면 거절됩니다.
- Worker 태스크를 Direct로 처리하지 않습니다.

## 4. Claude Code 대비 운영 포인트

| 항목 | Claude Code | Codex CLI |
|------|-------------|-----------|
| 실행 진입 | Skill 자동 트리거 | Agent 트리거 + MCP 호출 |
| 기본 구현 경로 | Worker 중심 (`/run`) | Worker 중심 (`c4-run` agent + `c4_get_task`) |
| 예외 처리 | Direct는 제한적 사용 | Direct는 `execution_mode=direct` 태스크에 한정 |

## 5. 체크리스트

- [ ] `c4 codex` 실행 후 `c4_status()` 호출로 연결 확인
- [ ] 구현 태스크는 Worker 프로토콜(`get_task/submit`) 사용
- [ ] Direct 태스크만 `claim/report` 사용
- [ ] 제출 전 `commit_sha`와 검증 결과를 함께 확인

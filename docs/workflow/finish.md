# /c4-finish

Trigger: `/c4-finish` or keywords: `마무리`, `finish`, `완료`

## What it does

Post-implementation completion routine. Run once all tasks are done.

```
1. Polish loop (build-test-review-fix until zero changes, max 8 rounds)
2. Record polish gate (c4_record_gate)
3. Acquire phase lock (prevents concurrent finish)
4. Build verification (project-specific)
5. Run full test suite
6. Install binary (make install)
7. Record session knowledge (c4_knowledge_record)
8. Commit all changes
9. Release phase lock
```

The polish loop is the same gate that workers use during `/c4-run`. Whether triggered by a worker or manually via `/c4-finish`, the Go-level gate ensures convergence.

## When to run

`/c4-run` calls `/c4-finish` automatically when the queue empties — you typically don't need to invoke it manually.

Run it manually when you've made additional changes after `/c4-run` completed, or when running only `/c4-plan` + manual edits without workers.

## Build verification

CQ detects your project type automatically:

| Project type | Build command |
|-------------|---------------|
| Go | `go build ./... && go vet ./...` |
| Python | `uv run python -m compileall . && uv run pytest tests/ -x` |
| Node | `npm run build` |
| Rust | `cargo build` |

Override in `.c4/config.yaml`:
```yaml
validation:
  build_command: "make build"
  test_command: "make test"
```

## Skip polish

```
/c4-finish --no-polish    # skip polish phase (emergency deploys only)
```

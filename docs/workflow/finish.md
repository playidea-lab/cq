# /c4-finish

Trigger: `/c4-finish` or keywords: `마무리`, `finish`, `완료`

## What it does

Post-implementation completion routine. Run once all tasks are done.

```
1. Run /c4-polish (fix until reviewer finds zero changes)
2. Acquire phase lock (prevents concurrent finish)
3. Build verification (project-specific)
4. Run full test suite
5. Update documentation
6. Record session knowledge (c4_knowledge_record)
7. Commit all changes
8. Generate CHANGELOG (calls /c4-release)
9. Release phase lock
```

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

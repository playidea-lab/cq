# /c4-finish

Trigger: `/c4-finish` or keywords: `마무리`, `finish`, `완료`

## What it does

Post-implementation completion routine. Run once all tasks are done.

```
1. Check for unmerged worktree branches
2. Acquire phase lock (prevents concurrent finish)
3. go build ./... && go vet ./...   (or project-specific build)
4. Run full test suite
5. Install binary to ~/.local/bin/cq
6. Update AGENTS.md / docs with new counts
7. Record session knowledge (c4_knowledge_record)
8. Commit all changes
9. Generate CHANGELOG (calls /c4-release)
10. Release phase lock
```

## When to run

After `/c4-run` completes and `/c4-status` shows all tasks done.

## Build verification

CQ detects your project type automatically:

| Project type | Build command |
|-------------|---------------|
| Go | `go build ./... && go vet ./...` |
| Python | `uv run python -m py_compile` |
| Node | `npm run build` |
| Rust | `cargo build` |

Override in `.c4/config.yaml`:
```yaml
validation:
  build_command: "make build"
  test_command: "make test"
```

## Skip steps

```
/c4-finish --skip-tests    # if tests are slow and already verified
```

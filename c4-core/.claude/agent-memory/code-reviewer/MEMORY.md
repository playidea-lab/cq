# Code Reviewer Agent Memory

## Project: c4-core (Go MCP Server)

### Architecture Overview (2026-02-08)
- **Primary**: `cmd/c4/mcp.go` + `internal/mcp/handlers/` = Go MCP server (stdio)
- **Bridge**: `internal/bridge/sidecar.go` manages Python sidecar (JSON-RPC/TCP)
- **Proxy**: `internal/mcp/handlers/proxy.go` forwards LSP/Knowledge/GPU to sidecar
- **Store**: `internal/mcp/handlers/sqlite_store.go` reads `.c4/c4.db` or `.c4/tasks.db`

### Key Finding: Dead Code Packages
- `internal/server/` (669 LOC) - Cloud HTTP server, Fly.io integration - NEVER imported
- `internal/auth/` (~600 LOC) - OAuth/PKCE/Session management - NEVER imported
- `internal/realtime/` (382 LOC) - Supabase WebSocket subscriptions - NEVER imported
- `internal/bridge/grpc_client.go` (569 LOC) - behind `//go:build grpc` tag (intentional gating)
- `internal/worker/worker.go` - just a package doc comment, empty
- `internal/git/git.go` - just a package doc comment, empty
- `internal/state/` - has tests but no imports from cmd/ or handlers/
- `internal/task/` - has full TaskStore impl + SupabaseStore, but not used by MCP handlers
- `internal/config/` - has full config loading via viper, only used by validation/runner.go

### Key Finding: Tool Count Mismatch
- register.go comment says "Total: 50 tools" but actual count is **47** unique tools
- 47 matches the .mcp.json expectation but comment is wrong

### Key Finding: Proxy Default Address Hardcoded
- `proxy.go:22` defaults to `localhost:50051` when addr is empty
- But sidecar uses dynamic port (port 0 = auto-assign)
- This means if sidecar fails, proxy will try wrong address

### Key Finding: Lighthouse Feature Issues (2026-02-14)
- `lighthousePromote` schema validation is dead code: Unregister(name) before GetToolSchema(name)
- T-LH- task ID parsing uses LastIndex("-") -- fragile if name contains hyphens, no name validation at registration
- `lighthouseRegister` raw SQL `UPDATE c4_lighthouses SET task_id=?` bypasses store abstraction, swallows errors
- Promote task completion via raw SQL skips persona stats/auto-learn
- `validateSchemaCompat` only checks property names + required, not types
- Lighthouse operations not atomic between Registry + SQLite (race possible in theory)
- No re-register after deprecate (getLighthouse finds deprecated record, blocks re-register)

### Key Finding: SkillEval SHP Issues (2026-03-06)
- `runner.go`: On LLM call error, appends `false` to `cr.Trials` (counts as real trial). When all k calls fail, silently skews accuracy for negative test cases (they appear correct).
- `runner.go`: `os.Stat(evalPath)` error triggers auto-generate for ANY error (not just `os.IsNotExist`). Permissions errors cause spurious LLM calls.
- `handler.go`: No upper bound on `k` param — caller can send k=1000, triggering 1000×N LLM calls per tool invocation.
- `eval.go` + `handler.go`: `skillName` used directly in `filepath.Join` without `..`/`/`/`\` check. Project pattern is `strings.Contains(id, "..")` validation (see `persona.go:54`).
- `runner.go`: `AvgConfidence` divides by `len(cr.Trials)` which includes error-appended entries, not just successful LLM calls — confidence metric is diluted.
- Missing test cases: all-error scenario, k upper bound enforcement, path traversal rejection.

### Key Finding: Dataset Versioning Pull Guard Bug (2026-03-06)
- `dataset.go` Pull(): path traversal guard uses `filepath.Clean(dest)+sep` as prefix
- Guard ONLY works with absolute dest. With relative dest (`.`, `""`, `./foo`), `filepath.Clean("data.csv")+"/"` = `"data.csv/"` which does NOT HasPrefix `"./"` → every legitimate file rejected
- CLI default `--dest "."` triggers this bug → Pull is completely broken for default usage
- Fix: add `filepath.Abs(dest)` before computing `destClean`
- Test gap: `TestDatasetPull_Incremental` uses `t.TempDir()` (absolute), masks the bug

### Key Finding: Experiment Hub Lifecycle Issues (2026-03-12)
- `hub.go`: `--experiment` flag creates an experiment run but never sets `req.ExpRunID` on the job submit request — run and job are orphaned, lifecycle bridge never fires.
- `jobs.go` `maybeCompleteExperimentRun`: stores `"succeeded"` (lowercased from `model.StatusSucceeded = "SUCCEEDED"`) but c5 API `validStatuses` and c4-core `validStatuses` both use `"success"`. Store accepts any string, so inconsistent status values accumulate silently.
- `experiment_handler.go` `hubPost`/`hubGet`: use `http.DefaultClient` (no timeout). MCP request hangs indefinitely if Hub is slow. `hub/client.go` uses 30s timeout — same pattern should apply.
- `c5/api/experiment.go` `handleExperimentCreateRun`: no empty-name validation — unnamed runs silently inserted.
- `c5/api/experiment.go` `handleExperimentSearch`: no upper bound on `limit` param — unbounded DB scan possible.
- `c4-core/store/experiment.go` `ShouldContinue`: returns `(false, nil)` for unknown run_id; c5 store returns `(false, ErrRunNotFound)` — divergent behaviour for same interface operation.

### Patterns Observed
- All handler files follow consistent pattern: Register*Handlers + handle* functions
- No hardcoded user paths in Go code (good)
- Build tags properly used for gRPC (grpc_client.go)
- Error handling generally good but some `_ = err` silencing in sqlite_store.go
- Best-effort migrations with `_, _ = s.db.Exec(m)` swallow all errors including non-duplicate-column failures
- **Path validation pattern**: entity IDs validated with `strings.Contains(id, "..")` + `/` + `\` (persona.go:54); file paths validated with `filepath.Clean` + `HasPrefix` (fileops.go:174-182)
- **LLM task type routing**: `"scout"` task type resolves to haiku via `Routes["default"]` fallback when not explicitly mapped — safe, matches knowledgehandler pattern
- **filepath.Clean+HasPrefix traversal guard**: requires absolute path as base. Always call `filepath.Abs(base)` before computing the prefix guard, otherwise relative paths break the guard entirely (both blocking legitimate files AND failing to catch traversal).
- **PostgREST Prefer: resolution=ignore-duplicates**: returns 200/201 on conflict (not 409). The 409 dead-code check is harmless but a sign the response semantics were misunderstood.
- **validateName empty-string gap**: `validateName("")` returns nil — callers must check for empty separately. Pattern in this codebase: explicit `if args.Name == ""` check at handler level.

### Key Finding: MCP Apps Widget System (2026-03-23)
- [review_mcp_apps_widgets.md](review_mcp_apps_widgets.md) — 11 widgets, 2 dead (nodes-map, experiment-compare never wired), 4 untested handlers, XSS defense verified

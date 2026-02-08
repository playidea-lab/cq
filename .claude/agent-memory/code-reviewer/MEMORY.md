# Code Reviewer Agent Memory

## Review History

### 2026-02-08: Canvas-App Project Explorer (REQUEST CHANGES)
- **Commit**: 9d4b10a, 25 files, +2770/-40 lines (Rust + React + CSS)
- **Result**: REQUEST CHANGES - 2 CRITICAL, 4 HIGH, 6 MEDIUM, 5 LOW
- **Critical**: Path traversal in read_config_file (arbitrary file read), CSP disabled in tauri.conf.json
- **Key bugs**: has_more pagination always false (off-by-one in collected counter), StatusBadge.replace only first underscore
- **Performance**: extract_session_meta reads entire file per session (154+ sessions, up to 57MB each); pagination re-reads from beginning
- **Security pattern**: Tauri IPC commands accepting raw file paths from frontend without validation = path traversal
- **Testing gap**: Zero frontend tests, Rust tests only cover utility functions not IPC commands
- **Lesson**: Tauri apps need path allowlisting on every file-access command; CSP null = no XSS protection
- **DRY**: formatSize duplicated in SessionList.tsx vs ConfigView.tsx with different implementations

### 2026-02-08: Canvas-App Fix Tasks Batch Review (ALL APPROVED)
- **Tasks**: R-CVR-004 through R-CVR-008 (5 fix tasks from initial review)
- **Result**: ALL APPROVE - 29 tests passing, build succeeds, all DoD items met
- **R-CVR-004**: vitest+RTL setup, 4 test files (format, StatusBadge, ProgressBar, MessageViewer), 29 tests
- **R-CVR-005**: formatSize/formatDate extracted to src/utils/format.ts, no duplicates remain
- **R-CVR-006**: StatusBadge uses /_/g regex (replaces all underscores)
- **R-CVR-007**: key={`${view}-${projectPath}`} on all 3 view components in App.tsx
- **R-CVR-008**: [...taskList].sort() immutable sort in useDashboard.ts

### 2026-02-07: User Profile Auto-Learning System - Follow-up Review (APPROVED)
- **Initial Review**: 2 CRITICAL, 6 WARNING, 5 SUGGESTION findings
- **Fix Commits**: 9f12982 (CRITICAL), 74b1d24 (WARNING)
- **Follow-up Result**: APPROVE - All CRITICAL/WARNING fixes correctly implemented
- **Tests**: 46 profile tests, all passing. No regressions in related test suites.
- **Remaining (deferred)**: substring matching in _extract_keywords, unbounded observation growth
- **Key lesson**: MCP handler hooks follow try/except pass pattern consistently - good for non-critical operations

### 2026-02-07: Weighted Workflow E2E Verification (READ-ONLY REVIEW)
- **Scope**: Full pipeline trace from persona YAML -> AgentGraphLoader -> ProfileLearner -> task_ops -> builder
- **Result**: PASS - No broken links in the data flow. All 72 tests passing.
- **Findings**: 0 CRITICAL, 2 WARNING (DRY violation in sorting/emphasis logic, repeated AgentGraphLoader instantiation), 4 SUGGESTION
- **Key insight**: Pydantic v2 auto-coerces dict->WorkflowWeight in `dict[str, dict[str, WorkflowWeight]]` type annotation on YAML roundtrip
- **DRY concern**: task_ops._build_workflow_instructions() and builder._build_user_context_section() duplicate workflow sorting/emphasis logic
- **Missing test**: No test that _build_user_context includes "## Workflow (user-adapted)" when workflow_weights exist

## Recurring Patterns in C4 Codebase
- Exception handling: non-critical operations wrapped in try/except with logger.debug (good pattern)
- Lazy initialization via @property pattern for daemon components
- MCP handlers follow: register_tool decorator + handler function + daemon delegation
- YAML registry follows consistent schema: skill.{id,name,description,impact,category,domains,metadata,capabilities,triggers,rules}
- AgentGraphLoader is frequently re-instantiated (no singleton/cache) - loads all ~27 YAML files each time
- Persona YAML workflow_steps: only 4 of ~27 personas have them (paper-reviewer, paper-reader, paper-writer, knowledge-engineer)
- WorkflowWeight sorting/emphasis logic duplicated in task_ops.py and builder.py - watch for drift

## Canvas-App (Tauri 2.x) Patterns
- Tauri IPC commands: async fn + spawn_blocking for I/O, return Result<T, String>
- Path validation: MUST canonicalize + allowlist check for any file-access command
- CSP: Never set to null in production; use restrictive default-src 'self'
- JSONL session files: located at ~/.claude/projects/{slug}/*.jsonl, can be 50MB+
- Session pagination: line-based offset/limit, must handle large files efficiently
- Frontend: BEM CSS + design tokens (--color-*, --space-*, --font-size-*), React hooks per view

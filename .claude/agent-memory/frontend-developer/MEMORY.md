# Frontend Developer Agent Memory

## Canvas App (C1) Architecture
- **Tauri 2.x** desktop app at `/Users/changmin/git/c4/c1/`
- **Rust backend**: `src-tauri/src/{commands,models,scanner,providers,watcher,lib}.rs`
- **React frontend**: `src/components/`, `src/hooks/`, `src/styles/`
- **CSS**: BEM pattern + `styles/tokens.css` design tokens
- **Tests**: Vitest + Testing Library, config in `vite.config.ts` (globals: true)
- **Build**: `pnpm build` runs `tsc && vite build`

## Key Patterns
- Tauri commands use `tokio::task::spawn_blocking` for I/O
- Frontend hooks follow `use{Feature}` convention (useSessions, useEditors, etc.)
- Tauri invoke mock: `vi.mock('@tauri-apps/api/core', () => ({ invoke: vi.fn() }))`
- Button styles: `.btn .btn--sm .btn--secondary` classes from `global.css`

## tauri-plugin-shell API (v2.3.5)
- `Shell::command()` returns `Command` from `process` module
- `Command::spawn()` returns `Result<(Receiver<CommandEvent>, CommandChild)>` NOT just CommandChild
- `Command::new()` is `pub(crate)` -- must use `shell.command()` via ShellExt trait
- For custom Tauri commands, can use `std::process::Command` directly (no shell plugin permissions needed)
- Shell plugin permissions (`shell:allow-spawn`, etc.) only gate JS-side shell API, not Rust-side

## Known Issues (as of 2026-02-08)
- `SessionsView.test.tsx` has 5 pre-existing failures (mocks reference `list_sessions` but component calls `list_sessions_for_provider`)
- `afterEach` was missing from vitest import in SessionsView.test.tsx (fixed)
- `watcher.rs` has unused import warnings (pre-existing)

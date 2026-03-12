# Code Reviewer Agent Memory

## Project Patterns

### Shell Scripts (CQ codebase)
- `grep -oP` (PCRE) is NOT supported by macOS BSD grep — always use POSIX-compatible alternatives or `awk`/`sed`
- Background process (`&`) always needs `trap 'kill $PID' EXIT` guard — `set -euo pipefail` alone does not ensure cleanup on unexpected exits
- `sleep N` readiness checks should be replaced with polling loops (deterministic)
- Hub submit output format: `Job submitted: <id> (status=..., queue_position=N)` — not `job_id: <id>`

### Go (hub_worker.go / systemd unit generation)
- systemd `Environment=` values with spaces must be quoted: `Environment="KEY=value with spaces"`
- Unit files written to `/etc/systemd/system/` should be `0o600` if they contain secrets, not `0o640`
- Package-level flag variables (cobra) mutated inside RunE should be avoided for testability — prefer local vars
- Config path for c5 worker: `~/.c5/config.yaml` (NOT `~/.c4/worker.yaml`)

### os.Exit patterns (c5/worker.go)
- `os.Exit(1)` after successful upgrade is correct for `systemd Restart=on-failure` and `launchd KeepAlive`
- Non-zero exit intent should always be logged explicitly

### Test Coverage Gaps to Watch
- When a function signature changes (added parameter), dry-run tests must be updated to assert new output
- `buildSystemdUnit` needs unit test for: key with spaces (quoting), injection chars, empty key

## Recurring Issues (seen across reviews)
1. macOS/Linux portability: `grep -oP`, `sed -i` (no backup arg on macOS), `date -d`
2. Missing `trap EXIT` on scripts with background processes
3. Plaintext secrets in config files — flag for secrets.db migration path
4. Doc path drift: code changes without updating docs that reference specific file paths
